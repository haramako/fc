	.export _interrupt
	.export _interrupt_irq
	
.segment "ppu"
	
;; function interrupt():void
_interrupt:

	lda _ppu_vsync_flag				; if( vsync_flag ){
	beq @else

	cmp #1						;   if( vsync_flag == 1 ){
	bne @else2
	lda #0						;     PPU_SPR_ADDR = 0;
	lda _nes_PPU_SPR_ADDR
	lda #7						;     SPRITE_DMA = 7;
	sta _nes_SPRITE_DMA
	lda #0						;     gr_sprite_idx = 0;
	sta _ppu_gr_sprite_idx
@else2:							;   }

	lda #0						;   i = 0;
	sta _ppu_interrupt_i
@loop:					  
	lda _ppu_interrupt_i			;   while( i < gr_idx ){
	cmp _ppu_gr_idx
	bpl @end
	jsr ppu_put3				;     ppu_put3(interrupt_i)
	inc _ppu_interrupt_i			;     i += 1;
	jmp @loop					;   }
@end:		
	lda #0						;   vsync_flag = gr_idx = 0
	sta _ppu_vsync_flag
	sta _ppu_gr_idx
@else:							; }
	
	lda _ppu_locked
	bne @else3

	lda _ppu_scroll1			; PPU_SCROLL1 = ppu_scroll1;
	sta _nes_PPU_SCROLL
	lda _ppu_scroll2			; PPU_SCROLL2 = ppu_scroll2;
	sta _nes_PPU_SCROLL
	lda _ppu_ctrl1_bak			; PPU_CTRL1 = ppu_ctrl1_bak;
	sta _nes_PPU_CTRL1
	lda _ppu_ctrl2_bak			; PPU_CTRL2 = ppu_ctrl2_bak;
	sta _nes_PPU_CTRL2

	;; setup irq
	jsr @jsr_irq_setup

@else3:

	jsr @jsr_on_vsync

	rts

@jsr_irq_setup:
	jmp (_ppu_irq_setup)

@jsr_on_vsync:
	jmp (_ppu_on_vsync)
	
;;; IRQ割り込み
;;; 丁度 113-134 サイクルで終わらせる必要あり
;;; ( irqベクタで24cycle使用しているので、実質89-110サイクル)
_interrupt_irq:
	jmp (_ppu_irq_next)

;;; same as ppu_put, but in interrupt
ppu_put3:
	ldx _ppu_interrupt_i			; ppu_put_size = gr_size_buf[i]
	lda _ppu_gr_size_buf,x
	sta _ppu_put_size

	lda _ppu_gr_flag_buf,x			; if( gr_flag_buf[i] & (1<<7) ){
	bpl @else2
	jmp ppu_put3_custom			;   goto ppu_put3_custom;
@else2:
	rol a
	;; 	bvc .else1
	bpl @else1					; }elsif( gr_flag_buf[i] & (1<<6) ){
	lda #%10000100				;   PPU_CTRL = 1 << 2 // VRAM address increment 32
	jmp @end1
@else1:							; }else{
	lda #%10000000				;   PPU_CTRL = 0 // VRAM address increment 1
@end1:							; }
	sta _nes_PPU_CTRL1

	txa							; (x = i*2 )
	asl a
	tax

	lda _ppu_gr_to_buf+1,x			; PPU_ADDR = gr_to_buf[i]
	sta _nes_PPU_ADDR
	lda _ppu_gr_to_buf+0,x
	sta _nes_PPU_ADDR
		
	lda _ppu_gr_from_buf+0,x		; gr_from_buf = ppu_put_from
	sta _ppu_put_from+0
	lda _ppu_gr_from_buf+1,x
	sta _ppu_put_from+1
	ldy #0
@loop:
    lda (_ppu_put_from),y
    sta _nes_PPU_DATA
    iny
	cpy _ppu_put_size
    bne @loop
    rts
	
ppu_put3_custom:
	rol _ppu_put_size			; ppu_put_size *= 8
	rol _ppu_put_size
	rol _ppu_put_size
	lda #%10000000				; PPU_CTRL = 0 // VRAM address increment 1
	sta _nes_PPU_CTRL1

	txa							; (x = i*2 )
	asl a
	tax

	lda _ppu_gr_to_buf+1,x			; ppu_put_to = gr_to_buf[i]
	sta _ppu_put_to
	lda _ppu_gr_to_buf+0,x
	sta _ppu_put_to+1
		
	lda _ppu_gr_from_buf+0,x		; gr_from_buf = ppu_put_from
	sta _ppu_put_from+0
	lda _ppu_gr_from_buf+1,x
	sta _ppu_put_from+1
	ldy #0
