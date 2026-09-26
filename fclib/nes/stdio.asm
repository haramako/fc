	
.segment "CODE"
	
_stdio_ppu_put:
		;; fastcall: addr = FC_FASTCALL_REG+0,1、data = +2,3、size = +4
		lda FC_FASTCALL_REG+1
		sta _nes_PPU_ADDR
		lda FC_FASTCALL_REG+0
		sta _nes_PPU_ADDR
		ldy #0
		lda FC_FASTCALL_REG+4		; size 0 なら何も書かない (256 バイトになっていた)
		beq @end
@loop:
		lda (FC_FASTCALL_REG+2),y
		sta _nes_PPU_DATA
		iny
		cpy FC_FASTCALL_REG+4
		bne @loop
@end:
		rts
		
_stdio_print:
		lda _stdio_print_addr+1
		sta _nes_PPU_ADDR
		lda _stdio_print_addr+0
		sta _nes_PPU_ADDR
		
		;; fastcall: str = FC_FASTCALL_REG+0,1
		ldy #0
@loop:	
		lda (FC_FASTCALL_REG+0),y
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

_stdio_print_int16:
		lda S+1,x
		sta reg+4
		call _stdio_print_int8, #2
		lda S+0,x
		sta reg+4
		call _stdio_print_int8, #2
		rts
		
_stdio_print_int8:
		lda reg+4
		ror a
		ror a
		ror a
		ror a
		and #15
		tay
		lda @char,y
		sta reg+5

		lda reg+4
		and #15
		tay
		lda @char,y
		sta reg+6
		
		lda #0
		sta reg+7

		;; print は fastcall (str = FC_FASTCALL_REG+0,1)
		lda #.LOBYTE(reg+5)
		sta FC_FASTCALL_REG+0
		lda #.HIBYTE(reg+5)
		sta FC_FASTCALL_REG+1
		jsr _stdio_print
		
		rts
@char:
		.byte 48,49,50,51,52,53,54,55,56,57,65,66,67,68,69,70
		
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
		