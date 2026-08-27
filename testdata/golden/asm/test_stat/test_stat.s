	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_STAT__ = 1
.segment "test_stat"
	.include "_unittest.inc"
_test_stat_I10 = 10
	.export _test_stat_test_if
	;;;=============================
	;;; function _test_stat_test_if
	;;;=============================
.segment "test_stat"
.proc _test_stat_test_if
	lda #1
	beq @else_2
@1:
@then_1:
	lda #1
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_5)
	sta <S+4,x
	lda #.HIBYTE(_5)
	sta <S+5,x
	jsr _unittest_assert_equal
	jmp @end_3
@else_2:
@end_3:
	lda #0
	beq @else_8
@2:
@then_7:
	jmp @end_9
@else_8:
	lda #2
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #2
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_11)
	sta <S+4,x
	lda #.HIBYTE(_11)
	sta <S+5,x
	jsr _unittest_assert_equal
@end_9:
	lda #0
	sta 0+<L+0
	lda #0
	sta 1+<L+0
	lda 0+<L+0
	bne @3
	lda 1+<L+0
	beq @else_14
@3:
@then_13:
	lda #0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_17)
	sta <S+4,x
	lda #.HIBYTE(_17)
	sta <S+5,x
	jsr _unittest_assert_equal
	jmp @end_15
@else_14:
@end_15:
	lda #1
	sta 0+<L+0
	lda #0
	sta 1+<L+0
	lda 0+<L+0
	bne @4
	lda 1+<L+0
	beq @else_20
@4:
@then_19:
	jmp @end_21
@else_20:
	lda #0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_23)
	sta <S+4,x
	lda #.HIBYTE(_23)
	sta <S+5,x
	jsr _unittest_assert_equal
@end_21:
	lda #0
	sta 0+<L+0
	lda #1
	sta 1+<L+0
	lda 0+<L+0
	bne @5
	lda 1+<L+0
	beq @else_26
@5:
@then_25:
	jmp @end_27
@else_26:
	lda #0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_29)
	sta <S+4,x
	lda #.HIBYTE(_29)
	sta <S+5,x
	jsr _unittest_assert_equal
@end_27:
	lda #0
	beq @else_32
@6:
@then_31:
	lda #0
	sta 0+<L+0
	jmp @end_33
@else_32:
	lda #1
	beq @else_35
@7:
@then_34:
	lda #1
	sta 0+<L+0
	jmp @end_36
@else_35:
	lda #2
	sta 0+<L+0
@end_36:
@end_33:
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_38)
	sta <S+4,x
	lda #.HIBYTE(_38)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_5:
		.byte 116,104,101,110,32,115,99,111,112,101,0
_11:
		.byte 101,108,115,101,32,115,99,111,112,101,0
_17:
		.byte 105,102,32,48,58,105,110,116,49,54,0
_23:
		.byte 105,102,32,49,58,105,110,116,49,54,0
_29:
		.byte 105,102,32,50,53,54,58,105,110,116,49,54,0
_38:
		.byte 105,102,32,101,108,115,101,32,101,108,115,101,0
.endproc
	.export _test_stat_test_loop
	;;;=============================
	;;; function _test_stat_test_loop
	;;;=============================
.segment "test_stat"
.proc _test_stat_test_loop
	lda #0
	sta 0+<S+0,x
@begin_40:
	clc
	lda 0+<S+0,x
	adc #1
	sta 0+<S+0,x
	lda 0+<S+0,x
	cmp #3
	bne @else_43
@then_42:
	jmp @end_41
	jmp @end_44
@else_43:
@end_44:
	jmp @begin_40
@end_41:
	lda 0+<S+0,x
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #3
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_48)
	sta <S+5,x
	lda #.HIBYTE(_48)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	lda #0
	sta 0+<S+0,x
@begin_50:
	clc
	lda 0+<S+0,x
	adc #1
	sta 0+<S+0,x
	lda 0+<S+0,x
	cmp #3
	bcs @else_54
@then_53:
	jmp @begin_50
	jmp @end_55
@else_54:
@end_55:
	jmp @end_51
	jmp @begin_50
@end_51:
	lda 0+<S+0,x
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #3
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_58)
	sta <S+5,x
	lda #.HIBYTE(_58)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
@begin_60:
	lda #1
	sta 0+<L+0
	lda 0+<L+0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #1
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_63)
	sta <S+5,x
	lda #.HIBYTE(_63)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	jmp @end_61
	jmp @begin_60
@end_61:
	rts
_48:
		.byte 98,114,101,97,107,0
_58:
		.byte 99,111,110,116,105,110,117,101,0
_63:
		.byte 115,99,111,112,101,0
.endproc
	.export _test_stat_test_for
	;;;=============================
	;;; function _test_stat_test_for
	;;;=============================
