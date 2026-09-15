	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
__MODULE_TEST_OP__ = 1
.segment "test_op"
	.include "_unittest.inc"
	.export _test_op_test_const_op
	;;;=============================
	;;; function _test_op_test_const_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_const_op
	lda #1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_2)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_2)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #8
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #8
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_5)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_5)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_8)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_8)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_11)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_11)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_14)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_14)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_17)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_17)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_20)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_20)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_23)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_23)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_26)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_26)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_29)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_29)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_32)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_32)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_35)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_35)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	rts
_2:
		.byte 110,117,109,0
_5:
		.byte 43,44,45,44,42,44,47,44,37,0
_8:
		.byte 60,0
_11:
		.byte 33,44,60,0
_14:
		.byte 62,0
_17:
		.byte 33,44,62,0
_20:
		.byte 60,61,0
_23:
		.byte 33,44,60,61,0
_26:
		.byte 62,61,0
_29:
		.byte 33,44,62,61,0
_32:
		.byte 61,61,0
_35:
		.byte 33,61,0
.endproc
	.export _test_op_test_int_op
	;;;=============================
	;;; function _test_op_test_int_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_int_op
	lda #1
	sta 0+<F_test_op_test_int_op+0
	lda #2
	sta 0+<F_test_op_test_int_op+1
	lda #3
	sta 0+<F_test_op_test_int_op+2
	lda #5
	sta 0+<F_test_op_test_int_op+3
	lda #10
	sta 0+<F_test_op_test_int_op+4
	lda #0
	sta <F_unittest_assert_equal+0
	lda #0
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
	lda 0+<F_test_op_test_int_op+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda 0+<F_test_op_test_int_op+0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_42)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_42)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+1
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+2
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #6
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_46)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_46)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+3
	sta <reg+0+0
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #25
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_50)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_50)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+3
	sta <reg+0+0
	lda #60
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #44
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_54)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_54)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+4
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+2
	sta <reg+2+0
	jsr __div_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_58)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_58)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+4
	sta <reg+2+0
	jsr __div_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #25
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_62)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_62)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+4
	sta <reg+2+0
	jsr __mod_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_66)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_66)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	clc
	lda 0+<F_test_op_test_int_op+4
	adc 0+<F_test_op_test_int_op+4
	sec
	sbc 0+<F_test_op_test_int_op+3
	sta 0+<F_test_op_test_int_op+5
	lda 0+<F_test_op_test_int_op+1
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+5
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta 0+<F_test_op_test_int_op+3
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+2
	sta <reg+2+0
	jsr __div_8
	lda <reg+4+0
	sta 0+<F_test_op_test_int_op+5
	lda #18
	sta <reg+0+0
	lda 0+<F_test_op_test_int_op+5
	sta <reg+2+0
	jsr __mod_8
	lda <reg+4+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #8
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_74)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_74)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+2
	and 0+<F_test_op_test_int_op+1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_78)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_78)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+0
	ora 0+<F_test_op_test_int_op+1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_82)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_82)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+2
	eor 0+<F_test_op_test_int_op+1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_86)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_86)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+0
	cmp 0+<F_test_op_test_int_op+1
	lda #0
	rol a
	eor #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_90)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_90)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+1
	lda #0
	rol a
	eor #1
	beq @7
	lda #0
	sta 0+<F_test_op_test_int_op+3
	jmp @8
@7:
	lda #1
	sta 0+<F_test_op_test_int_op+3
@8:
	lda 0+<F_test_op_test_int_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_95)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_95)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+0
	cmp 0+<F_test_op_test_int_op+1
	lda #0
	rol a
	eor #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_99)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_99)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+1
	lda #0
	rol a
	eor #1
	beq @15
	lda #0
	sta 0+<F_test_op_test_int_op+3
	jmp @16
@15:
	lda #1
	sta 0+<F_test_op_test_int_op+3
@16:
	lda 0+<F_test_op_test_int_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_104)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_104)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+1
	lda #0
	rol a
	eor #1
	beq @20
	lda #0
	sta 0+<F_test_op_test_int_op+3
	jmp @21
@20:
	lda #1
	sta 0+<F_test_op_test_int_op+3
@21:
	lda 0+<F_test_op_test_int_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_109)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_109)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+2
	lda #0
	rol a
	beq @25
	lda #0
	sta 0+<F_test_op_test_int_op+3
	jmp @26
@25:
	lda #1
	sta 0+<F_test_op_test_int_op+3
@26:
	lda 0+<F_test_op_test_int_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_115)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_115)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+2
	cmp 0+<F_test_op_test_int_op+1
	lda #0
	rol a
	eor #1
	beq @30
	lda #0
	sta 0+<F_test_op_test_int_op+3
	jmp @31
@30:
	lda #1
	sta 0+<F_test_op_test_int_op+3
@31:
	lda 0+<F_test_op_test_int_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_120)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_120)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+2
	lda #0
	rol a
	beq @35
	lda #0
	sta 0+<F_test_op_test_int_op+3
	jmp @36
@35:
	lda #1
	sta 0+<F_test_op_test_int_op+3
@36:
	lda 0+<F_test_op_test_int_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_126)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_126)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+1
	bne @37
	lda #1
	jmp @38
@37:
	lda #0
@38:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_130)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_130)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+1
	cmp 0+<F_test_op_test_int_op+2
	beq @43
	lda #0
	jmp @44
@43:
	lda #1
@44:
	beq @41
	lda #0
	sta 0+<F_test_op_test_int_op+1
	jmp @42
@41:
	lda #1
	sta 0+<F_test_op_test_int_op+1
@42:
	lda 0+<F_test_op_test_int_op+1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_135)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_135)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int_op+4
	asl a
	asl a
	asl a
	asl a
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #160
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_139)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_139)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+4
	lsr a
	lsr a
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_143)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_143)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+4
	and #7
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_147)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_147)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_150)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_150)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int_op+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_153)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_153)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	sec
	lda #0
	sbc 0+<F_test_op_test_int_op+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #255
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_157)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_157)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_39:
		.byte 42,48,0
