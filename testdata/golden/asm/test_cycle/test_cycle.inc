.ifndef __MODULE_TEST_CYCLE__
__MODULE_TEST_CYCLE__ = 1
	.include "_unittest.inc"
	.include "_cycle_use.inc"
	.import _test_cycle_cycle_var
	.import _test_cycle_test_cycle_use
	.import _main
_test_cycle_main = _main
.endif
