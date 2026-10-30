	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_MEM__ = 1
.segment "mem"
FC_USED_mem_set = 0
FC_USED_mem_zero = 0
FC_USED_mem_copy = 0
FC_USED_mem_compare = 1
	.include "mem.asm"
	.global _mem_compare
