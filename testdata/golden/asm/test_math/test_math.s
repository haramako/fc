	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
__MODULE_TEST_MATH__ = 1
.segment "test_math"
	.include "_unittest.inc"
	.include "_math.inc"
	.export _test_math_test_sin
	;;;=============================
	;;; function _test_math_test_sin
	;;;=============================
.segment "test_math"
.proc _test_math_test_sin
	lda #0
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @1
	lda #255
	jmp @2
@1:
	lda #0
@2:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_4)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_4)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #63
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @3
	lda #255
	jmp @4
@3:
	lda #0
@4:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #127
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_9)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_9)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #64
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @5
	lda #255
	jmp @6
@5:
	lda #0
@6:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #127
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_14)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_14)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #127
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @7
	lda #255
	jmp @8
@7:
	lda #0
@8:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_19)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_19)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #128
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @9
	lda #255
	jmp @10
@9:
	lda #0
@10:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_24)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_24)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #191
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @11
	lda #255
	jmp @12
@11:
	lda #0
@12:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #129
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_29)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_29)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #192
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @13
	lda #255
	jmp @14
@13:
	lda #0
@14:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #129
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_34)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_34)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_sin+0
	sta 0+<F_test_math_test_sin+1
	bpl @15
	lda #255
	jmp @16
@15:
	lda #0
@16:
	sta 1+<F_test_math_test_sin+1
	lda 0+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_sin+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_39)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_39)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_4:
		.byte 115,105,110,40,48,41,0
_9:
		.byte 115,105,110,40,54,51,41,0
_14:
		.byte 115,105,110,40,54,52,41,0
_19:
		.byte 115,105,110,40,49,50,55,41,0
_24:
		.byte 115,105,110,40,49,50,56,41,0
_29:
		.byte 115,105,110,40,49,57,49,41,0
_34:
		.byte 115,105,110,40,49,57,50,41,0
_39:
		.byte 115,105,110,40,50,53,53,41,0
.endproc
	.export _test_math_test_cos
	;;;=============================
	;;; function _test_math_test_cos
	;;;=============================
.segment "test_math"
.proc _test_math_test_cos
	lda #64
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @17
	lda #255
	jmp @18
@17:
	lda #0
@18:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #127
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_44)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_44)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #127
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @19
	lda #255
	jmp @20
@19:
	lda #0
@20:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_49)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_49)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #128
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @21
	lda #255
	jmp @22
@21:
	lda #0
@22:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_54)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_54)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #191
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @23
	lda #255
	jmp @24
@23:
	lda #0
@24:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #129
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_59)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_59)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #192
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @25
	lda #255
	jmp @26
@25:
	lda #0
@26:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #129
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_64)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_64)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @27
	lda #255
	jmp @28
@27:
	lda #0
@28:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_69)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_69)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #0
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @29
	lda #255
	jmp @30
@29:
	lda #0
@30:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_74)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_74)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #63
	sta <F_math_sin+1
	jsr _math_sin
	lda <F_math_sin+0
	sta 0+<F_test_math_test_cos+0
	sta 0+<F_test_math_test_cos+1
	bpl @31
	lda #255
	jmp @32
@31:
	lda #0
@32:
	sta 1+<F_test_math_test_cos+1
	lda 0+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_math_test_cos+1
	sta <F_unittest_assert_equal+1
	lda #127
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_79)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_79)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_44:
		.byte 99,111,115,40,48,41,0
_49:
		.byte 99,111,115,40,54,51,41,0
_54:
		.byte 99,111,115,40,54,52,41,0
_59:
		.byte 99,111,115,40,49,50,55,41,0
_64:
		.byte 99,111,115,40,49,50,56,41,0
_69:
		.byte 99,111,115,40,49,57,49,41,0
_74:
		.byte 99,111,115,40,49,57,50,41,0
_79:
		.byte 99,111,115,40,50,53,53,41,0
.endproc
	.export _test_math_test_atan
	;;;=============================
	;;; function _test_math_test_atan
	;;;=============================
.segment "test_math"
.proc _test_math_test_atan
	lda #0
	sta <F_math_atan+1
	lda #1
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_83)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_83)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #1
	sta <F_math_atan+1
	lda #1
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #32
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_87)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_87)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #1
	sta <F_math_atan+1
	lda #0
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #63
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_91)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_91)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #1
	sta <F_math_atan+1
	lda #255
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #96
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_95)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_95)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #0
	sta <F_math_atan+1
	lda #255
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #128
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_99)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_99)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <F_math_atan+1
	lda #255
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #160
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_103)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_103)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <F_math_atan+1
	lda #0
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #193
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_107)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_107)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <F_math_atan+1
	lda #1
	sta <F_math_atan+2
	jsr _math_atan
	lda <F_math_atan+0
	sta 0+<F_test_math_test_atan+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #224
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_111)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_111)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_83:
		.byte 97,116,97,110,40,48,41,0
_87:
		.byte 97,116,97,110,40,51,50,41,0
_91:
		.byte 97,116,97,110,40,54,51,41,0
_95:
		.byte 97,116,97,110,40,57,54,41,0
_99:
		.byte 97,116,97,110,40,49,50,56,41,0
_103:
		.byte 97,116,97,110,40,49,54,48,41,0
_107:
		.byte 97,116,97,110,40,49,57,51,41,0
_111:
		.byte 97,116,97,110,40,50,50,52,41,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_math"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_114)
	sta <F_stdio_print+0
	lda #.HIBYTE(_114)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_math_test_sin
	lda #.LOBYTE(_117)
	sta <F_stdio_print+0
	lda #.HIBYTE(_117)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_120)
	sta <F_stdio_print+0
	lda #.HIBYTE(_120)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_math_test_cos
	lda #.LOBYTE(_123)
	sta <F_stdio_print+0
	lda #.HIBYTE(_123)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_126)
	sta <F_stdio_print+0
	lda #.HIBYTE(_126)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_math_test_atan
	lda #.LOBYTE(_129)
	sta <F_stdio_print+0
	lda #.HIBYTE(_129)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #0
	sta <F_stdio_exit+0
	jsr _stdio_exit
	rts
_114:
		.byte 116,101,115,116,95,115,105,110,58,0
_117:
		.byte 10,0
_120:
		.byte 116,101,115,116,95,99,111,115,58,0
_123:
		.byte 10,0
_126:
		.byte 116,101,115,116,95,97,116,97,110,58,0
_129:
		.byte 10,0
.endproc
_test_math_main = _main
.segment "CHARS"
	.incbin "character.chr"
