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
	.export _interrupt__direct
	;;;=============================
	;;; function _interrupt
	;;;=============================
.segment "stdio"
_interrupt:
.proc _interrupt__direct
	rts
.endproc
_stdio_interrupt = _interrupt
	.export _interrupt_irq
	.export _interrupt_irq__direct
	;;;=============================
	;;; function _interrupt_irq
	;;;=============================
.segment "stdio"
_interrupt_irq:
.proc _interrupt_irq__direct
	rts
.endproc
_stdio_interrupt_irq = _interrupt_irq
