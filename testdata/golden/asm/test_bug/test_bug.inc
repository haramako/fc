.ifndef __MODULE_TEST_BUG__
__MODULE_TEST_BUG__ = 1
	.include "_unittest.inc"
	.include "_math.inc"
	.import _test_bug_hoge
	.import _test_bug_test_pointer_access
	.import _main
_test_bug_main = _main
.endif