_42:
		.byte 110,117,109,0
_46:
		.byte 50,42,51,0
_50:
		.byte 43,44,45,44,42,44,47,44,37,0
_54:
		.byte 43,44,45,44,42,44,47,44,37,0
_58:
		.byte 47,0
_62:
		.byte 47,0
_66:
		.byte 37,0
_74:
		.byte 43,44,45,44,42,44,47,44,37,0
_78:
		.byte 38,0
_82:
		.byte 124,0
_86:
		.byte 94,0
_90:
		.byte 60,0
_95:
		.byte 33,44,60,0
_99:
		.byte 62,0
_104:
		.byte 33,44,62,0
_109:
		.byte 60,61,0
_115:
		.byte 33,44,60,61,0
_120:
		.byte 62,61,0
_126:
		.byte 33,44,62,61,0
_130:
		.byte 61,61,0
_135:
		.byte 33,61,0
_139:
		.byte 42,99,111,110,115,116,0
_143:
		.byte 47,99,111,110,115,116,0
_147:
		.byte 37,99,111,110,115,116,0
_150:
		.byte 43,49,0
_153:
		.byte 43,105,49,0
_157:
		.byte 45,105,49,0
.endproc
	.export _test_op_test_int8_op
	;;;=============================
	;;; function _test_op_test_int8_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_int8_op
	lda #255
	sta 0+<F_test_op_test_int8_op+0
	lda #254
	sta 0+<F_test_op_test_int8_op+1
	lda #3
	sta 0+<F_test_op_test_int8_op+2
	lda #253
	sta 0+<F_test_op_test_int8_op+3
	lda #251
	sta 0+<F_test_op_test_int8_op+4
	lda #10
	sta 0+<F_test_op_test_int8_op+5
	lda #246
	sta 0+<F_test_op_test_int8_op+6
	bmi @45
	lsr a
	lsr a
	jmp @46
@45:
	lsr a
	lsr a
	ora #192
@46:
	sta 0+<F_test_op_test_int8_op+7
	bpl @47
	lda #255
	jmp @48
@47:
	lda #0
@48:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #253
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_162)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_162)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+6
	asl a
	sta 0+<F_test_op_test_int8_op+7
	bpl @49
	lda #255
	jmp @50
@49:
	lda #0
@50:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #236
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_167)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_167)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @51
	lda #255
	jmp @52
@51:
	lda #0
@52:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda 0+<F_test_op_test_int8_op+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @53
	lda #255
	jmp @54
@53:
	lda #0
@54:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+2
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_172)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_172)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+5
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+2
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @55
	lda #255
	jmp @56
@55:
	lda #0
@56:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_177)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_177)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+6
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+2
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @57
	lda #255
	jmp @58
@57:
	lda #0
@58:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #252
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_182)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_182)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+5
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+3
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @59
	lda #255
	jmp @60
@59:
	lda #0
@60:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #252
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_187)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_187)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+6
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+3
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @61
	lda #255
	jmp @62
@61:
	lda #0
@62:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_192)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_192)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+5
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+1
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @63
	lda #255
	jmp @64
@63:
	lda #0
@64:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #251
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_197)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_197)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+4
	sta <reg+0+0
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+7
	bpl @65
	lda #255
	jmp @66
@65:
	lda #0
@66:
	sta 1+<F_test_op_test_int8_op+7
	lda 0+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+7
	sta <F_unittest_assert_equal+1
	lda #25
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_202)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_202)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+4
	sta <reg+0+0
	lda #60
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+3
	bpl @67
	lda #255
	jmp @68
@67:
	lda #0
@68:
	sta 1+<F_test_op_test_int8_op+3
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+1
	lda #212
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_207)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_207)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+5
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+2
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+3
	bpl @69
	lda #255
	jmp @70
@69:
	lda #0
@70:
	sta 1+<F_test_op_test_int8_op+3
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_212)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_212)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #255
	sta <reg+0+0
	lda 0+<F_test_op_test_int8_op+5
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<F_test_op_test_int8_op+3
	bpl @71
	lda #255
	jmp @72
@71:
	lda #0
@72:
	sta 1+<F_test_op_test_int8_op+3
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+1
	lda #255
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_217)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_217)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+2
	and 0+<F_test_op_test_int8_op+1
	sta 0+<F_test_op_test_int8_op+3
	bpl @73
	lda #255
	jmp @74
@73:
	lda #0
@74:
	sta 1+<F_test_op_test_int8_op+3
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_222)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_222)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+0
	ora 0+<F_test_op_test_int8_op+1
	sta 0+<F_test_op_test_int8_op+3
	bpl @75
	lda #255
	jmp @76
@75:
	lda #0
@76:
	sta 1+<F_test_op_test_int8_op+3
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+1
	lda #255
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_227)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_227)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+2
	eor 0+<F_test_op_test_int8_op+1
	sta 0+<F_test_op_test_int8_op+3
	bpl @77
	lda #255
	jmp @78
@77:
	lda #0
@78:
	sta 1+<F_test_op_test_int8_op+3
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_equal+1
	lda #253
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_232)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_232)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+0
	bvc @81
	eor #$80
@81:
	bmi @79
	lda #0
	beq @80
@79:
	lda #1
@80:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_236)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_236)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc #255
	bvc @84
	eor #$80
@84:
	bmi @82
	lda #0
	beq @83
@82:
	lda #1
@83:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_240)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_240)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #240
	sec
	sbc 0+<F_test_op_test_int8_op+1
	bvc @87
	eor #$80
@87:
	bmi @85
	lda #0
	beq @86
@85:
	lda #1
@86:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_244)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_244)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc #1
	bvc @90
	eor #$80
@90:
	bmi @88
	lda #0
	beq @89
@88:
	lda #1
@89:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_248)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_248)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sec
	sbc 0+<F_test_op_test_int8_op+1
	bvc @93
	eor #$80
@93:
	bmi @96
	lda #0
	jmp @97
@96:
	lda #1
