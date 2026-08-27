	.setcpu "6502"
	.include "macro.inc"
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
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_2)
	sta <S+4,x
	lda #.HIBYTE(_2)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #8
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #8
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_5)
	sta <S+4,x
	lda #.HIBYTE(_5)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_8)
	sta <S+1,x
	lda #.HIBYTE(_8)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_11)
	sta <S+1,x
	lda #.HIBYTE(_11)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_14)
	sta <S+1,x
	lda #.HIBYTE(_14)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_17)
	sta <S+1,x
	lda #.HIBYTE(_17)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_20)
	sta <S+1,x
	lda #.HIBYTE(_20)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_23)
	sta <S+1,x
	lda #.HIBYTE(_23)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_26)
	sta <S+1,x
	lda #.HIBYTE(_26)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_29)
	sta <S+1,x
	lda #.HIBYTE(_29)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_32)
	sta <S+1,x
	lda #.HIBYTE(_32)
	sta <S+2,x
	jsr _unittest_assert_true
	lda #1
	sta <S+0,x
	lda #.LOBYTE(_35)
	sta <S+1,x
	lda #.HIBYTE(_35)
	sta <S+2,x
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
	sta 0+<S+0,x
	lda #2
	sta 0+<S+1,x
	lda #3
	sta 0+<S+2,x
	lda #5
	sta 0+<S+3,x
	lda #10
	sta 0+<S+4,x
	lda #0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #0
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_39)
	sta <S+9,x
	lda #.HIBYTE(_39)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+0,x
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda 0+<S+0,x
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_42)
	sta <S+9,x
	lda #.HIBYTE(_42)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+1,x
	sta <reg+0+0
	lda 0+<S+2,x
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #6
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_46)
	sta <S+9,x
	lda #.HIBYTE(_46)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+3,x
	sta <reg+0+0
	lda 0+<S+3,x
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #25
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_50)
	sta <S+9,x
	lda #.HIBYTE(_50)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+3,x
	sta <reg+0+0
	lda #60
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #44
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_54)
	sta <S+9,x
	lda #.HIBYTE(_54)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+4,x
	sta <reg+0+0
	lda 0+<S+2,x
	sta <reg+2+0
	jsr __div_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #3
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_58)
	sta <S+9,x
	lda #.HIBYTE(_58)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda #255
	sta <reg+0+0
	lda 0+<S+4,x
	sta <reg+2+0
	jsr __div_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #25
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_62)
	sta <S+9,x
	lda #.HIBYTE(_62)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda #255
	sta <reg+0+0
	lda 0+<S+4,x
	sta <reg+2+0
	jsr __mod_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #5
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_66)
	sta <S+9,x
	lda #.HIBYTE(_66)
	sta <S+10,x
	call _unittest_assert_equal, #5
	clc
	lda 0+<S+4,x
	adc 0+<S+4,x
	sec
	sbc 0+<S+3,x
	sta 0+<L+0
	lda 0+<S+1,x
	sta <reg+0+0
	lda 0+<L+0
	sta <reg+2+0
	jsr __mul_8
	lda <reg+4+0
	sta 0+<L+2
	lda 0+<L+2
	sta <reg+0+0
	lda 0+<S+2,x
	sta <reg+2+0
	jsr __div_8
	lda <reg+4+0
	sta 0+<L+0
	lda #18
	sta <reg+0+0
	lda 0+<L+0
	sta <reg+2+0
	jsr __mod_8
	lda <reg+4+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #8
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_74)
	sta <S+9,x
	lda #.HIBYTE(_74)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+2,x
	and 0+<S+1,x
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #2
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_78)
	sta <S+9,x
	lda #.HIBYTE(_78)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+0,x
	ora 0+<S+1,x
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #3
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_82)
	sta <S+9,x
	lda #.HIBYTE(_82)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+2,x
	eor 0+<S+1,x
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #1
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_86)
	sta <S+9,x
	lda #.HIBYTE(_86)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+0,x
	cmp 0+<S+1,x
	bcc @1
	lda #0
	jmp @2
@1:
	lda #1
@2:
	sta <S+5,x
	lda #.LOBYTE(_90)
	sta <S+6,x
	lda #.HIBYTE(_90)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+1,x
	lda #0
	rol a
	eor #1
	beq @5
	lda #0
	sta 0+<L+0
	jmp @6
@5:
	lda #1
	sta 0+<L+0
@6:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_95)
	sta <S+6,x
	lda #.HIBYTE(_95)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+0,x
	cmp 0+<S+1,x
	bcc @7
	lda #0
	jmp @8
@7:
	lda #1
@8:
	sta <S+5,x
	lda #.LOBYTE(_99)
	sta <S+6,x
	lda #.HIBYTE(_99)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+1,x
	lda #0
	rol a
	eor #1
	beq @11
	lda #0
	sta 0+<L+0
	jmp @12
@11:
	lda #1
	sta 0+<L+0
@12:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_104)
	sta <S+6,x
	lda #.HIBYTE(_104)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+1,x
	lda #0
	rol a
	eor #1
	beq @15
	lda #0
	sta 0+<L+0
	jmp @16
@15:
	lda #1
	sta 0+<L+0
@16:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_109)
	sta <S+6,x
	lda #.HIBYTE(_109)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+2,x
	lda #0
	rol a
	beq @19
	lda #0
	sta 0+<L+0
	jmp @20
@19:
	lda #1
	sta 0+<L+0
@20:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_115)
	sta <S+6,x
	lda #.HIBYTE(_115)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+2,x
	cmp 0+<S+1,x
	lda #0
	rol a
	eor #1
	beq @23
	lda #0
	sta 0+<L+0
	jmp @24
@23:
	lda #1
	sta 0+<L+0
