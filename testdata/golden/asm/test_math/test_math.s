	.setcpu "6502"
	.include "macro.inc"
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
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @1
	lda #255
	jmp @2
@1:
	lda #0
@2:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_7)
	sta <S+4,x
	lda #.HIBYTE(_7)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #63
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @3
	lda #255
	jmp @4
@3:
	lda #0
@4:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #127
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_12)
	sta <S+4,x
	lda #.HIBYTE(_12)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #64
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @5
	lda #255
	jmp @6
@5:
	lda #0
@6:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #127
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_17)
	sta <S+4,x
	lda #.HIBYTE(_17)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #127
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @7
	lda #255
	jmp @8
@7:
	lda #0
@8:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_22)
	sta <S+4,x
	lda #.HIBYTE(_22)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #128
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @9
	lda #255
	jmp @10
@9:
	lda #0
@10:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_27)
	sta <S+4,x
	lda #.HIBYTE(_27)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #191
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @11
	lda #255
	jmp @12
@11:
	lda #0
@12:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #129
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_32)
	sta <S+4,x
	lda #.HIBYTE(_32)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #192
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @13
	lda #255
	jmp @14
@13:
	lda #0
@14:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #129
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_37)
	sta <S+4,x
	lda #.HIBYTE(_37)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @15
	lda #255
	jmp @16
@15:
	lda #0
@16:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_42)
	sta <S+4,x
	lda #.HIBYTE(_42)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_7:
		.byte 115,105,110,40,48,41,0
_12:
		.byte 115,105,110,40,54,51,41,0
_17:
		.byte 115,105,110,40,54,52,41,0
_22:
		.byte 115,105,110,40,49,50,55,41,0
_27:
		.byte 115,105,110,40,49,50,56,41,0
_32:
		.byte 115,105,110,40,49,57,49,41,0
_37:
		.byte 115,105,110,40,49,57,50,41,0
_42:
		.byte 115,105,110,40,50,53,53,41,0
.endproc
	.export _test_math_test_cos
	;;;=============================
	;;; function _test_math_test_cos
	;;;=============================
.segment "test_math"
.proc _test_math_test_cos
	lda #64
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @17
	lda #255
	jmp @18
@17:
	lda #0
@18:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #127
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_47)
	sta <S+4,x
	lda #.HIBYTE(_47)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #127
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @19
	lda #255
	jmp @20
@19:
	lda #0
@20:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_52)
	sta <S+4,x
	lda #.HIBYTE(_52)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #128
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @21
	lda #255
	jmp @22
@21:
	lda #0
@22:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_57)
	sta <S+4,x
	lda #.HIBYTE(_57)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #191
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @23
	lda #255
	jmp @24
@23:
	lda #0
@24:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #129
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_62)
	sta <S+4,x
	lda #.HIBYTE(_62)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #192
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @25
	lda #255
	jmp @26
@25:
	lda #0
@26:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #129
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_67)
	sta <S+4,x
	lda #.HIBYTE(_67)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @27
	lda #255
	jmp @28
@27:
	lda #0
@28:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_72)
	sta <S+4,x
	lda #.HIBYTE(_72)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #0
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @29
	lda #255
	jmp @30
@29:
	lda #0
@30:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_77)
	sta <S+4,x
	lda #.HIBYTE(_77)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #63
	sta <FC_FASTCALL_REG+1
	jsr _math_sin
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	bpl @31
	lda #255
	jmp @32
@31:
	lda #0
@32:
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #127
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_82)
	sta <S+4,x
	lda #.HIBYTE(_82)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_47:
		.byte 99,111,115,40,48,41,0
_52:
		.byte 99,111,115,40,54,51,41,0
_57:
		.byte 99,111,115,40,54,52,41,0
_62:
		.byte 99,111,115,40,49,50,55,41,0
_67:
		.byte 99,111,115,40,49,50,56,41,0
_72:
		.byte 99,111,115,40,49,57,49,41,0
_77:
		.byte 99,111,115,40,49,57,50,41,0
_82:
		.byte 99,111,115,40,50,53,53,41,0
.endproc
	.export _test_math_test_atan
	;;;=============================
	;;; function _test_math_test_atan
	;;;=============================
.segment "test_math"
.proc _test_math_test_atan
	lda #0
	sta <FC_FASTCALL_REG+1
	lda #1
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_86)
	sta <S+4,x
	lda #.HIBYTE(_86)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <FC_FASTCALL_REG+1
	lda #1
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #32
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_90)
	sta <S+4,x
	lda #.HIBYTE(_90)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <FC_FASTCALL_REG+1
	lda #0
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #63
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_94)
	sta <S+4,x
	lda #.HIBYTE(_94)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <FC_FASTCALL_REG+1
	lda #255
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #96
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_98)
	sta <S+4,x
	lda #.HIBYTE(_98)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #0
	sta <FC_FASTCALL_REG+1
	lda #255
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #128
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_102)
	sta <S+4,x
	lda #.HIBYTE(_102)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <FC_FASTCALL_REG+1
	lda #255
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #160
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_106)
	sta <S+4,x
	lda #.HIBYTE(_106)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <FC_FASTCALL_REG+1
	lda #0
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #193
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_110)
	sta <S+4,x
	lda #.HIBYTE(_110)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <FC_FASTCALL_REG+1
	lda #1
	sta <FC_FASTCALL_REG+2
	jsr _math_atan
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #224
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_114)
	sta <S+4,x
	lda #.HIBYTE(_114)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_86:
		.byte 97,116,97,110,40,48,41,0
_90:
		.byte 97,116,97,110,40,51,50,41,0
_94:
		.byte 97,116,97,110,40,54,51,41,0
_98:
		.byte 97,116,97,110,40,57,54,41,0
_102:
		.byte 97,116,97,110,40,49,50,56,41,0
_106:
		.byte 97,116,97,110,40,49,54,48,41,0
_110:
		.byte 97,116,97,110,40,49,57,51,41,0
_114:
		.byte 97,116,97,110,40,50,50,52,41,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_math"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_117)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_117)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_math_test_sin
	lda #.LOBYTE(_120)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_120)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_123)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_123)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_math_test_cos
	lda #.LOBYTE(_126)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_126)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_129)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_129)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_math_test_atan
	lda #.LOBYTE(_132)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_132)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_117:
		.byte 116,101,115,116,95,115,105,110,58,0
_120:
		.byte 10,0
_123:
		.byte 116,101,115,116,95,99,111,115,58,0
_126:
		.byte 10,0
_129:
		.byte 116,101,115,116,95,97,116,97,110,58,0
_132:
		.byte 10,0
.endproc
_test_math_main = _main
.segment "CHARS"
	.incbin "character.chr"
