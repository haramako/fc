	.setcpu "6502"
	.include "macro.inc"
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
	lda 0+<FC_FASTCALL_REG+0
	sta 0+_stdio_EMU_DATA
	lda 1+<FC_FASTCALL_REG+0
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
	lda 0+<FC_FASTCALL_REG+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<FC_FASTCALL_REG+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	rts
.endproc
	.export _stdio_puts
	;;;=============================
	;;; function _stdio_puts
	;;;=============================
.segment "stdio"
.proc _stdio_puts
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_168)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_168)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	rts
_168:
		.byte 10,0
.endproc
	.export _stdio_exit
	;;;=============================
	;;; function _stdio_exit
	;;;=============================
.segment "stdio"
.proc _stdio_exit
	lda 0+<FC_FASTCALL_REG+0
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
