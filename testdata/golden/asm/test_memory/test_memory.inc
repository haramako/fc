.ifndef __MODULE_TEST_MEMORY__
__MODULE_TEST_MEMORY__ = 1
	.include "_unittest.inc"
	.include "_mem.inc"
	.import _test_memory_str
	.import _test_memory_buf1
	.import _test_memory_buf2
	.import _test_memory_test_copy
	.import _test_memory_test_strcpy
	.import _test_memory_test_set
	.import _main
_test_memory_main = _main
.endif