@24:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_120)
	sta <S+6,x
	lda #.HIBYTE(_120)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+2,x
	lda #0
	rol a
	beq @27
	lda #0
	sta 0+<L+0
	jmp @28
@27:
	lda #1
	sta 0+<L+0
@28:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_126)
	sta <S+6,x
	lda #.HIBYTE(_126)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+1,x
	bne @29
	lda #1
	jmp @30
@29:
	lda #0
@30:
	sta <S+5,x
	lda #.LOBYTE(_130)
	sta <S+6,x
	lda #.HIBYTE(_130)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+1,x
	cmp 0+<S+2,x
	beq @35
	lda #0
	jmp @36
@35:
	lda #1
@36:
	beq @33
	lda #0
	sta 0+<L+0
	jmp @34
@33:
	lda #1
	sta 0+<L+0
@34:
	lda 0+<L+0
	sta <S+5,x
	lda #.LOBYTE(_135)
	sta <S+6,x
	lda #.HIBYTE(_135)
	sta <S+7,x
	call _unittest_assert_true, #5
	lda 0+<S+4,x
	asl a
	asl a
	asl a
	asl a
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #160
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_139)
	sta <S+9,x
	lda #.HIBYTE(_139)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+4,x
	lsr a
	lsr a
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #2
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_143)
	sta <S+9,x
	lda #.HIBYTE(_143)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+4,x
	and #7
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #2
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_147)
	sta <S+9,x
	lda #.HIBYTE(_147)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda #1
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #1
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_150)
	sta <S+9,x
	lda #.HIBYTE(_150)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+0,x
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #1
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_153)
	sta <S+9,x
	lda #.HIBYTE(_153)
	sta <S+10,x
	call _unittest_assert_equal, #5
	sec
	lda #0
	sbc 0+<S+0,x
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #255
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_157)
	sta <S+9,x
	lda #.HIBYTE(_157)
	sta <S+10,x
	call _unittest_assert_equal, #5
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
	sta 0+<S+0,x
	lda #254
	sta 0+<S+1,x
	lda #3
	sta 0+<S+2,x
	lda #253
	sta 0+<S+3,x
	lda #251
	sta 0+<S+4,x
	lda #10
	sta 0+<S+5,x
	lda #246
	sta 0+<S+6,x
	lda 0+<S+6,x
	bmi @37
	lsr a
	lsr a
	jmp @38
@37:
	lsr a
	lsr a
	ora #192
@38:
	sta 0+<L+0
	bpl @39
	lda #255
	jmp @40
@39:
	lda #0
@40:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #253
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_162)
	sta <S+11,x
	lda #.HIBYTE(_162)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+6,x
	asl a
	sta 0+<L+0
	bpl @41
	lda #255
	jmp @42
@41:
	lda #0
@42:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #236
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_167)
	sta <S+11,x
	lda #.HIBYTE(_167)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+0,x
	sta 0+<L+0
	bpl @43
	lda #255
	jmp @44
@43:
	lda #0
@44:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda 0+<S+0,x
	sta 0+<L+0
	bpl @45
	lda #255
	jmp @46
@45:
	lda #0
@46:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+9,x
	lda 1+<L+0
	sta <S+10,x
	lda #.LOBYTE(_172)
	sta <S+11,x
	lda #.HIBYTE(_172)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+5,x
	sta <reg+0+0
	lda 0+<S+2,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @47
	lda #255
	jmp @48
@47:
	lda #0
@48:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #3
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_177)
	sta <S+11,x
	lda #.HIBYTE(_177)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+6,x
	sta <reg+0+0
	lda 0+<S+2,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @49
	lda #255
	jmp @50
@49:
	lda #0
@50:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #252
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_182)
	sta <S+11,x
	lda #.HIBYTE(_182)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+5,x
	sta <reg+0+0
	lda 0+<S+3,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @51
	lda #255
	jmp @52
@51:
	lda #0
@52:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #252
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_187)
	sta <S+11,x
	lda #.HIBYTE(_187)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+6,x
	sta <reg+0+0
	lda 0+<S+3,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @53
	lda #255
	jmp @54
@53:
	lda #0
@54:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #3
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_192)
	sta <S+11,x
	lda #.HIBYTE(_192)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+5,x
	sta <reg+0+0
	lda 0+<S+1,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @55
	lda #255
	jmp @56
@55:
	lda #0
@56:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #251
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_197)
	sta <S+11,x
	lda #.HIBYTE(_197)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+4,x
	sta <reg+0+0
	lda 0+<S+4,x
	sta <reg+2+0
	jsr __mul_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @57
	lda #255
	jmp @58
@57:
	lda #0
@58:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #25
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_202)
	sta <S+11,x
	lda #.HIBYTE(_202)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+4,x
	sta <reg+0+0
	lda #60
	sta <reg+2+0
	jsr __mul_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @59
	lda #255
	jmp @60
@59:
	lda #0
@60:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #212
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_207)
	sta <S+11,x
	lda #.HIBYTE(_207)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+5,x
	sta <reg+0+0
	lda 0+<S+2,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @61
	lda #255
	jmp @62
@61:
	lda #0
@62:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #3
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_212)
	sta <S+11,x
	lda #.HIBYTE(_212)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda #255
	sta <reg+0+0
	lda 0+<S+5,x
	sta <reg+2+0
	jsr __div_8s
	lda <reg+4+0
	sta 0+<L+0
	bpl @63
	lda #255
	jmp @64
@63:
	lda #0
@64:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #255
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_217)
	sta <S+11,x
	lda #.HIBYTE(_217)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+2,x
	and 0+<S+1,x
	sta 0+<L+0
	bpl @65
	lda #255
	jmp @66
@65:
	lda #0