@97:
	beq @94
	lda #0
	sta 0+<F_test_op_test_int8_op+3
	jmp @95
@94:
	lda #1
	sta 0+<F_test_op_test_int8_op+3
@95:
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_253)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_253)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #1
	sec
	sbc 0+<F_test_op_test_int8_op+2
	bvc @100
	eor #$80
@100:
	bmi @98
	lda #0
	beq @99
@98:
	lda #1
@99:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_257)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_257)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+1
	bvc @103
	eor #$80
@103:
	bmi @106
	lda #0
	jmp @107
@106:
	lda #1
@107:
	beq @104
	lda #0
	sta 0+<F_test_op_test_int8_op+3
	jmp @105
@104:
	lda #1
	sta 0+<F_test_op_test_int8_op+3
@105:
	lda 0+<F_test_op_test_int8_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_262)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_262)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+0
	bvc @110
	eor #$80
@110:
	bmi @108
	lda #0
	beq @109
@108:
	lda #1
@109:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_266)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_266)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+1
	bvc @113
	eor #$80
@113:
	bmi @116
	lda #0
	jmp @117
@116:
	lda #1
@117:
	beq @114
	lda #0
	sta 0+<F_test_op_test_int8_op+0
	jmp @115
@114:
	lda #1
	sta 0+<F_test_op_test_int8_op+0
@115:
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_271)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_271)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+1
	bvc @120
	eor #$80
@120:
	bmi @123
	lda #0
	jmp @124
@123:
	lda #1
@124:
	beq @121
	lda #0
	sta 0+<F_test_op_test_int8_op+0
	jmp @122
@121:
	lda #1
	sta 0+<F_test_op_test_int8_op+0
@122:
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_276)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_276)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+2
	bvc @127
	eor #$80
@127:
	bpl @130
	lda #0
	jmp @131
@130:
	lda #1
@131:
	beq @128
	lda #0
	sta 0+<F_test_op_test_int8_op+0
	jmp @129
@128:
	lda #1
	sta 0+<F_test_op_test_int8_op+0
@129:
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_282)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_282)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+2
	sec
	sbc 0+<F_test_op_test_int8_op+1
	bvc @134
	eor #$80
@134:
	bmi @137
	lda #0
	jmp @138
@137:
	lda #1
@138:
	beq @135
	lda #0
	sta 0+<F_test_op_test_int8_op+0
	jmp @136
@135:
	lda #1
	sta 0+<F_test_op_test_int8_op+0
@136:
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_287)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_287)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	sec
	sbc 0+<F_test_op_test_int8_op+2
	bvc @141
	eor #$80
@141:
	bpl @144
	lda #0
	jmp @145
@144:
	lda #1
@145:
	beq @142
	lda #0
	sta 0+<F_test_op_test_int8_op+0
	jmp @143
@142:
	lda #1
	sta 0+<F_test_op_test_int8_op+0
@143:
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_293)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_293)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	cmp 0+<F_test_op_test_int8_op+1
	bne @146
	lda #1
	jmp @147
@146:
	lda #0
@147:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_297)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_297)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+1
	cmp 0+<F_test_op_test_int8_op+2
	beq @152
	lda #0
	jmp @153
@152:
	lda #1
@153:
	beq @150
	lda #0
	sta 0+<F_test_op_test_int8_op+0
	jmp @151
@150:
	lda #1
	sta 0+<F_test_op_test_int8_op+0
@151:
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_302)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_302)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8_op+6
	asl a
	asl a
	asl a
	asl a
	sta 0+<F_test_op_test_int8_op+0
	bpl @154
	lda #255
	jmp @155
@154:
	lda #0
@155:
	sta 1+<F_test_op_test_int8_op+0
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_equal+1
	lda #96
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_307)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_307)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+6
	bmi @156
	lsr a
	lsr a
	jmp @157
@156:
	lsr a
	lsr a
	ora #192
@157:
	sta 0+<F_test_op_test_int8_op+0
	bpl @158
	lda #255
	jmp @159
@158:
	lda #0
@159:
	sta 1+<F_test_op_test_int8_op+0
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_equal+1
	lda #253
	sta <F_unittest_assert_equal+2
	lda #255
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_312)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_312)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8_op+6
	and #7
	sta 0+<F_test_op_test_int8_op+0
	bpl @160
	lda #255
	jmp @161
@160:
	lda #0
@161:
	sta 1+<F_test_op_test_int8_op+0
	lda 0+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8_op+0
	sta <F_unittest_assert_equal+1
	lda #6
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_317)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_317)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_162:
		.byte 45,49,48,47,52,0
_167:
		.byte 45,49,48,42,50,0
_172:
		.byte 110,117,109,0
_177:
		.byte 49,48,47,51,0
_182:
		.byte 49,48,47,45,51,0
_187:
		.byte 45,49,48,47,45,51,0
_192:
		.byte 45,49,48,47,45,51,0
_197:
		.byte 43,44,45,44,42,44,47,44,37,0
_202:
		.byte 43,44,45,44,42,44,47,44,37,0
_207:
		.byte 43,44,45,44,42,44,47,44,37,0
_212:
		.byte 47,0
_217:
		.byte 47,0
_222:
		.byte 38,0
_227:
		.byte 124,0
_232:
		.byte 94,0
_236:
		.byte 60,0
_240:
		.byte 60,0
_244:
		.byte 60,0
_248:
		.byte 60,0
_253:
		.byte 62,0
_257:
		.byte 60,0
_262:
		.byte 33,44,60,0
_266:
		.byte 62,0
_271:
		.byte 33,44,62,0
_276:
		.byte 60,61,0
_282:
		.byte 33,44,60,61,0
_287:
		.byte 62,61,0
_293:
		.byte 33,44,62,61,0
_297:
		.byte 61,61,0
_302:
		.byte 33,61,0
_307:
		.byte 42,99,111,110,115,116,0
_312:
		.byte 47,99,111,110,115,116,0
_317:
		.byte 37,99,111,110,115,116,0
