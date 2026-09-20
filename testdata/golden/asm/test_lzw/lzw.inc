.ifndef __MODULE_LZW__
__MODULE_LZW__ = 1
	.include "_mem.inc"
_lzw_addr = 126
_lzw_bpos = 125
_lzw_cur = 124
	.global _lzw_read_bit
	.global _lzw_read_vln
	.global _lzw_read_vln16
	.global _lzw_unpack
.endif
