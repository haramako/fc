	.setcpu "6502"
	.include "macro.inc"
__MODULE_RLE__ = 1
.segment "rle"
	.include "rle.asm"
	.export _rle_unpack
