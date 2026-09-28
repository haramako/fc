	
.segment "CODE"
	
_stdio_ppu_put:
		;; abi "frame": addr = F_stdio_ppu_put+0,1、data = +2,3、size = +4
		lda F_stdio_ppu_put+1
		sta _nes_PPU_ADDR
		lda F_stdio_ppu_put+0
		sta _nes_PPU_ADDR
		ldy #0
		lda F_stdio_ppu_put+4		; size 0 なら何も書かない (256 バイトになっていた)
		beq @end
@loop:
		lda (F_stdio_ppu_put+2),y
		sta _nes_PPU_DATA
		iny
		cpy F_stdio_ppu_put+4
		bne @loop
@end:
		rts
		
_stdio_print:
		lda _stdio_print_addr+1
		sta _nes_PPU_ADDR
		lda _stdio_print_addr+0
		sta _nes_PPU_ADDR
		
		;; abi "frame": str = F_stdio_print+0,1
		ldy #0
@loop:	
		lda (F_stdio_print+0),y
		beq @end
		iny
		cmp #10
		bne @not_lf
		
		lda _stdio_print_addr+0
		and #%11100000
		clc
		adc #32
		sta _stdio_print_addr+0
		lda #0
		adc _stdio_print_addr+1
		sta _stdio_print_addr+1
		sta _nes_PPU_ADDR
		lda _stdio_print_addr+0
		sta _nes_PPU_ADDR
		jmp @loop
		
@not_lf:		
		sta _nes_PPU_DATA
		inc _stdio_print_addr+0		; print_addr[0,1] += y
		bne @loop
		inc _stdio_print_addr+1
		jmp @loop
@end:
		rts

_interrupt:
		lda #1
		sta _stdio_vsync_flag
		rts
	
_interrupt_irq:
	rts
		
_stdio_wait_vsync:
		lda #0
		sta _stdio_vsync_flag
@loop:
		lda _stdio_vsync_flag
		beq @loop
		rts
		