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
