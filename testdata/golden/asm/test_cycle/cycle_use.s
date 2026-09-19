	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_CYCLE_USE__ = 1
.segment "cycle_use"
	.include "_test_cycle.inc"
_cycle_use_CYCLE_CONST = 99
