.ifndef __MODULE_STDIO__
__MODULE_STDIO__ = 1
	.include "_nes.inc"
	.import _stdio_vsync_flag
	.import _stdio_ppu_addr
	.import _stdio_print_addr
	.import _stdio_print_int16
	.import _stdio_wait_vsync
	.import _stdio_print
	.import _stdio_ppu_put
	.import _stdio_puts
	.import _stdio_exit
	.import _stdio_init
	.import _interrupt
_stdio_interrupt = _interrupt
	.import _interrupt_irq
_stdio_interrupt_irq = _interrupt_irq
.endif
