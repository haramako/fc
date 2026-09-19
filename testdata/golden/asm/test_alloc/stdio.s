	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_STDIO__ = 1
.segment "stdio"
_stdio_EMU_ADDR = 65520
_stdio_EMU_DATA = 65522
_stdio_EMU_PRINT = 65534
_stdio_EMU_EXIT = 65535
	.export _stdio_print_int16
	;;;=============================
	;;; function _stdio_print_int16
	;;;=============================
.segment "stdio"
.proc _stdio_print_int16
	lda 0+<F_stdio_print_int16+0
	sta 0+_stdio_EMU_DATA
	lda 1+<F_stdio_print_int16+0
	sta 1+_stdio_EMU_DATA
	lda #2
	sta 0+_stdio_EMU_PRINT
	rts
.endproc
	.export _stdio_print
	;;;=============================
	;;; function _stdio_print
	;;;=============================
.segment "stdio"
.proc _stdio_print
	lda 0+<F_stdio_print+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_stdio_print+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	rts
.endproc
	.export _stdio_exit
	;;;=============================
	;;; function _stdio_exit
	;;;=============================
.segment "stdio"
.proc _stdio_exit
	sta <F_stdio_exit+0
		.export _stdio_exit__frame
_stdio_exit__frame:
	lda 0+<F_stdio_exit+0
	sta 0+_stdio_EMU_EXIT
	rts
.endproc
	.export _stdio_init
	;;;=============================
	;;; function _stdio_init
	;;;=============================
.segment "stdio"
.proc _stdio_init
	rts
.endproc
	.export _interrupt
	;;;=============================
	;;; function _interrupt
	;;;=============================
.segment "stdio"
.proc _interrupt
	rts
.endproc
_stdio_interrupt = _interrupt
	.export _interrupt_irq
	;;;=============================
	;;; function _interrupt_irq
	;;;=============================
.segment "stdio"
.proc _interrupt_irq
	rts
.endproc
_stdio_interrupt_irq = _interrupt_irq