.endproc
	.export _test_op_test_int16_op
	;;;=============================
	;;; function _test_op_test_int16_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_int16_op
	lda #1
	sta 0+<F_test_op_test_int16_op+0
	lda #0
	sta 1+<F_test_op_test_int16_op+0
	lda #2
	sta 0+<F_test_op_test_int16_op+2
	lda #0
	sta 1+<F_test_op_test_int16_op+2
	lda #3
	sta 0+<F_test_op_test_int16_op+4
	lda #0
	sta 1+<F_test_op_test_int16_op+4
	lda #10
	sta 0+<F_test_op_test_int16_op+6
	lda #0
	sta 1+<F_test_op_test_int16_op+6
	lda #16
	sta 0+<F_test_op_test_int16_op+8
	lda #39
	sta 1+<F_test_op_test_int16_op+8
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+1
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+2
	lda 1+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_320)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_320)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+8
	sta <reg+0+0
	lda #10
	sta <reg+2+0
	lda 1+<F_test_op_test_int16_op+8
	sta <reg+0+1
	lda #0
	sta <reg+2+1
	jsr __div_16
	lda <reg+4+0
	sta 0+<F_test_op_test_int16_op+10
	lda <reg+4+1
	sta 1+<F_test_op_test_int16_op+10
	lda 0+<F_test_op_test_int16_op+10
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+10
	sta <F_unittest_assert_equal+1
	lda #232
	sta <F_unittest_assert_equal+2
	lda #3
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_324)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_324)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+4
	and 0+<F_test_op_test_int16_op+2
	sta 0+<F_test_op_test_int16_op+8
	lda 1+<F_test_op_test_int16_op+4
	and 1+<F_test_op_test_int16_op+2
	sta 1+<F_test_op_test_int16_op+8
	lda 0+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_328)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_328)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+0
	ora 0+<F_test_op_test_int16_op+2
	sta 0+<F_test_op_test_int16_op+8
	lda 1+<F_test_op_test_int16_op+0
	ora 1+<F_test_op_test_int16_op+2
	sta 1+<F_test_op_test_int16_op+8
	lda 0+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_332)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_332)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+4
	eor 0+<F_test_op_test_int16_op+2
	sta 0+<F_test_op_test_int16_op+8
	lda 1+<F_test_op_test_int16_op+4
	eor 1+<F_test_op_test_int16_op+2
	sta 1+<F_test_op_test_int16_op+8
	lda 0+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_336)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_336)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+0
	cmp 0+<F_test_op_test_int16_op+2
	lda 1+<F_test_op_test_int16_op+0
	sbc 1+<F_test_op_test_int16_op+2
	lda #0
	rol a
	eor #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_340)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_340)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+2
	lda 1+<F_test_op_test_int16_op+2
	sbc 1+<F_test_op_test_int16_op+2
	lda #0
	rol a
	eor #1
	beq @168
	lda #0
	sta 0+<F_test_op_test_int16_op+8
	jmp @169
@168:
	lda #1
	sta 0+<F_test_op_test_int16_op+8
@169:
	lda 0+<F_test_op_test_int16_op+8
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_345)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_345)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+0
	cmp 0+<F_test_op_test_int16_op+2
	lda 1+<F_test_op_test_int16_op+0
	sbc 1+<F_test_op_test_int16_op+2
	lda #0
	rol a
	eor #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_349)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_349)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+2
	lda 1+<F_test_op_test_int16_op+2
	sbc 1+<F_test_op_test_int16_op+2
	lda #0
	rol a
	eor #1
	beq @176
	lda #0
	sta 0+<F_test_op_test_int16_op+0
	jmp @177
@176:
	lda #1
	sta 0+<F_test_op_test_int16_op+0
@177:
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_354)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_354)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+2
	lda 1+<F_test_op_test_int16_op+2
	sbc 1+<F_test_op_test_int16_op+2
	lda #0
	rol a
	eor #1
	beq @181
	lda #0
	sta 0+<F_test_op_test_int16_op+0
	jmp @182
@181:
	lda #1
	sta 0+<F_test_op_test_int16_op+0
@182:
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_359)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_359)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+4
	lda 1+<F_test_op_test_int16_op+2
	sbc 1+<F_test_op_test_int16_op+4
	lda #0
	rol a
	beq @186
	lda #0
	sta 0+<F_test_op_test_int16_op+0
	jmp @187
@186:
	lda #1
	sta 0+<F_test_op_test_int16_op+0
@187:
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_365)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_365)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+4
	cmp 0+<F_test_op_test_int16_op+2
	lda 1+<F_test_op_test_int16_op+4
	sbc 1+<F_test_op_test_int16_op+2
	lda #0
	rol a
	eor #1
	beq @191
	lda #0
	sta 0+<F_test_op_test_int16_op+0
	jmp @192
@191:
	lda #1
	sta 0+<F_test_op_test_int16_op+0
@192:
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_370)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_370)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+4
	lda 1+<F_test_op_test_int16_op+2
	sbc 1+<F_test_op_test_int16_op+4
	lda #0
	rol a
	beq @196
	lda #0
	sta 0+<F_test_op_test_int16_op+0
	jmp @197
@196:
	lda #1
	sta 0+<F_test_op_test_int16_op+0
@197:
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_376)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_376)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+2
	bne @198
	lda 1+<F_test_op_test_int16_op+2
	cmp 1+<F_test_op_test_int16_op+2
	bne @198
	lda #1
	jmp @199
@198:
	lda #0
@199:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_380)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_380)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+2
	cmp 0+<F_test_op_test_int16_op+4
	bne @200
	lda 1+<F_test_op_test_int16_op+2
	cmp 1+<F_test_op_test_int16_op+4
@200:
	beq @204
	lda #0
	jmp @205
@204:
	lda #1
@205:
	beq @202
	lda #0
	sta 0+<F_test_op_test_int16_op+0
	jmp @203
@202:
	lda #1
	sta 0+<F_test_op_test_int16_op+0