@loop:
	lda _ppu_put_to
	sta _nes_PPU_ADDR
	lda _ppu_put_to+1
	sta _nes_PPU_ADDR
	clc
	adc #8
	sta _ppu_put_to+1
	
    lda (_ppu_put_from),y
    sta _nes_PPU_DATA
	
	tya							; y += 8
	clc
	adc #8
	tay
	
	cpy _ppu_put_size
    bne @loop
    rts

;;; ppu_put internal
ppu_put_sub:
        lda _ppu_put_to+1
        sta _nes_PPU_ADDR
        lda _ppu_put_to+0
        sta _nes_PPU_ADDR
        ldy #0
@loop:
        lda (_ppu_put_from),y
        sta _nes_PPU_DATA
        iny
        cpy _ppu_put_size
        bne @loop
        sty $100
        rts
		
;;; function ppu_put(addr:int16, from:int*, size:int ):void
_ppu_put_in_lock:
		lda S+0,x
		sta _ppu_put_to+0
		lda S+1,x
		sta _ppu_put_to+1
		lda S+2,x
		sta _ppu_put_from+0
		lda S+3,x
		sta _ppu_put_from+1
		lda S+4,x
		sta _ppu_put_size
		jmp ppu_put_sub

;;; function ppu_fill_in_lock(addr:int16, size:int16, n:int)
;;; use: reg[0]
_ppu_fill_in_lock:
	lda S+1,x
	sta _nes_PPU_ADDR
	lda S+0,x
	sta _nes_PPU_ADDR
	
	;; low loop
	lda S+4,x
	ldy S+2,x
	beq @end1
@loop1:
	sta _nes_PPU_DATA
	dey
	bne @loop1
@end1:

	;; high loop
	stx reg+0					; backup x
	lda S+3,x
	tay
	beq @end2
	lda S+4,x
@loop2:
	ldx #0
@loop3:
	sta _nes_PPU_DATA
	dex
	bne @loop3
	dey
	bne @loop2
@end2:
	ldx reg+0					; restore x
	
	rts
	
	
		
;;; function gr_pos( x:int, y:int ):int16
_ppu_pos:
	lda S+3,x					; if( y < 0 ) y += 30;
	bpl @end2
	clc
	adc #30
	sta S+3,x
@end2:	
	lda S+3,x					; if( y > 30 ) y -= 30;
	cmp #30
	bmi @end
	sec
	sbc #30
	sta S+3,x
@end:	
	lda S+3,x		; result[0] = x + y * 32
	asl a
	asl a
	asl a
	asl a
	asl a
	clc
	adc S+2,x
	sta S+0,x
	lda S+3,x       ; result[1] = 0x20 + y / 8
	lsr a
	lsr a
	lsr a
	clc
	adc #$20
	sta S+1,x
	rts
		
		

;; function ppu_sprite( x:int, y:int, pat:int, mode:int ):void options (extern:true) {}
;; {
;;   if( gr_sprite_idx >= 252 ){ return; }
;;   var p:int = gr_sprite_idx;
;;   gr_sprite_buf[p] = y-1;
;;   gr_sprite_buf[p+1] = pat+1;
;;   gr_sprite_buf[p+2] = mode;
;;   gr_sprite_buf[p+3] = x;
;;   gr_sprite_idx += 4;
;; }
;; USING: X
_ppu_sprite:
    ldy _ppu_gr_sprite_idx      ; if( gr_sprite_idx >= 252 ){ return; } var p:int = gr_sprite_idx;
    cpy #252
    bcs @end
    lda S+1,x      ; gr_sprite_buf[p] = y;
	sec
	sbc #1
    sta _ppu_gr_sprite_buf,y   
    iny                     ; gr_sprite_buf[p+1] = pat;
    lda S+2,x
	;; ora #1
    sta _ppu_gr_sprite_buf,y
    iny                     ; gr_sprite_buf[p+2] = mode;
    lda S+3,x
    sta _ppu_gr_sprite_buf,y
    iny                     ; gr_sprite_buf[p+3] = x;
    lda S+0,x
    sta _ppu_gr_sprite_buf,y
    iny                     ; gr_sprite_idx += 4;
    sty _ppu_gr_sprite_idx
@end:
    rts
        
