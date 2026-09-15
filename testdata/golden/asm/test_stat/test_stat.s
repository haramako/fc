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
	sta 0+<L+0
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
	lda #2
	sta 0+<L+0
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
	lda #0
	sta 0+<L+0
	lda #0
	sta 1+<L+0
	lda 0+<L+0
	bne @1
	lda 1+<L+0
	beq @end_15
@1:
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
@end_15:
	lda #1
	sta 0+<L+0
	lda #0
	sta 1+<L+0
	lda 0+<L+0
	bne @end_21
	lda 1+<L+0
	bne @end_21
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
	bne @then_34
	lda 1+<L+0
	bne @then_34
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
@then_34:
	lda #1
	sta 0+<L+0
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
	inc 0+<S+0,x
	lda 0+<S+0,x
	cmp #3
	bne @begin_40
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
	inc 0+<S+0,x
	lda 0+<S+0,x
	cmp #3
	bcc @begin_50
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
	lda #1
	sta 0+<L+0
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
	jmp @begin_65
@body_67:
	lda #1
	sta 0+<L+0
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
	inc 0+<S+0,x
@begin_65:
	lda 0+<S+0,x
	cmp #10
	bcc @body_67
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
	sta 0+<L+1
	lda #0
	sta 0+<L+0
	jmp @begin_82
@body_97:
	lda 0+<L+0
	cmp #1
	beq @then_89
	lda 0+<L+0
	cmp #2
	beq @then_89
	lda 0+<L+0
	cmp #3
	bne @else_90
@then_89:
	clc
	lda 0+<L+1
	adc 0+<L+0
	sta 0+<L+1
	jmp @end_88
@else_90:
	lda 0+<L+0
	cmp #4
	bne @else_96
	clc
	lda 0+<L+1
	adc #10
	sta 0+<L+1
	jmp @end_88
@else_96:
	clc
	lda 0+<L+1
	adc #20
	sta 0+<L+1
@end_88:
	inc 0+<L+0
@begin_82:
	lda 0+<L+0
	cmp #6
	bcc @body_97
	lda #56
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda 0+<L+1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_102)
	sta <S+4,x
	lda #.HIBYTE(_102)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_102:
		.byte 115,119,105,116,99,104,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_stat"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_105)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_105)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_if
	lda #.LOBYTE(_108)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_108)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_111)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_111)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_loop
	lda #.LOBYTE(_114)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_114)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_117)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_117)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_stat_test_for
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
	jsr _test_stat_test_switch
	lda #.LOBYTE(_126)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_126)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_105:
		.byte 116,101,115,116,95,105,102,58,0
_108:
		.byte 10,0
_111:
		.byte 116,101,115,116,95,108,111,111,112,58,0
_114:
		.byte 10,0
_117:
		.byte 116,101,115,116,95,102,111,114,58,0
_120:
		.byte 10,0
_123:
		.byte 116,101,115,116,95,115,119,105,116,99,104,58,0
_126:
		.byte 10,0
.endproc
_test_stat_main = _main
.segment "CHARS"
	.incbin "character.chr"