@203:
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_385)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_385)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int16_op+6
	sta 0+<F_test_op_test_int16_op+0
	lda 1+<F_test_op_test_int16_op+6
	sta 1+<F_test_op_test_int16_op+0
	asl 0+<F_test_op_test_int16_op+0
	rol 1+<F_test_op_test_int16_op+0
	asl 0+<F_test_op_test_int16_op+0
	rol 1+<F_test_op_test_int16_op+0
	asl 0+<F_test_op_test_int16_op+0
	rol 1+<F_test_op_test_int16_op+0
	asl 0+<F_test_op_test_int16_op+0
	rol 1+<F_test_op_test_int16_op+0
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+1
	lda #160
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_389)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_389)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+6
	sta 0+<F_test_op_test_int16_op+0
	lda 1+<F_test_op_test_int16_op+6
	sta 1+<F_test_op_test_int16_op+0
	lsr 1+<F_test_op_test_int16_op+0
	ror 0+<F_test_op_test_int16_op+0
	lsr 1+<F_test_op_test_int16_op+0
	ror 0+<F_test_op_test_int16_op+0
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_393)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_393)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int16_op+6
	and #7
	sta 0+<F_test_op_test_int16_op+0
	lda 1+<F_test_op_test_int16_op+6
	and #0
	sta 1+<F_test_op_test_int16_op+0
	lda 0+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int16_op+0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_397)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_397)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_320:
		.byte 110,117,109,0
_324:
		.byte 47,0
_328:
		.byte 38,0
_332:
		.byte 124,0
_336:
		.byte 94,0
_340:
		.byte 60,0
_345:
		.byte 33,44,60,0
_349:
		.byte 62,0
_354:
		.byte 33,44,62,0
_359:
		.byte 60,61,0
_365:
		.byte 33,44,60,61,0
_370:
		.byte 62,61,0
_376:
		.byte 33,44,62,61,0
_380:
		.byte 61,61,0
_385:
		.byte 33,61,0
_389:
		.byte 42,99,111,110,115,116,0
_393:
		.byte 47,99,111,110,115,116,0
_397:
		.byte 37,99,111,110,115,116,0
.endproc
	.export _test_op_test_int8x16_op
	;;;=============================
	;;; function _test_op_test_int8x16_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_int8x16_op
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
	lda #2
	sta 0+<F_test_op_test_int8x16_op+1
	lda #3
	sta 0+<F_test_op_test_int8x16_op+2
	lda #1
	sta 0+<F_test_op_test_int8x16_op+3
	lda #0
	sta 1+<F_test_op_test_int8x16_op+3
	lda #2
	sta 0+<F_test_op_test_int8x16_op+5
	lda #0
	sta 1+<F_test_op_test_int8x16_op+5
	lda #3
	sta 0+<F_test_op_test_int8x16_op+7
	lda #0
	sta 1+<F_test_op_test_int8x16_op+7
	lda 0+<F_test_op_test_int8x16_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_int8x16_op+3
	sta <F_unittest_assert_equal+1
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_400)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_400)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #8
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #8
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_403)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_403)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_int8x16_op+3
	cmp 0+<F_test_op_test_int8x16_op+1
	lda 1+<F_test_op_test_int8x16_op+3
	sbc #0
	lda #0
	rol a
	eor #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_407)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_407)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+5
	cmp 0+<F_test_op_test_int8x16_op+1
	lda 1+<F_test_op_test_int8x16_op+5
	sbc #0
	lda #0
	rol a
	eor #1
	beq @212
	lda #0
	sta 0+<F_test_op_test_int8x16_op+3
	jmp @213
@212:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+3
@213:
	lda 0+<F_test_op_test_int8x16_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_412)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_412)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+0
	cmp 0+<F_test_op_test_int8x16_op+5
	lda #0
	sbc 1+<F_test_op_test_int8x16_op+5
	lda #0
	rol a
	eor #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_416)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_416)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+1
	cmp 0+<F_test_op_test_int8x16_op+5
	lda #0
	sbc 1+<F_test_op_test_int8x16_op+5
	lda #0
	rol a
	eor #1
	beq @220
	lda #0
	sta 0+<F_test_op_test_int8x16_op+0
	jmp @221
@220:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
@221:
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_421)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_421)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+1
	cmp 0+<F_test_op_test_int8x16_op+5
	lda #0
	sbc 1+<F_test_op_test_int8x16_op+5
	lda #0
	rol a
	eor #1
	beq @225
	lda #0
	sta 0+<F_test_op_test_int8x16_op+0
	jmp @226
@225:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
@226:
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_426)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_426)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+1
	cmp 0+<F_test_op_test_int8x16_op+7
	lda #0
	sbc 1+<F_test_op_test_int8x16_op+7
	lda #0
	rol a
	beq @230
	lda #0
	sta 0+<F_test_op_test_int8x16_op+0
	jmp @231
@230:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
@231:
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_432)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_432)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+7
	cmp 0+<F_test_op_test_int8x16_op+1
	lda 1+<F_test_op_test_int8x16_op+7
	sbc #0
	lda #0
	rol a
	eor #1
	beq @235
	lda #0
	sta 0+<F_test_op_test_int8x16_op+0
	jmp @236
@235:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
@236:
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_437)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_437)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+5
	cmp 0+<F_test_op_test_int8x16_op+2
	lda 1+<F_test_op_test_int8x16_op+5
	sbc #0
	lda #0
	rol a
	beq @240
	lda #0
	sta 0+<F_test_op_test_int8x16_op+0
	jmp @241
@240:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
@241:
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_443)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_443)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+5
	cmp 0+<F_test_op_test_int8x16_op+1
	bne @242
	lda 1+<F_test_op_test_int8x16_op+5
	bne @242
	lda #1
	jmp @243
@242:
	lda #0
@243:
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_447)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_447)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda 0+<F_test_op_test_int8x16_op+5
	cmp 0+<F_test_op_test_int8x16_op+2
	bne @244
	lda 1+<F_test_op_test_int8x16_op+5
	cmp #0
