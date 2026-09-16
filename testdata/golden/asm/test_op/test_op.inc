.ifndef __MODULE_TEST_OP__
__MODULE_TEST_OP__ = 1
	.include "_unittest.inc"
	.import _test_op_test_const_op
	.import _test_op_test_int_op
	.import _test_op_test_int8_op
	.import _test_op_test_int16_op
	.import _test_op_test_int8x16_op
	.import _test_op_test_logical_op
	.import _test_op_test_shift_op
	.import _test_op_test_shift_op16
	.import _test_op_gi
	.import _test_op_gw
	.import _test_op_test_pointer_op
	.import _main
_test_op_main = _main
.endif
