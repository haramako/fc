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
	.export _stdio_print_int16
	.export _stdio_wait_vsync
	.export _stdio_print
	.export _stdio_ppu_put
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
	ldx FC_SP
	lda #.LOBYTE(_6)
	sta <S+0,x
	lda #.HIBYTE(_6)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	ldx FC_SP
	lda 0+<F_stdio_exit+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print_int16
	ldx FC_SP
	lda #.LOBYTE(_8)
	sta <S+0,x
	lda #.HIBYTE(_8)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	lda #200
	sta 0+_nes_PPU_CTRL1
@then_12:
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
	jmp @then_12
_6:
		.byte 101,120,105,116,40,0
_8:
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
	ldx FC_SP
	lda #0
	sta <S+0,x
	lda #63
	sta <S+1,x
	lda #.LOBYTE(pallet)
	sta <S+2,x
	lda #.HIBYTE(pallet)
	sta <S+3,x
	lda #16
	sta <S+4,x
	ldx FC_SP
	jsr _stdio_ppu_put
	lda #0
	sta 0+_stdio_print_addr
	lda #32
	sta 1+_stdio_print_addr
	rts
pallet:
		.byte 15,61,16,48,0,17,33,49,0,18,34,50,0,19,35,51
.endproc
	.export _interrupt
_stdio_interrupt = _interrupt
	.export _interrupt_irq
_stdio_interrupt_irq = _interrupt_irq