@244:
	beq @248
	lda #0
	jmp @249
@248:
	lda #1
@249:
	beq @246
	lda #0
	sta 0+<F_test_op_test_int8x16_op+0
	jmp @247
@246:
	lda #1
	sta 0+<F_test_op_test_int8x16_op+0
@247:
	lda 0+<F_test_op_test_int8x16_op+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_452)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_452)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	rts
_400:
		.byte 110,117,109,0
_403:
		.byte 43,44,45,44,42,44,47,44,37,0
_407:
		.byte 60,0
_412:
		.byte 33,44,60,0
_416:
		.byte 62,0
_421:
		.byte 33,44,62,0
_426:
		.byte 60,61,0
_432:
		.byte 33,44,60,61,0
_437:
		.byte 62,61,0
_443:
		.byte 33,44,62,61,0
_447:
		.byte 61,61,0
_452:
		.byte 33,61,0
.endproc
	.export _test_op_test_logical_op
	;;;=============================
	;;; function _test_op_test_logical_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_logical_op
	lda #1
	sta 0+<F_test_op_test_logical_op+0
	lda #0
	sta 0+<F_test_op_test_logical_op+1
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+0
	beq @end_455
@250:
	lda 0+<F_test_op_test_logical_op+0
	beq @end_455
@251:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_455:
	lda 0+<F_test_op_test_logical_op+2
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_457)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_457)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+0
	beq @end_460
@252:
	lda 0+<F_test_op_test_logical_op+1
	beq @end_460
@253:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_460:
	lda 0+<F_test_op_test_logical_op+2
	beq @254
	lda #0
	sta 0+<F_test_op_test_logical_op+3
	jmp @255
@254:
	lda #1
	sta 0+<F_test_op_test_logical_op+3
@255:
	lda 0+<F_test_op_test_logical_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_463)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_463)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+1
	beq @end_466
@256:
	lda 0+<F_test_op_test_logical_op+0
	beq @end_466
@257:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_466:
	lda 0+<F_test_op_test_logical_op+2
	beq @258
	lda #0
	sta 0+<F_test_op_test_logical_op+3
	jmp @259
@258:
	lda #1
	sta 0+<F_test_op_test_logical_op+3
@259:
	lda 0+<F_test_op_test_logical_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_469)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_469)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+1
	beq @end_472
@260:
	lda 0+<F_test_op_test_logical_op+1
	beq @end_472
@261:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_472:
	lda 0+<F_test_op_test_logical_op+2
	beq @262
	lda #0
	sta 0+<F_test_op_test_logical_op+3
	jmp @263
@262:
	lda #1
	sta 0+<F_test_op_test_logical_op+3
@263:
	lda 0+<F_test_op_test_logical_op+3
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_475)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_475)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+0
	bne @skip_480
	lda 0+<F_test_op_test_logical_op+1
	beq @end_478
@264:
@skip_480:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_478:
	lda 0+<F_test_op_test_logical_op+2
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_481)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_481)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+0
	bne @skip_486
	lda 0+<F_test_op_test_logical_op+1
	beq @end_484
@265:
@skip_486:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_484:
	lda 0+<F_test_op_test_logical_op+2
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_487)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_487)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+2
	lda 0+<F_test_op_test_logical_op+1
	bne @skip_492
	lda 0+<F_test_op_test_logical_op+0
	beq @end_490
@266:
@skip_492:
	lda #1
	sta 0+<F_test_op_test_logical_op+2
@end_490:
	lda 0+<F_test_op_test_logical_op+2
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_493)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_493)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	lda #0
	sta 0+<F_test_op_test_logical_op+0
	lda 0+<F_test_op_test_logical_op+1
	bne @skip_498
	beq @end_496
@267:
@skip_498:
	lda #1
	sta 0+<F_test_op_test_logical_op+0
@end_496:
	lda 0+<F_test_op_test_logical_op+0
	beq @268
	lda #0
	sta 0+<F_test_op_test_logical_op+1
	jmp @269
@268:
	lda #1
	sta 0+<F_test_op_test_logical_op+1
@269:
	lda 0+<F_test_op_test_logical_op+1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_500)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_500)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	rts
_457:
		.byte 84,38,38,84,0
_463:
		.byte 84,38,38,70,0
_469:
		.byte 70,38,38,84,0
_475:
		.byte 70,38,38,70,0
_481:
		.byte 84,124,124,70,0
_487:
		.byte 84,124,124,70,0
_493:
		.byte 70,124,124,84,0
_500:
		.byte 70,124,124,70,0
.endproc
	.export _test_op_test_shift_op
	;;;=============================
	;;; function _test_op_test_shift_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_shift_op
	lda #0
	sta 0+<F_test_op_test_shift_op+0
	lda #1
	sta 0+<F_test_op_test_shift_op+1
	lda #2
	sta 0+<F_test_op_test_shift_op+2
	lda #7
	sta 0+<F_test_op_test_shift_op+3
	lda #20
	sta 0+<F_test_op_test_shift_op+4
	lda #4
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #4
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_503)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_503)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+1
	clc
	rol a
	clc
	rol a
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #4
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_507)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_507)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+0
	tay
	lda 0+<F_test_op_test_shift_op+1
@270:
	cpy #0
	beq @271
	clc
	rol a
	dey
	jmp @270
@271:
	sta 0+<F_test_op_test_shift_op+5
	lda 0+<F_test_op_test_shift_op+5
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_511)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_511)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+2
	tay
	lda 0+<F_test_op_test_shift_op+1
@272:
	cpy #0
	beq @273
	clc
	rol a
	dey
	jmp @272
@273:
	sta 0+<F_test_op_test_shift_op+5
	lda 0+<F_test_op_test_shift_op+5
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #4
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_515)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_515)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+3
	tay
	lda 0+<F_test_op_test_shift_op+1
@274:
	cpy #0
	beq @275
	clc
	rol a
	dey
	jmp @274