@66:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #2
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_222)
	sta <S+11,x
	lda #.HIBYTE(_222)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+0,x
	ora 0+<S+1,x
	sta 0+<L+0
	bpl @67
	lda #255
	jmp @68
@67:
	lda #0
@68:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #255
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_227)
	sta <S+11,x
	lda #.HIBYTE(_227)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+2,x
	eor 0+<S+1,x
	sta 0+<L+0
	bpl @69
	lda #255
	jmp @70
@69:
	lda #0
@70:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #253
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_232)
	sta <S+11,x
	lda #.HIBYTE(_232)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+1,x
	cmp 0+<S+0,x
	bmi @71
	lda #0
	jmp @72
@71:
	lda #1
@72:
	sta <S+7,x
	lda #.LOBYTE(_236)
	sta <S+8,x
	lda #.HIBYTE(_236)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp #255
	bmi @73
	lda #0
	jmp @74
@73:
	lda #1
@74:
	sta <S+7,x
	lda #.LOBYTE(_240)
	sta <S+8,x
	lda #.HIBYTE(_240)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda #240
	cmp 0+<S+1,x
	bmi @75
	lda #0
	jmp @76
@75:
	lda #1
@76:
	sta <S+7,x
	lda #.LOBYTE(_244)
	sta <S+8,x
	lda #.HIBYTE(_244)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp #1
	bmi @77
	lda #0
	jmp @78
@77:
	lda #1
@78:
	sta <S+7,x
	lda #.LOBYTE(_248)
	sta <S+8,x
	lda #.HIBYTE(_248)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda #1
	cmp 0+<S+1,x
	bmi @83
	lda #0
	jmp @84
@83:
	lda #1
@84:
	beq @81
	lda #0
	sta 0+<L+0
	jmp @82
@81:
	lda #1
	sta 0+<L+0
@82:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_253)
	sta <S+8,x
	lda #.HIBYTE(_253)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda #1
	cmp 0+<S+2,x
	bcc @85
	lda #0
	jmp @86
@85:
	lda #1
@86:
	sta <S+7,x
	lda #.LOBYTE(_257)
	sta <S+8,x
	lda #.HIBYTE(_257)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+1,x
	bmi @91
	lda #0
	jmp @92
@91:
	lda #1
@92:
	beq @89
	lda #0
	sta 0+<L+0
	jmp @90
@89:
	lda #1
	sta 0+<L+0
@90:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_262)
	sta <S+8,x
	lda #.HIBYTE(_262)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+0,x
	bmi @93
	lda #0
	jmp @94
@93:
	lda #1
@94:
	sta <S+7,x
	lda #.LOBYTE(_266)
	sta <S+8,x
	lda #.HIBYTE(_266)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+1,x
	bmi @99
	lda #0
	jmp @100
@99:
	lda #1
@100:
	beq @97
	lda #0
	sta 0+<L+0
	jmp @98
@97:
	lda #1
	sta 0+<L+0
@98:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_271)
	sta <S+8,x
	lda #.HIBYTE(_271)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+1,x
	bmi @105
	lda #0
	jmp @106
@105:
	lda #1
@106:
	beq @103
	lda #0
	sta 0+<L+0
	jmp @104
@103:
	lda #1
	sta 0+<L+0
@104:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_276)
	sta <S+8,x
	lda #.HIBYTE(_276)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+2,x
	bpl @111
	lda #0
	jmp @112
@111:
	lda #1
@112:
	beq @109
	lda #0
	sta 0+<L+0
	jmp @110
@109:
	lda #1
	sta 0+<L+0
@110:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_282)
	sta <S+8,x
	lda #.HIBYTE(_282)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+2,x
	cmp 0+<S+1,x
	bmi @117
	lda #0
	jmp @118
@117:
	lda #1
@118:
	beq @115
	lda #0
	sta 0+<L+0
	jmp @116
@115:
	lda #1
	sta 0+<L+0
@116:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_287)
	sta <S+8,x
	lda #.HIBYTE(_287)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+2,x
	bpl @123
	lda #0
	jmp @124
@123:
	lda #1
@124:
	beq @121
	lda #0
	sta 0+<L+0
	jmp @122
@121:
	lda #1
	sta 0+<L+0
@122:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_293)
	sta <S+8,x
	lda #.HIBYTE(_293)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+1,x
	bne @125
	lda #1
	jmp @126
@125:
	lda #0
@126:
	sta <S+7,x
	lda #.LOBYTE(_297)
	sta <S+8,x
	lda #.HIBYTE(_297)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+1,x
	cmp 0+<S+2,x
	beq @131
	lda #0
	jmp @132
@131:
	lda #1
@132:
	beq @129
	lda #0
	sta 0+<L+0
	jmp @130
@129:
	lda #1
	sta 0+<L+0
@130:
	lda 0+<L+0
	sta <S+7,x
	lda #.LOBYTE(_302)
	sta <S+8,x
	lda #.HIBYTE(_302)
	sta <S+9,x
	call _unittest_assert_true, #7
	lda 0+<S+6,x
	asl a
	asl a
	asl a
	asl a
	sta 0+<L+0
	bpl @133
	lda #255
	jmp @134
@133:
	lda #0
@134:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #96
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_307)
	sta <S+11,x
	lda #.HIBYTE(_307)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+6,x
	bmi @135
	lsr a
	lsr a
	jmp @136
@135:
	lsr a
	lsr a
	ora #192
@136:
	sta 0+<L+0
	bpl @137
	lda #255
	jmp @138
@137:
	lda #0
