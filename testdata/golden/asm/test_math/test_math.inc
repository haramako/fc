.ifndef __MODULE_TEST_MATH__
__MODULE_TEST_MATH__ = 1
	.include "_unittest.inc"
	.include "_math.inc"
	.import _test_math_test_sin
	.import _test_math_test_cos
	.import _test_math_test_atan
	.import _main
_test_math_main = _main
.endif