@275:
	sta 0+<F_test_op_test_shift_op+5
	lda 0+<F_test_op_test_shift_op+5
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #128
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_519)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_519)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #5
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_522)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_522)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+4
	clc
	ror a
	clc
	ror a
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_526)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_526)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+0
	tay
	lda 0+<F_test_op_test_shift_op+4
@276:
	cpy #0
	beq @277
	clc
	ror a
	dey
	jmp @276
@277:
	sta 0+<F_test_op_test_shift_op+1
	lda 0+<F_test_op_test_shift_op+1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #20
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_530)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_530)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+2
	tay
	lda 0+<F_test_op_test_shift_op+4
@278:
	cpy #0
	beq @279
	clc
	ror a
	dey
	jmp @278
@279:
	sta 0+<F_test_op_test_shift_op+0
	lda 0+<F_test_op_test_shift_op+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_534)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_534)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op+3
	tay
	lda #255
@280:
	cpy #0
	beq @281
	clc
	ror a
	dey
	jmp @280
@281:
	sta 0+<F_test_op_test_shift_op+0
	lda 0+<F_test_op_test_shift_op+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_538)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_538)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_503:
		.byte 60,60,0
_507:
		.byte 60,60,0
_511:
		.byte 60,60,0
_515:
		.byte 60,60,0
_519:
		.byte 60,60,0
_522:
		.byte 62,62,0
_526:
		.byte 62,62,0
_530:
		.byte 60,60,0
_534:
		.byte 62,62,0
_538:
		.byte 62,62,0
.endproc
	.export _test_op_test_shift_op16
	;;;=============================
	;;; function _test_op_test_shift_op16
	;;;=============================
.segment "test_op"
.proc _test_op_test_shift_op16
	lda #1
	sta 0+<F_test_op_test_shift_op16+0
	lda #0
	sta 1+<F_test_op_test_shift_op16+0
	lda #2
	sta 0+<F_test_op_test_shift_op16+2
	lda #0
	sta 1+<F_test_op_test_shift_op16+2
	lda #20
	sta 0+<F_test_op_test_shift_op16+4
	lda #0
	sta 1+<F_test_op_test_shift_op16+4
	lda #4
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #4
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_541)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_541)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op16+0
	sta 0+<F_test_op_test_shift_op16+6
	lda 1+<F_test_op_test_shift_op16+0
	sta 1+<F_test_op_test_shift_op16+6
	asl 0+<F_test_op_test_shift_op16+6
	rol 1+<F_test_op_test_shift_op16+6
	asl 0+<F_test_op_test_shift_op16+6
	rol 1+<F_test_op_test_shift_op16+6
	lda 0+<F_test_op_test_shift_op16+6
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_shift_op16+6
	sta <F_unittest_assert_equal+1
	lda #4
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_545)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_545)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op16+2
	sta 0+<F_test_op_test_shift_op16+0
	lda 1+<F_test_op_test_shift_op16+2
	sta 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	asl 0+<F_test_op_test_shift_op16+0
	rol 1+<F_test_op_test_shift_op16+0
	lda 0+<F_test_op_test_shift_op16+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_shift_op16+0
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #1
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_549)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_549)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #5
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_552)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_552)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_op_test_shift_op16+4
	sta 0+<F_test_op_test_shift_op16+0
	lda 1+<F_test_op_test_shift_op16+4
	sta 1+<F_test_op_test_shift_op16+0
	lsr 1+<F_test_op_test_shift_op16+0
	ror 0+<F_test_op_test_shift_op16+0
	lsr 1+<F_test_op_test_shift_op16+0
	ror 0+<F_test_op_test_shift_op16+0
	lda 0+<F_test_op_test_shift_op16+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_shift_op16+0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_556)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_556)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_541:
		.byte 60,60,0
_545:
		.byte 60,60,0
_549:
		.byte 60,60,0
_552:
		.byte 62,62,0
_556:
		.byte 62,62,119,0
.endproc
	.export _test_op_gi
.segment "BSS"
_test_op_gi: .res 1
	.export _test_op_gw
.segment "BSS"
_test_op_gw: .res 2
	.export _test_op_test_pointer_op
	;;;=============================
	;;; function _test_op_test_pointer_op
	;;;=============================
