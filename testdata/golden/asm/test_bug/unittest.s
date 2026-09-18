	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_UNITTEST__ = 1
.segment "unittest"
	.include "_stdio.inc"
