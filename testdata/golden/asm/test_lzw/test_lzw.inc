.ifndef __MODULE_TEST_LZW__
__MODULE_TEST_LZW__ = 1
	.include "_lzw.inc"
	.include "_mem.inc"
	.include "_unittest.inc"
	.include "_stdio.inc"
	.import _test_lzw_test_read_bit
	.import _test_lzw_test_read_bit2
	.import _test_lzw_test_read_vln
	.import _test_lzw_test_read_vln16
	.import _test_lzw_buf
	.import _test_lzw_test_unpack
	.import _main
_test_lzw_main = _main
.endif