.segment "test_op"
.proc _test_op_test_pointer_op
	lda #.LOBYTE(_test_op_gi)
	sta 0+<F_test_op_test_pointer_op+3
	lda #.HIBYTE(_test_op_gi)
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta 0+<F_test_op_test_pointer_op+5
	lda 1+<F_test_op_test_pointer_op+3
	sta 1+<F_test_op_test_pointer_op+5
	lda #99
	sta 0+_test_op_gi
	ldy #0
	lda (F_test_op_test_pointer_op+5),y
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #99
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_561)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_561)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #98
	ldy #0
	sta (F_test_op_test_pointer_op+5),y
	lda 0+_test_op_gi
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #98
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_564)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_564)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_op_gw)
	sta 0+<F_test_op_test_pointer_op+3
	lda #.HIBYTE(_test_op_gw)
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta 0+<F_test_op_test_pointer_op+5
	lda 1+<F_test_op_test_pointer_op+3
	sta 1+<F_test_op_test_pointer_op+5
	lda #15
	sta 0+_test_op_gw
	lda #39
	sta 1+_test_op_gw
	ldy #0
	lda (F_test_op_test_pointer_op+5),y
	sta 0+<F_test_op_test_pointer_op+3
	ldy #1
	lda (F_test_op_test_pointer_op+5),y
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_pointer_op+3
	sta <F_unittest_assert_equal+1
	lda #15
	sta <F_unittest_assert_equal+2
	lda #39
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_569)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_569)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #14
	ldy #0
	sta (F_test_op_test_pointer_op+5),y
	lda #39
	ldy #1
	sta (F_test_op_test_pointer_op+5),y
	lda 0+_test_op_gw
	sta <F_unittest_assert_equal+0
	lda 1+_test_op_gw
	sta <F_unittest_assert_equal+1
	lda #14
	sta <F_unittest_assert_equal+2
	lda #39
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_572)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_572)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #99
	sta 0+<F_test_op_test_pointer_op+0
	lda #.LOBYTE(F_test_op_test_pointer_op+0)
	sta 0+<F_test_op_test_pointer_op+3
	lda #.HIBYTE(F_test_op_test_pointer_op+0)
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta 0+<F_test_op_test_pointer_op+5
	lda 1+<F_test_op_test_pointer_op+3
	sta 1+<F_test_op_test_pointer_op+5
	ldy #0
	lda (F_test_op_test_pointer_op+5),y
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #99
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_577)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_577)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #98
	sta 0+<F_test_op_test_pointer_op+0
	ldy #0
	lda (F_test_op_test_pointer_op+5),y
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #98
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_581)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_581)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #15
	sta 0+<F_test_op_test_pointer_op+1
	lda #39
	sta 1+<F_test_op_test_pointer_op+1
	lda #.LOBYTE(F_test_op_test_pointer_op+1)
	sta 0+<F_test_op_test_pointer_op+3
	lda #.HIBYTE(F_test_op_test_pointer_op+1)
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta 0+<F_test_op_test_pointer_op+5
	lda 1+<F_test_op_test_pointer_op+3
	sta 1+<F_test_op_test_pointer_op+5
	ldy #0
	lda (F_test_op_test_pointer_op+5),y
	sta 0+<F_test_op_test_pointer_op+3
	ldy #1
	lda (F_test_op_test_pointer_op+5),y
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_pointer_op+3
	sta <F_unittest_assert_equal+1
	lda #15
	sta <F_unittest_assert_equal+2
	lda #39
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_586)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_586)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #14
	sta 0+<F_test_op_test_pointer_op+1
	lda #39
	sta 1+<F_test_op_test_pointer_op+1
	ldy #0
	lda (F_test_op_test_pointer_op+5),y
	sta 0+<F_test_op_test_pointer_op+3
	ldy #1
	lda (F_test_op_test_pointer_op+5),y
	sta 1+<F_test_op_test_pointer_op+3
	lda 0+<F_test_op_test_pointer_op+3
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_op_test_pointer_op+3
	sta <F_unittest_assert_equal+1
	lda #14
	sta <F_unittest_assert_equal+2
	lda #39
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_590)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_590)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_561:
		.byte 38,120,0
_564:
		.byte 42,112,120,0
_569:
		.byte 38,119,0
_572:
		.byte 42,112,119,0
_577:
		.byte 38,112,105,0
_581:
		.byte 42,112,105,0
_586:
		.byte 38,112,105,0
_590:
		.byte 42,112,105,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_op"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_593)
	sta <F_stdio_print+0
	lda #.HIBYTE(_593)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_const_op
	lda #.LOBYTE(_596)
	sta <F_stdio_print+0
	lda #.HIBYTE(_596)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_599)
	sta <F_stdio_print+0
	lda #.HIBYTE(_599)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_int_op
	lda #.LOBYTE(_602)
	sta <F_stdio_print+0
	lda #.HIBYTE(_602)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_605)
	sta <F_stdio_print+0
	lda #.HIBYTE(_605)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_int8_op
	lda #.LOBYTE(_608)
	sta <F_stdio_print+0
	lda #.HIBYTE(_608)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_611)
	sta <F_stdio_print+0
	lda #.HIBYTE(_611)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_int16_op
	lda #.LOBYTE(_614)
	sta <F_stdio_print+0
	lda #.HIBYTE(_614)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_617)
	sta <F_stdio_print+0
	lda #.HIBYTE(_617)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_int8x16_op
	lda #.LOBYTE(_620)
	sta <F_stdio_print+0
	lda #.HIBYTE(_620)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_623)
	sta <F_stdio_print+0
	lda #.HIBYTE(_623)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_logical_op
	lda #.LOBYTE(_626)
	sta <F_stdio_print+0
	lda #.HIBYTE(_626)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_629)
	sta <F_stdio_print+0
	lda #.HIBYTE(_629)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_shift_op
	lda #.LOBYTE(_632)
	sta <F_stdio_print+0
	lda #.HIBYTE(_632)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_635)
	sta <F_stdio_print+0
	lda #.HIBYTE(_635)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_shift_op16
	lda #.LOBYTE(_638)
	sta <F_stdio_print+0
	lda #.HIBYTE(_638)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_641)
	sta <F_stdio_print+0
	lda #.HIBYTE(_641)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_op_test_pointer_op
	lda #.LOBYTE(_644)
	sta <F_stdio_print+0
	lda #.HIBYTE(_644)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #0
	sta <F_stdio_exit+0
	jsr _stdio_exit
	lda #1
	sta <F_stdio_exit+0
	jsr _stdio_exit
	rts
_593:
		.byte 116,101,115,116,95,99,111,110,115,116,95,111,112,58,0
_596:
		.byte 10,0
_599:
		.byte 116,101,115,116,95,105,110,116,95,111,112,58,0
_602:
		.byte 10,0
_605:
		.byte 116,101,115,116,95,105,110,116,56,95,111,112,58,0
_608:
		.byte 10,0
_611:
		.byte 116,101,115,116,95,105,110,116,49,54,95,111,112,58,0
_614:
		.byte 10,0
_617:
		.byte 116,101,115,116,95,105,110,116,56,120,49,54,95,111,112,58
		.byte 0
_620:
		.byte 10,0
_623:
		.byte 116,101,115,116,95,108,111,103,105,99,97,108,95,111,112,58
		.byte 0
_626:
		.byte 10,0
_629:
		.byte 116,101,115,116,95,115,104,105,102,116,95,111,112,58,0
_632:
		.byte 10,0
_635:
		.byte 116,101,115,116,95,115,104,105,102,116,95,111,112,49,54,58
		.byte 0
_638:
		.byte 10,0
_641:
		.byte 116,101,115,116,95,112,111,105,110,116,101,114,95,111,112,58
		.byte 0
_644:
		.byte 10,0
.endproc
_test_op_main = _main
.segment "CHARS"
	.incbin "character.chr"
