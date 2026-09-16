.ifndef __MODULE_TEST_OPTION__
__MODULE_TEST_OPTION__ = 1
	.include "_unittest.inc"
	.include "_mem.inc"
	.import _test_option_base_data
	.import _test_option_test_address
	.import _main
_test_option_main = _main
.endif