.segment "test_stat"
.proc _test_stat_test_for
	lda #0
	sta 0+<S+0,x
	lda #0
	sta 0+<S+1,x
	lda #0
	sta 0+<S+0,x
@begin_65:
	lda 0+<S+0,x
	cmp #10
	bcs @else_68
@then_67:
	lda #1
	sta 0+<L+0
	lda 0+<L+0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #1
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_72)
	sta <S+6,x
	lda #.HIBYTE(_72)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	clc
	lda 0+<S+1,x
	adc 0+<S+0,x
	sta 0+<S+1,x
	clc
	lda 0+<S+0,x
	adc #1
	sta 0+<S+0,x
	jmp @end_69
@else_68:
	jmp @end_66
@end_69:
	jmp @begin_65
@end_66:
	lda 0+<S+0,x
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #10
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_77)
	sta <S+6,x
	lda #.HIBYTE(_77)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	lda 0+<S+1,x
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #45
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_80)
	sta <S+6,x
	lda #.HIBYTE(_80)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	rts
_72:
		.byte 115,99,111,112,101,0
_77:
		.byte 105,0
_80:
		.byte 110,0
.endproc
	.export _test_stat_test_switch
	;;;=============================
	;;; function _test_stat_test_switch
	;;;=============================
.segment "test_stat"
.proc _test_stat_test_switch
	lda #0
	sta 0+<L+2
	lda #0
	sta 0+<L+0
@begin_82:
	lda 0+<L+0
	cmp #6
	bcc @36
	jmp @else_85
@36:
@then_84:
	lda 0+<L+0
	cmp #1
	bne @16
	lda #1
	sta 0+<L+4
	jmp @17
@16:
	lda #0
	sta 0+<L+4
@17:
	lda 0+<L+4
	beq @18
	lda #0
	sta 0+<L+4
	jmp @19
@18:
	lda #1
	sta 0+<L+4
@19:
	lda 0+<L+4
	beq @then_90
@20:
	lda 0+<L+0
	cmp #2
	bne @21
	lda #1
	sta 0+<L+4
	jmp @22
@21:
	lda #0
	sta 0+<L+4
@22:
	lda 0+<L+4
	beq @23
	lda #0
	sta 0+<L+4
	jmp @24
@23:
	lda #1
	sta 0+<L+4
@24:
	lda 0+<L+4
	beq @then_90
@25:
	lda 0+<L+0
	cmp #3
	bne @26
	lda #1
	sta 0+<L+4
	jmp @27
@26:
	lda #0
	sta 0+<L+4
@27:
	lda 0+<L+4
	beq @28
	lda #0
	sta 0+<L+4
	jmp @29
@28:
	lda #1
	sta 0+<L+4
@29:
	lda 0+<L+4
	beq @then_90
@30:
	jmp @else_91
@then_90:
	clc
	lda 0+<L+2
	adc 0+<L+0
	sta 0+<L+2
	jmp @end_89
@else_91:
	lda 0+<L+0
	cmp #4
	bne @31
	lda #1
	sta 0+<L+4
	jmp @32
@31:
	lda #0
	sta 0+<L+4
@32:
	lda 0+<L+4
	beq @33
	lda #0
	sta 0+<L+4
	jmp @34
@33:
	lda #1
	sta 0+<L+4
@34:
	lda 0+<L+4
	beq @then_93
@35:
	jmp @else_94
@then_93:
	clc
	lda 0+<L+2
	adc #10
	sta 0+<L+2
	jmp @end_89
@else_94:
	clc
	lda 0+<L+2
	adc #20
	sta 0+<L+2
@end_89:
	clc
	lda 0+<L+0
	adc #1
	sta 0+<L+0
	jmp @end_86
@else_85:
	jmp @end_83
@end_86:
	jmp @begin_82
@end_83:
	lda #56
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda 0+<L+2
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_99)
	sta <S+4,x
	lda #.HIBYTE(_99)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_99:
		.byte 115,119,105,116,99,104,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_stat"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_102)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_102)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_if
	lda #.LOBYTE(_105)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_105)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_108)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_108)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_loop
	lda #.LOBYTE(_111)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_111)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_114)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_114)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_for
	lda #.LOBYTE(_117)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_117)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_120)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_120)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_switch
	lda #.LOBYTE(_123)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_123)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_102:
		.byte 116,101,115,116,95,105,102,58,0
_105:
		.byte 10,0
_108:
		.byte 116,101,115,116,95,108,111,111,112,58,0
_111:
		.byte 10,0
_114:
		.byte 116,101,115,116,95,102,111,114,58,0
_117:
		.byte 10,0
_120:
		.byte 116,101,115,116,95,115,119,105,116,99,104,58,0
_123:
		.byte 10,0
.endproc
_test_stat_main = _main
.segment "CHARS"
	.incbin "character.chr"
