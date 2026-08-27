.ifndef __MODULE_TEST_BASIC__
__MODULE_TEST_BASIC__ = 1
	.include "_unittest.inc"
	.include "_mem.inc"
	.import _test_basic_hoge
	.import _test_basic_i1
_test_basic_CONST = 2
	.import _test_basic_a1
	.import _test_basic_a2
	.import _test_basic_a3
	.import _test_basic_s1
_test_basic_c = 3
_test_basic_UINT = 1
_test_basic_SINT = -1
_test_basic_UINT16 = 256
_test_basic_SINT16 = -255
	.import _test_basic_ARRAY
	.import _test_basic_test_nesasm_limit
	.import _test_basic_add
	.import _test_basic_fib
	.import _test_basic_test_function
	.import _test_basic_test_misc
	.import _test_basic_add_fastcall
	.import _test_basic_test_fastcall
	.import _main
_test_basic_main = _main
.endif
