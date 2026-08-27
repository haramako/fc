	.setcpu "6502"
	.include "macro.inc"
__MODULE_LZW__ = 1
.segment "lzw"
	.include "_mem.inc"
	.include "lzw.asm"
_lzw_addr = 126
_lzw_bpos = 125
_lzw_cur = 124
	.export _lzw_read_bit
	.export _lzw_read_vln
	.export _lzw_read_vln16
	.export _lzw_unpack
