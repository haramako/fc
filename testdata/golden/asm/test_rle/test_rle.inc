.ifndef __MODULE_TEST_RLE__
__MODULE_TEST_RLE__ = 1
	.include "_unittest.inc"
	.include "_rle.inc"
	.include "_mem.inc"
	.import _test_rle_src
	.import _test_rle_src2
	.import _test_rle_dest
	.import _test_rle_test_rle
	.import _main
_test_rle_main = _main
.endif
