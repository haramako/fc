	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_STDIO__ = 1
.segment "stdio"
	.include "_nes.inc"
	.include "stdio.asm"
	.export _stdio_vsync_flag
.segment "BSS"
_stdio_vsync_flag: .res 1
	.export _stdio_ppu_addr
.segment "BSS"
_stdio_ppu_addr: .res 2
	.export _stdio_print_addr
.segment "BSS"
_stdio_print_addr: .res 2
	.global _stdio_wait_vsync
	.global _stdio_print
	.global _stdio_ppu_put
	.export _stdio_HEX
.segment "stdio"
_stdio_HEX:
	.byte 48,49,50,51,52,53,54,55,56,57,65,66,67,68,69,70
	.byte 0
	.export _stdio_print_int16
	;;;=============================
	;;; function _stdio_print_int16
	;;;=============================
.segment "stdio"
.proc _stdio_print_int16
	lda 1+<F_stdio_print_int16+0
	lsr a
	lsr a
	lsr a
	lsr a
	sta 0+<F_stdio_print_int16+7
	lda #0
	sta 1+<F_stdio_print_int16+7
	ldy 0+<F_stdio_print_int16+7
	lda _stdio_HEX+0,y
	sta 0+<F_stdio_print_int16+9
	lda #.LOBYTE(F_stdio_print_int16+2+0)
	sta 0+<F_stdio_print_int16+7
	lda #.HIBYTE(F_stdio_print_int16+2+0)
	sta 1+<F_stdio_print_int16+7
	lda 0+<F_stdio_print_int16+9
	ldy #0
	sta (F_stdio_print_int16+7),y
	lda 0+<F_stdio_print_int16+1
	and #15
	tay
	lda _stdio_HEX+0,y
	sta 0+<F_stdio_print_int16+9
	lda #.LOBYTE(F_stdio_print_int16+2+1)
	sta 0+<F_stdio_print_int16+7
	lda #.HIBYTE(F_stdio_print_int16+2+1)
	sta 1+<F_stdio_print_int16+7
	lda 0+<F_stdio_print_int16+9
	ldy #0
	sta (F_stdio_print_int16+7),y
	lda 0+<F_stdio_print_int16+0
	sta 0+<F_stdio_print_int16+7
	lda 1+<F_stdio_print_int16+0
	sta 1+<F_stdio_print_int16+7
	lsr 1+<F_stdio_print_int16+7
	ror 0+<F_stdio_print_int16+7
	lsr 1+<F_stdio_print_int16+7
	ror 0+<F_stdio_print_int16+7
	lsr 1+<F_stdio_print_int16+7
	ror 0+<F_stdio_print_int16+7
	lsr 1+<F_stdio_print_int16+7
	ror 0+<F_stdio_print_int16+7
	lda 0+<F_stdio_print_int16+7
	and #15
	tay
	lda _stdio_HEX+0,y
	sta 0+<F_stdio_print_int16+9
	lda #.LOBYTE(F_stdio_print_int16+2+2)
	sta 0+<F_stdio_print_int16+7
	lda #.HIBYTE(F_stdio_print_int16+2+2)
	sta 1+<F_stdio_print_int16+7
	lda 0+<F_stdio_print_int16+9
	ldy #0
	sta (F_stdio_print_int16+7),y
	lda 0+<F_stdio_print_int16+0
	and #15
	tay
	lda _stdio_HEX+0,y
	sta 0+<F_stdio_print_int16+9
	lda #.LOBYTE(F_stdio_print_int16+2+3)
	sta 0+<F_stdio_print_int16+7
	lda #.HIBYTE(F_stdio_print_int16+2+3)
	sta 1+<F_stdio_print_int16+7
	lda 0+<F_stdio_print_int16+9
	ldy #0
	sta (F_stdio_print_int16+7),y
	lda #.LOBYTE(F_stdio_print_int16+2+4)
	sta 0+<F_stdio_print_int16+7
	lda #.HIBYTE(F_stdio_print_int16+2+4)
	sta 1+<F_stdio_print_int16+7
	lda #0
	sta (F_stdio_print_int16+7),y
	lda #.LOBYTE(F_stdio_print_int16+2+0)
	sta 0+<F_stdio_print_int16+7
	lda #.HIBYTE(F_stdio_print_int16+2+0)
	sta 1+<F_stdio_print_int16+7
	lda 0+<F_stdio_print_int16+7
	sta <F_stdio_print+0
	lda 1+<F_stdio_print_int16+7
	sta <F_stdio_print+1
	jsr _stdio_print
	rts
.endproc
	.export _stdio_exit
	.export _stdio_exit__frame
	;;;=============================
	;;; function _stdio_exit
	;;;=============================
.segment "stdio"
.proc _stdio_exit__frame
	lda <F_stdio_exit+0
	.endproc
.proc _stdio_exit
	sta <F_stdio_exit+0
	lda #.LOBYTE(_47)
	sta <F_stdio_print+0
	lda #.HIBYTE(_47)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda 0+<F_stdio_exit+0
	sta <F_stdio_print_int16+0
	lda #0
	sta <F_stdio_print_int16+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_49)
	sta <F_stdio_print+0
	lda #.HIBYTE(_49)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #200
	sta 0+_nes_PPUCTRL
@then_53:
	jsr _stdio_wait_vsync
	lda #0
	sta 0+_nes_PPUSCROLL
	sta 0+_nes_PPUSCROLL
	lda #200
	sta 0+_nes_PPUCTRL
	lda #10
	sta 0+_nes_PPUMASK
	jmp @then_53
_47:
		.byte 101,120,105,116,40,0
_49:
		.byte 41,10,0
.endproc
	.export _stdio_init
	;;;=============================
	;;; function _stdio_init
	;;;=============================
.segment "stdio"
.proc _stdio_init
	lda #253
	sta 0+_stdio_vsync_flag
	lda #0
	sta <F_stdio_ppu_put+0
	lda #63
	sta <F_stdio_ppu_put+1
	lda #.LOBYTE(_L_pallet)
	sta <F_stdio_ppu_put+2
	lda #.HIBYTE(_L_pallet)
	sta <F_stdio_ppu_put+3
	lda #16
	sta <F_stdio_ppu_put+4
	jsr _stdio_ppu_put
	lda #0
	sta 0+_stdio_print_addr
	lda #32
	sta 1+_stdio_print_addr
	rts
_L_pallet:
		.byte 15,61,16,48,0,17,33,49,0,18,34,50,0,19,35,51
.endproc
	.global _interrupt
_stdio_interrupt = _interrupt
	.global _interrupt_irq
_stdio_interrupt_irq = _interrupt_irq
