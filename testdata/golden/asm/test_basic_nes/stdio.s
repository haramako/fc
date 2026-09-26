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
	.global _stdio_print_int16
	.global _stdio_wait_vsync
	.global _stdio_print
	.global _stdio_ppu_put
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
	lda #.LOBYTE(_26)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_26)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	ldx FC_SP
	lda 0+<F_stdio_exit+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print_int16
	lda #.LOBYTE(_28)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_28)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #200
	sta 0+_nes_PPU_CTRL1
@then_32:
	ldx FC_SP
	ldx FC_SP
	jsr _stdio_wait_vsync
	lda #0
	sta 0+_nes_PPU_SCROLL
	sta 0+_nes_PPU_SCROLL
	lda #200
	sta 0+_nes_PPU_CTRL1
	lda #10
	sta 0+_nes_PPU_CTRL2
	jmp @then_32
_26:
		.byte 101,120,105,116,40,0
_28:
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
	sta <FC_FASTCALL_REG+0
	lda #63
	sta <FC_FASTCALL_REG+1
	lda #.LOBYTE(pallet)
	sta <FC_FASTCALL_REG+2
	lda #.HIBYTE(pallet)
	sta <FC_FASTCALL_REG+3
	lda #16
	sta <FC_FASTCALL_REG+4
	jsr _stdio_ppu_put
	lda #0
	sta 0+_stdio_print_addr
	lda #32
	sta 1+_stdio_print_addr
	rts
pallet:
		.byte 15,61,16,48,0,17,33,49,0,18,34,50,0,19,35,51
.endproc
	.global _interrupt
_stdio_interrupt = _interrupt
	.global _interrupt_irq
_stdio_interrupt_irq = _interrupt_irq