@138:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #253
	sta <S+9,x
	lda #255
	sta <S+10,x
	lda #.LOBYTE(_312)
	sta <S+11,x
	lda #.HIBYTE(_312)
	sta <S+12,x
	call _unittest_assert_equal, #7
	lda 0+<S+6,x
	and #7
	sta 0+<L+0
	bpl @139
	lda #255
	jmp @140
@139:
	lda #0
@140:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+7,x
	lda 1+<L+0
	sta <S+8,x
	lda #6
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #.LOBYTE(_317)
	sta <S+11,x
	lda #.HIBYTE(_317)
	sta <S+12,x
	call _unittest_assert_equal, #7
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
	sta 0+<S+0,x
	lda #0
	sta 1+<S+0,x
	lda #2
	sta 0+<S+2,x
	lda #0
	sta 1+<S+2,x
	lda #3
	sta 0+<S+4,x
	lda #0
	sta 1+<S+4,x
	lda #10
	sta 0+<S+6,x
	lda #0
	sta 1+<S+6,x
	lda #16
	sta 0+<S+8,x
	lda #39
	sta 1+<S+8,x
	lda 0+<S+0,x
	sta <S+10,x
	lda 1+<S+0,x
	sta <S+11,x
	lda 0+<S+0,x
	sta <S+12,x
	lda 1+<S+0,x
	sta <S+13,x
	lda #.LOBYTE(_320)
	sta <S+14,x
	lda #.HIBYTE(_320)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 0+<S+8,x
	sta <reg+0+0
	lda #10
	sta <reg+2+0
	lda 1+<S+8,x
	sta <reg+0+1
	lda #0
	sta <reg+2+1
	jsr __div_16
	lda <reg+4+0
	sta 0+<L+0
	lda <reg+4+1
	sta 1+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #232
	sta <S+12,x
	lda #3
	sta <S+13,x
	lda #.LOBYTE(_324)
	sta <S+14,x
	lda #.HIBYTE(_324)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 0+<S+4,x
	and 0+<S+2,x
	sta 0+<L+0
	lda 1+<S+4,x
	and 1+<S+2,x
	sta 1+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #2
	sta <S+12,x
	lda #0
	sta <S+13,x
	lda #.LOBYTE(_328)
	sta <S+14,x
	lda #.HIBYTE(_328)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 0+<S+0,x
	ora 0+<S+2,x
	sta 0+<L+0
	lda 1+<S+0,x
	ora 1+<S+2,x
	sta 1+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #3
	sta <S+12,x
	lda #0
	sta <S+13,x
	lda #.LOBYTE(_332)
	sta <S+14,x
	lda #.HIBYTE(_332)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 0+<S+4,x
	eor 0+<S+2,x
	sta 0+<L+0
	lda 1+<S+4,x
	eor 1+<S+2,x
	sta 1+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #1
	sta <S+12,x
	lda #0
	sta <S+13,x
	lda #.LOBYTE(_336)
	sta <S+14,x
	lda #.HIBYTE(_336)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 1+<S+0,x
	cmp 1+<S+2,x
	bcc @141
	lda 0+<S+0,x
	cmp 0+<S+2,x
	bcc @141
	lda #0
	jmp @142
@141:
	lda #1
@142:
	sta <S+10,x
	lda #.LOBYTE(_340)
	sta <S+11,x
	lda #.HIBYTE(_340)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+2,x
	cmp 1+<S+2,x
	bcc @143
	lda 0+<S+2,x
	cmp 0+<S+2,x
@143:
	lda #0
	rol a
	eor #1
	beq @145
	lda #0
	sta 0+<L+0
	jmp @146
@145:
	lda #1
	sta 0+<L+0
@146:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_345)
	sta <S+11,x
	lda #.HIBYTE(_345)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+0,x
	cmp 1+<S+2,x
	bcc @147
	lda 0+<S+0,x
	cmp 0+<S+2,x
	bcc @147
	lda #0
	jmp @148
@147:
	lda #1
@148:
	sta <S+10,x
	lda #.LOBYTE(_349)
	sta <S+11,x
	lda #.HIBYTE(_349)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+2,x
	cmp 1+<S+2,x
	bcc @149
	lda 0+<S+2,x
	cmp 0+<S+2,x
@149:
	lda #0
	rol a
	eor #1
	beq @151
	lda #0
	sta 0+<L+0
	jmp @152
@151:
	lda #1
	sta 0+<L+0
@152:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_354)
	sta <S+11,x
	lda #.HIBYTE(_354)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+2,x
	cmp 1+<S+2,x
	bcc @153
	lda 0+<S+2,x
	cmp 0+<S+2,x
@153:
	lda #0
	rol a
	eor #1
	beq @155
	lda #0
	sta 0+<L+0
	jmp @156
@155:
	lda #1
	sta 0+<L+0
@156:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_359)
	sta <S+11,x
	lda #.HIBYTE(_359)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+2,x
	cmp 1+<S+4,x
	bcc @157
	lda 0+<S+2,x
	cmp 0+<S+4,x
@157:
	lda #0
	rol a
	beq @159
	lda #0
	sta 0+<L+0
	jmp @160
@159:
	lda #1
	sta 0+<L+0
@160:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_365)
	sta <S+11,x
	lda #.HIBYTE(_365)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+4,x
	cmp 1+<S+2,x
	bcc @161
	lda 0+<S+4,x
	cmp 0+<S+2,x
@161:
	lda #0
	rol a
	eor #1
	beq @163
	lda #0
	sta 0+<L+0
	jmp @164
@163:
	lda #1
	sta 0+<L+0
@164:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_370)
	sta <S+11,x
	lda #.HIBYTE(_370)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 1+<S+2,x
	cmp 1+<S+4,x
	bcc @165
	lda 0+<S+2,x
	cmp 0+<S+4,x
@165:
	lda #0
	rol a
	beq @167
	lda #0
	sta 0+<L+0
	jmp @168
