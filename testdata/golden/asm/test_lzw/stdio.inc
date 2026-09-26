.ifndef __MODULE_STDIO__
__MODULE_STDIO__ = 1
_stdio_EMU_ADDR = 65520
_stdio_EMU_DATA = 65522
_stdio_EMU_PRINT = 65534
_stdio_EMU_EXIT = 65535
	.import _interrupt
	.import _interrupt__direct
_stdio_interrupt = _interrupt
	.import _interrupt_irq
	.import _interrupt_irq__direct
_stdio_interrupt_irq = _interrupt_irq
.endif
