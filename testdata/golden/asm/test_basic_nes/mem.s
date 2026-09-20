	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_MEM__ = 1
.segment "mem"
	.include "mem.asm"
	.global _mem_set
	.global _mem_zero
	.global _mem_copy
	.global _mem_compare
