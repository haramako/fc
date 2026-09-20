.ifndef __MODULE_STDIO__
__MODULE_STDIO__ = 1
	.include "_nes.inc"
	.import _stdio_vsync_flag
	.import _stdio_ppu_addr
	.import _stdio_print_addr
	.global _stdio_print_int16
	.global _stdio_wait_vsync
	.global _stdio_print
	.global _stdio_ppu_put
	.import _stdio_exit
	.import _stdio_exit__frame
	.import _stdio_init
	.global _interrupt
_stdio_interrupt = _interrupt
	.global _interrupt_irq
_stdio_interrupt_irq = _interrupt_irq
.endif
