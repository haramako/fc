	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_RLE__ = 1
.segment "rle"
	.include "rle.asm"
	.export _rle_unpack
