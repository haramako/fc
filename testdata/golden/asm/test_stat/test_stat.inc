.ifndef __MODULE_TEST_STAT__
__MODULE_TEST_STAT__ = 1
	.include "_unittest.inc"
_test_stat_I10 = 10
	.import _test_stat_test_if
	.import _test_stat_test_loop
	.import _test_stat_test_for
	.import _test_stat_test_switch
	.import _main
_test_stat_main = _main
.endif
