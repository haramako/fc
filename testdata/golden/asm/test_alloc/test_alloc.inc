.ifndef __MODULE_TEST_ALLOC__
__MODULE_TEST_ALLOC__ = 1
	.include "_unittest.inc"
	.import _test_alloc_test_a_alloc
	.import _main
	.import _main__direct
_test_alloc_main = _main
.endif
