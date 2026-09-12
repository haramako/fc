.ifndef __MODULE_TEST_VAR__
__MODULE_TEST_VAR__ = 1
	.include "_unittest.inc"
	.import _test_var_array
	.import _test_var_CONST
	.import _test_var__D2
_test_var_add2 = _test_var__D2
	.import _test_var_test_const
	.import _test_var_test_pointer
	.import _test_var_test_array
	.import _test_var_add1
	.import _test_var_mul2
	.import _test_var_FUNC_TABLE
	.import _test_var_test_func_pointer
	.import _test_var_test_escape
	.import _test_var_segmented_function
	.import _test_var_test_segment_option
	.import _main
_test_var_main = _main
	.import _test_var__D96
.endif