@167:
	lda #1
	sta 0+<L+0
@168:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_376)
	sta <S+11,x
	lda #.HIBYTE(_376)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 0+<S+2,x
	cmp 0+<S+2,x
	bne @169
	lda 1+<S+2,x
	cmp 1+<S+2,x
	bne @169
	lda #1
	jmp @170
@169:
	lda #0
@170:
	sta <S+10,x
	lda #.LOBYTE(_380)
	sta <S+11,x
	lda #.HIBYTE(_380)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 0+<S+2,x
	cmp 0+<S+4,x
	bne @171
	lda 1+<S+2,x
	cmp 1+<S+4,x
@171:
	beq @175
	lda #0
	jmp @176
@175:
	lda #1
@176:
	beq @173
	lda #0
	sta 0+<L+0
	jmp @174
@173:
	lda #1
	sta 0+<L+0
@174:
	lda 0+<L+0
	sta <S+10,x
	lda #.LOBYTE(_385)
	sta <S+11,x
	lda #.HIBYTE(_385)
	sta <S+12,x
	call _unittest_assert_true, #10
	lda 0+<S+6,x
	sta 0+<L+0
	lda 1+<S+6,x
	sta 1+<L+0
	asl 0+<L+0
	rol 1+<L+0
	asl 0+<L+0
	rol 1+<L+0
	asl 0+<L+0
	rol 1+<L+0
	asl 0+<L+0
	rol 1+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #160
	sta <S+12,x
	lda #0
	sta <S+13,x
	lda #.LOBYTE(_389)
	sta <S+14,x
	lda #.HIBYTE(_389)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 0+<S+6,x
	sta 0+<L+0
	lda 1+<S+6,x
	sta 1+<L+0
	lsr 1+<L+0
	ror 0+<L+0
	lsr 1+<L+0
	ror 0+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #2
	sta <S+12,x
	lda #0
	sta <S+13,x
	lda #.LOBYTE(_393)
	sta <S+14,x
	lda #.HIBYTE(_393)
	sta <S+15,x
	call _unittest_assert_equal, #10
	lda 0+<S+6,x
	and #7
	sta 0+<L+0
	lda 1+<S+6,x
	and #0
	sta 1+<L+0
	lda 0+<L+0
	sta <S+10,x
	lda 1+<L+0
	sta <S+11,x
	lda #2
	sta <S+12,x
	lda #0
	sta <S+13,x
	lda #.LOBYTE(_397)
	sta <S+14,x
	lda #.HIBYTE(_397)
	sta <S+15,x
	call _unittest_assert_equal, #10
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
	sta 0+<S+0,x
	lda #2
	sta 0+<S+1,x
	lda #3
	sta 0+<S+2,x
	lda #1
	sta 0+<S+3,x
	lda #0
	sta 1+<S+3,x
	lda #2
	sta 0+<S+5,x
	lda #0
	sta 1+<S+5,x
	lda #3
	sta 0+<S+7,x
	lda #0
	sta 1+<S+7,x
	lda 0+<S+3,x
	sta <S+9,x
	lda 1+<S+3,x
	sta <S+10,x
	lda 0+<S+0,x
	sta <S+11,x
	lda #0
	sta <S+12,x
	lda #.LOBYTE(_400)
	sta <S+13,x
	lda #.HIBYTE(_400)
	sta <S+14,x
	call _unittest_assert_equal, #9
	lda #8
	sta <S+9,x
	lda #0
	sta <S+10,x
	lda #8
	sta <S+11,x
	lda #0
	sta <S+12,x
	lda #.LOBYTE(_403)
	sta <S+13,x
	lda #.HIBYTE(_403)
	sta <S+14,x
	call _unittest_assert_equal, #9
	lda 1+<S+3,x
	cmp #0
	bcc @177
	lda 0+<S+3,x
	cmp 0+<S+1,x
	bcc @177
	lda #0
	jmp @178
@177:
	lda #1
@178:
	sta <S+9,x
	lda #.LOBYTE(_407)
	sta <S+10,x
	lda #.HIBYTE(_407)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda 1+<S+5,x
	cmp #0
	bcc @179
	lda 0+<S+5,x
	cmp 0+<S+1,x
@179:
	lda #0
	rol a
	eor #1
	beq @181
	lda #0
	sta 0+<L+0
	jmp @182
@181:
	lda #1
	sta 0+<L+0
@182:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_412)
	sta <S+10,x
	lda #.HIBYTE(_412)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda #0
	cmp 1+<S+5,x
	bcc @183
	lda 0+<S+0,x
	cmp 0+<S+5,x
	bcc @183
	lda #0
	jmp @184
@183:
	lda #1
@184:
	sta <S+9,x
	lda #.LOBYTE(_416)
	sta <S+10,x
	lda #.HIBYTE(_416)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda #0
	cmp 1+<S+5,x
	bcc @185
	lda 0+<S+1,x
	cmp 0+<S+5,x
@185:
	lda #0
	rol a
	eor #1
	beq @187
	lda #0
	sta 0+<L+0
	jmp @188
@187:
	lda #1
	sta 0+<L+0
@188:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_421)
	sta <S+10,x
	lda #.HIBYTE(_421)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda #0
	cmp 1+<S+5,x
	bcc @189
	lda 0+<S+1,x
	cmp 0+<S+5,x
@189:
	lda #0
	rol a
	eor #1
	beq @191
	lda #0
	sta 0+<L+0
	jmp @192
@191:
	lda #1
	sta 0+<L+0
@192:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_426)
	sta <S+10,x
	lda #.HIBYTE(_426)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda #0
	cmp 1+<S+7,x
	bcc @193
	lda 0+<S+1,x
	cmp 0+<S+7,x
