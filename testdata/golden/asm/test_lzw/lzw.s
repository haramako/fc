	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_LZW__ = 1
.segment "lzw"
	.include "lzw.asm"
_lzw_addr = 126
_lzw_bpos = 125
_lzw_cur = 124
	.export _lzw_rbits
.segment "BSS"
_lzw_rbits: .res 2
	.global _lzw_read_bit
	.global _lzw_read_vln
	.global _lzw_read_vln16
	.global _lzw_unpack
