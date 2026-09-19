	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
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
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_5)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_5)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #2
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_11)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_11)
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
	lda #.LOBYTE(_38)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_38)
	sta <F_unittest_assert_equal+5
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
	sta 0+<F_test_stat_test_loop+0
	tay
@begin_40:
	iny
	cpy #3
	bne @begin_40
	sty 0+<F_test_stat_test_loop+0
	lda 0+<F_test_stat_test_loop+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_48)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_48)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #0
	sta 0+<F_test_stat_test_loop+0
@begin_50:
	inc 0+<F_test_stat_test_loop+0
	lda 0+<F_test_stat_test_loop+0
	cmp #3
	bcc @begin_50
	lda 0+<F_test_stat_test_loop+0
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
	lda #1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_63)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_63)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
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
	sta 0+<F_test_stat_test_for+1
	cmp #10
	bcs @end_66
	sta 0+<F_test_stat_test_for+0
@body_67:
	lda #1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_72)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_72)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	clc
	lda 0+<F_test_stat_test_for+1
	adc 0+<F_test_stat_test_for+0
	sta 0+<F_test_stat_test_for+1
	inc 0+<F_test_stat_test_for+0
	lda 0+<F_test_stat_test_for+0
	cmp #10
	bcc @body_67
	lda 0+<F_test_stat_test_for+0
@end_66:
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #10
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_77)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_77)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+<F_test_stat_test_for+1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #45
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_80)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_80)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
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
	sta 0+<F_test_stat_test_switch+1
	sta 0+<F_test_stat_test_switch+0
	cmp #6
	bcs @end_83
	tax
@body_97:
	cpx #1
	beq @then_89
	cpx #2
	beq @then_89
	cpx #3
	bne @else_90
@then_89:
	stx 0+<F_test_stat_test_switch+0
	sta 0+<F_test_stat_test_switch+1
	clc
	adc 0+<F_test_stat_test_switch+0
	sta 0+<F_test_stat_test_switch+1
	ldx 0+<F_test_stat_test_switch+0
	jmp @end_88
@else_90:
	cpx #4
	bne @else_96
	clc
	adc #10
	jmp @end_88
@else_96:
	clc
	adc #20
@end_88:
	inx
	cpx #6
	bcc @body_97
	sta 0+<F_test_stat_test_switch+1
@end_83:
	lda #56
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda 0+<F_test_stat_test_switch+1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_102)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_102)
	sta <F_unittest_assert_equal+5
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
	lda #.LOBYTE(_105)
	sta 0+<F_main+0
	lda #.HIBYTE(_105)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_stat_test_if
	lda #.LOBYTE(_108)
	sta 0+<F_main+0
	lda #.HIBYTE(_108)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda #.LOBYTE(_111)
	sta 0+<F_main+0
	lda #.HIBYTE(_111)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_stat_test_loop
	lda #.LOBYTE(_114)
	sta 0+<F_main+0
	lda #.HIBYTE(_114)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda #.LOBYTE(_117)
	sta 0+<F_main+0
	lda #.HIBYTE(_117)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_stat_test_for
	lda #.LOBYTE(_120)
	sta 0+<F_main+0
	lda #.HIBYTE(_120)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda #.LOBYTE(_123)
	sta 0+<F_main+0
	lda #.HIBYTE(_123)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_stat_test_switch
	lda #.LOBYTE(_126)
	sta 0+<F_main+0
	lda #.HIBYTE(_126)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda #0
	sta 0+_stdio_EMU_EXIT
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