@193:
	lda #0
	rol a
	beq @195
	lda #0
	sta 0+<L+0
	jmp @196
@195:
	lda #1
	sta 0+<L+0
@196:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_432)
	sta <S+10,x
	lda #.HIBYTE(_432)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda 1+<S+7,x
	cmp #0
	bcc @197
	lda 0+<S+7,x
	cmp 0+<S+1,x
@197:
	lda #0
	rol a
	eor #1
	beq @199
	lda #0
	sta 0+<L+0
	jmp @200
@199:
	lda #1
	sta 0+<L+0
@200:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_437)
	sta <S+10,x
	lda #.HIBYTE(_437)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda 1+<S+5,x
	cmp #0
	bcc @201
	lda 0+<S+5,x
	cmp 0+<S+2,x
@201:
	lda #0
	rol a
	beq @203
	lda #0
	sta 0+<L+0
	jmp @204
@203:
	lda #1
	sta 0+<L+0
@204:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_443)
	sta <S+10,x
	lda #.HIBYTE(_443)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda 0+<S+5,x
	cmp 0+<S+1,x
	bne @205
	lda 1+<S+5,x
	cmp #0
	bne @205
	lda #1
	jmp @206
@205:
	lda #0
@206:
	sta <S+9,x
	lda #.LOBYTE(_447)
	sta <S+10,x
	lda #.HIBYTE(_447)
	sta <S+11,x
	call _unittest_assert_true, #9
	lda 0+<S+5,x
	cmp 0+<S+2,x
	bne @207
	lda 1+<S+5,x
	cmp #0
@207:
	beq @211
	lda #0
	jmp @212
@211:
	lda #1
@212:
	beq @209
	lda #0
	sta 0+<L+0
	jmp @210
@209:
	lda #1
	sta 0+<L+0
@210:
	lda 0+<L+0
	sta <S+9,x
	lda #.LOBYTE(_452)
	sta <S+10,x
	lda #.HIBYTE(_452)
	sta <S+11,x
	call _unittest_assert_true, #9
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
	sta 0+<S+0,x
	lda #0
	sta 0+<S+1,x
	lda 0+<S+0,x
	sta 0+<L+0
	lda 0+<L+0
	beq @end_455
@213:
	lda 0+<S+0,x
	sta 0+<L+0
@end_455:
	lda 0+<L+0
	sta <S+2,x
	lda #.LOBYTE(_457)
	sta <S+3,x
	lda #.HIBYTE(_457)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+0,x
	sta 0+<L+0
	lda 0+<L+0
	beq @end_460
@214:
	lda 0+<S+1,x
	sta 0+<L+0
@end_460:
	lda 0+<L+0
	beq @215
	lda #0
	sta 0+<L+2
	jmp @216
@215:
	lda #1
	sta 0+<L+2
@216:
	lda 0+<L+2
	sta <S+2,x
	lda #.LOBYTE(_463)
	sta <S+3,x
	lda #.HIBYTE(_463)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+1,x
	sta 0+<L+0
	lda 0+<L+0
	beq @end_466
@217:
	lda 0+<S+0,x
	sta 0+<L+0
@end_466:
	lda 0+<L+0
	beq @218
	lda #0
	sta 0+<L+2
	jmp @219
@218:
	lda #1
	sta 0+<L+2
@219:
	lda 0+<L+2
	sta <S+2,x
	lda #.LOBYTE(_469)
	sta <S+3,x
	lda #.HIBYTE(_469)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+1,x
	sta 0+<L+0
	lda 0+<L+0
	beq @end_472
@220:
	lda 0+<S+1,x
	sta 0+<L+0
@end_472:
	lda 0+<L+0
	beq @221
	lda #0
	sta 0+<L+2
	jmp @222
@221:
	lda #1
	sta 0+<L+2
@222:
	lda 0+<L+2
	sta <S+2,x
	lda #.LOBYTE(_475)
	sta <S+3,x
	lda #.HIBYTE(_475)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+0,x
	sta 0+<L+0
	lda 0+<L+0
	beq @223
	lda #0
	sta 0+<L+2
	jmp @224
@223:
	lda #1
	sta 0+<L+2
@224:
	lda 0+<L+2
	beq @end_478
@225:
	lda 0+<S+1,x
	sta 0+<L+0
@end_478:
	lda 0+<L+0
	sta <S+2,x
	lda #.LOBYTE(_481)
	sta <S+3,x
	lda #.HIBYTE(_481)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+0,x
	sta 0+<L+0
	lda 0+<L+0
	beq @226
	lda #0
	sta 0+<L+2
	jmp @227
@226:
	lda #1
	sta 0+<L+2
@227:
	lda 0+<L+2
	beq @end_484
@228:
	lda 0+<S+1,x
	sta 0+<L+0
@end_484:
	lda 0+<L+0
	sta <S+2,x
	lda #.LOBYTE(_487)
	sta <S+3,x
	lda #.HIBYTE(_487)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+1,x
	sta 0+<L+0
	lda 0+<L+0
	beq @229
	lda #0
	sta 0+<L+2
	jmp @230
@229:
	lda #1
	sta 0+<L+2
@230:
	lda 0+<L+2
	beq @end_490
@231:
	lda 0+<S+0,x
	sta 0+<L+0
@end_490:
	lda 0+<L+0
	sta <S+2,x
	lda #.LOBYTE(_493)
	sta <S+3,x
	lda #.HIBYTE(_493)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda 0+<S+1,x
	sta 0+<L+0
	lda 0+<L+0
	beq @232
	lda #0
	sta 0+<L+2
	jmp @233
@232:
	lda #1
	sta 0+<L+2
@233:
	lda 0+<L+2
	beq @end_496
@234:
	lda 0+<S+1,x
	sta 0+<L+0
@end_496:
	lda 0+<L+0
	beq @235
	lda #0
	sta 0+<L+2
	jmp @236
@235:
	lda #1
	sta 0+<L+2
@236:
	lda 0+<L+2
	sta <S+2,x
	lda #.LOBYTE(_500)
	sta <S+3,x
	lda #.HIBYTE(_500)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
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
	sta 0+<S+0,x
	lda #1
	sta 0+<S+1,x
	lda #2
	sta 0+<S+2,x
	lda #7
	sta 0+<S+3,x
	lda #20
	sta 0+<S+4,x
	lda #4
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #4
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_503)
	sta <S+9,x
	lda #.HIBYTE(_503)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+1,x
	clc
	rol a
	clc
	rol a
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #4
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_507)
	sta <S+9,x
	lda #.HIBYTE(_507)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+0,x
	tay
	lda 0+<S+1,x
@237:
	cpy #0
	beq @238
	clc
	rol a
	dey
	jmp @237
@238:
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #1
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_511)
	sta <S+9,x
	lda #.HIBYTE(_511)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+2,x
	tay
	lda 0+<S+1,x
@239:
	cpy #0
	beq @240
	clc
	rol a
	dey
	jmp @239
@240:
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #4
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_515)
	sta <S+9,x
	lda #.HIBYTE(_515)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+3,x
	tay
	lda 0+<S+1,x
@241:
	cpy #0
	beq @242
	clc
	rol a
	dey
	jmp @241
@242:
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #128
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_519)
	sta <S+9,x
	lda #.HIBYTE(_519)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda #5
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #5
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_522)
	sta <S+9,x
	lda #.HIBYTE(_522)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+4,x
	clc
	ror a
	clc
	ror a
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #5
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_526)
	sta <S+9,x
	lda #.HIBYTE(_526)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+0,x
	tay
	lda 0+<S+4,x
@243:
	cpy #0
	beq @244
	clc
	ror a
	dey
	jmp @243
@244:
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #20
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_530)
	sta <S+9,x
	lda #.HIBYTE(_530)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+2,x
	tay
	lda 0+<S+4,x
@245:
	cpy #0
	beq @246
	clc
	ror a
	dey
	jmp @245
@246:
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #5
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_534)
	sta <S+9,x
	lda #.HIBYTE(_534)
	sta <S+10,x
	call _unittest_assert_equal, #5
	lda 0+<S+3,x
	tay
	lda #255
@247:
	cpy #0
	beq @248
	clc
	ror a
	dey
	jmp @247
@248:
	sta 0+<L+0
	lda 0+<L+0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #1
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #.LOBYTE(_538)
	sta <S+9,x
	lda #.HIBYTE(_538)
	sta <S+10,x
	call _unittest_assert_equal, #5
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
	sta 0+<S+0,x
	lda #0
	sta 1+<S+0,x
	lda #2
	sta 0+<S+2,x
	lda #0
	sta 1+<S+2,x
	lda #20
	sta 0+<S+4,x
	lda #0
	sta 1+<S+4,x
	lda #4
	sta <S+6,x
	lda #0
	sta <S+7,x
	lda #4
	sta <S+8,x
	lda #0
	sta <S+9,x
	lda #.LOBYTE(_541)
	sta <S+10,x
	lda #.HIBYTE(_541)
	sta <S+11,x
	call _unittest_assert_equal, #6
	lda 0+<S+0,x
	sta 0+<L+0
	lda 1+<S+0,x
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	sta <S+6,x
	lda 1+<L+0
	sta <S+7,x
	lda #4
	sta <S+8,x
	lda #0
	sta <S+9,x
	lda #.LOBYTE(_545)
	sta <S+10,x
	lda #.HIBYTE(_545)
	sta <S+11,x
	call _unittest_assert_equal, #6
	lda 0+<S+2,x
	sta 0+<L+0
	lda 1+<S+2,x
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	clc
	rol a
	sta 0+<L+0
	lda 1+<L+0
	rol a
	sta 1+<L+0
	lda 0+<L+0
	sta <S+6,x
	lda 1+<L+0
	sta <S+7,x
	lda #0
	sta <S+8,x
	lda #1
	sta <S+9,x
	lda #.LOBYTE(_549)
	sta <S+10,x
	lda #.HIBYTE(_549)
	sta <S+11,x
	call _unittest_assert_equal, #6
	lda #5
	sta <S+6,x
	lda #0
	sta <S+7,x
	lda #5
	sta <S+8,x
	lda #0
	sta <S+9,x
	lda #.LOBYTE(_552)
	sta <S+10,x
	lda #.HIBYTE(_552)
	sta <S+11,x
	call _unittest_assert_equal, #6
	lda 0+<S+4,x
	sta 0+<L+0
	lda 1+<S+4,x
	sta 1+<L+0
	lda 1+<L+0
	clc
	ror a
	sta 1+<L+0
	lda 0+<L+0
	ror a
	sta 0+<L+0
	lda 1+<L+0
	clc
	ror a
	sta 1+<L+0
	lda 0+<L+0
	ror a
	sta 0+<L+0
	lda 0+<L+0
	sta <S+6,x
	lda 1+<L+0
	sta <S+7,x
	lda #5
	sta <S+8,x
	lda #0
	sta <S+9,x
	lda #.LOBYTE(_556)
	sta <S+10,x
	lda #.HIBYTE(_556)
	sta <S+11,x
	call _unittest_assert_equal, #6
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
	sta 0+<L+0
	lda #.HIBYTE(_test_op_gi)
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<S+0,x
	lda 1+<L+0
	sta 1+<S+0,x
	lda #99
	sta 0+_test_op_gi
	lda 0+<S+0,x
	sta <reg+0
	lda 1+<S+0,x
	sta <reg+1
	ldy #0
	lda (reg),y
	sta <S+11,x
	lda #0
	sta <S+12,x
	lda #99
	sta <S+13,x
	lda #0
	sta <S+14,x
	lda #.LOBYTE(_561)
	sta <S+15,x
	lda #.HIBYTE(_561)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda 0+<S+0,x
	sta <reg+0
	lda 1+<S+0,x
	sta <reg+1
	lda #98
	ldy #0
	sta (reg),y
	lda 0+_test_op_gi
	sta <S+11,x
	lda #0
	sta <S+12,x
	lda #98
	sta <S+13,x
	lda #0
	sta <S+14,x
	lda #.LOBYTE(_564)
	sta <S+15,x
	lda #.HIBYTE(_564)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda #.LOBYTE(_test_op_gw)
	sta 0+<L+0
	lda #.HIBYTE(_test_op_gw)
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<S+2,x
	lda 1+<L+0
	sta 1+<S+2,x
	lda #15
	sta 0+_test_op_gw
	lda #39
	sta 1+_test_op_gw
	lda 0+<S+2,x
	sta <reg+0
	lda 1+<S+2,x
	sta <reg+1
	ldy #0
	lda (reg),y
	sta 0+<L+0
	ldy #1
	lda (reg),y
	sta 1+<L+0
	lda 0+<L+0
	sta <S+11,x
	lda 1+<L+0
	sta <S+12,x
	lda #15
	sta <S+13,x
	lda #39
	sta <S+14,x
	lda #.LOBYTE(_569)
	sta <S+15,x
	lda #.HIBYTE(_569)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda 0+<S+2,x
	sta <reg+0
	lda 1+<S+2,x
	sta <reg+1
	lda #14
	ldy #0
	sta (reg),y
	lda #39
	ldy #1
	sta (reg),y
	lda 0+_test_op_gw
	sta <S+11,x
	lda 1+_test_op_gw
	sta <S+12,x
	lda #14
	sta <S+13,x
	lda #39
	sta <S+14,x
	lda #.LOBYTE(_572)
	sta <S+15,x
	lda #.HIBYTE(_572)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda #99
	sta 0+<S+4,x
	txa
	clc
	adc #.LOBYTE(S+4)
	sta 0+<L+0
	lda #0
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<S+5,x
	lda 1+<L+0
	sta 1+<S+5,x
	lda 0+<S+5,x
	sta <reg+0
	lda 1+<S+5,x
	sta <reg+1
	ldy #0
	lda (reg),y
	sta <S+11,x
	lda #0
	sta <S+12,x
	lda #99
	sta <S+13,x
	lda #0
	sta <S+14,x
	lda #.LOBYTE(_577)
	sta <S+15,x
	lda #.HIBYTE(_577)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda #98
	sta 0+<S+4,x
	lda 0+<S+5,x
	sta <reg+0
	lda 1+<S+5,x
	sta <reg+1
	ldy #0
	lda (reg),y
	sta <S+11,x
	lda #0
	sta <S+12,x
	lda #98
	sta <S+13,x
	lda #0
	sta <S+14,x
	lda #.LOBYTE(_581)
	sta <S+15,x
	lda #.HIBYTE(_581)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda #15
	sta 0+<S+7,x
	lda #39
	sta 1+<S+7,x
	txa
	clc
	adc #.LOBYTE(S+7)
	sta 0+<L+0
	lda #0
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<S+9,x
	lda 1+<L+0
	sta 1+<S+9,x
	lda 0+<S+9,x
	sta <reg+0
	lda 1+<S+9,x
	sta <reg+1
	ldy #0
	lda (reg),y
	sta 0+<L+0
	ldy #1
	lda (reg),y
	sta 1+<L+0
	lda 0+<L+0
	sta <S+11,x
	lda 1+<L+0
	sta <S+12,x
	lda #15
	sta <S+13,x
	lda #39
	sta <S+14,x
	lda #.LOBYTE(_586)
	sta <S+15,x
	lda #.HIBYTE(_586)
	sta <S+16,x
	call _unittest_assert_equal, #11
	lda #14
	sta 0+<S+7,x
	lda #39
	sta 1+<S+7,x
	lda 0+<S+9,x
	sta <reg+0
	lda 1+<S+9,x
	sta <reg+1
	ldy #0
	lda (reg),y
	sta 0+<L+0
	ldy #1
	lda (reg),y
	sta 1+<L+0
	lda 0+<L+0
	sta <S+11,x
	lda 1+<L+0
	sta <S+12,x
	lda #14
	sta <S+13,x
	lda #39
	sta <S+14,x
	lda #.LOBYTE(_590)
	sta <S+15,x
	lda #.HIBYTE(_590)
	sta <S+16,x
	call _unittest_assert_equal, #11
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
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_593)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_const_op
	lda #.LOBYTE(_596)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_596)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_599)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_599)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_int_op
	lda #.LOBYTE(_602)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_602)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_605)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_605)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_int8_op
	lda #.LOBYTE(_608)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_608)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_611)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_611)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_int16_op
	lda #.LOBYTE(_614)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_614)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_617)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_617)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_int8x16_op
	lda #.LOBYTE(_620)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_620)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_623)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_623)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_logical_op
	lda #.LOBYTE(_626)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_626)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_629)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_629)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_shift_op
	lda #.LOBYTE(_632)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_632)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_635)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_635)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_shift_op16
	lda #.LOBYTE(_638)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_638)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_641)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_641)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_op_test_pointer_op
	lda #.LOBYTE(_644)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_644)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	lda #1
	sta <FC_FASTCALL_REG+0
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
