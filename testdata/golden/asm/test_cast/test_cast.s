	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_TEST_CAST__ = 1
.segment "test_cast"
	.include "_unittest.inc"
	.include "_stdio.inc"
	.export _test_cast_a1
.segment "test_cast"
_test_cast_a1:
	.byte 0,1,2,3
	.export _test_cast_test_cast
	;;;=============================
	;;; function _test_cast_test_cast
	;;;=============================
.segment "test_cast"
.proc _test_cast_test_cast
	lda #255
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #255
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_3)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_3)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_cast_a1)
	sta 0+<F_test_cast_test_cast+0
	lda #.HIBYTE(_test_cast_a1)
	sta 1+<F_test_cast_test_cast+0
	lda 0+<F_test_cast_test_cast+0
	sta 0+<F_test_cast_test_cast+2
	lda 1+<F_test_cast_test_cast+0
	sta 1+<F_test_cast_test_cast+2
	lda #0
	asl a
	tay
	lda (F_test_cast_test_cast+2),y
	sta 0+<F_test_cast_test_cast+0
	iny
	lda (F_test_cast_test_cast+2),y
	sta 1+<F_test_cast_test_cast+0
	lda 0+<F_test_cast_test_cast+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_cast_test_cast+0
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	lda #1
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_8)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_8)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda 0+_test_cast_a1
	sta 0+<F_test_cast_test_cast+0
	lda 1+_test_cast_a1
	sta 1+<F_test_cast_test_cast+0
	lda #0
	asl a
	tay
	lda (F_test_cast_test_cast+0),y
	sta 0+<F_test_cast_test_cast+2
	iny
	lda (F_test_cast_test_cast+0),y
	sta 1+<F_test_cast_test_cast+2
	lda 0+<F_test_cast_test_cast+2
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_cast_test_cast+2
	sta <F_unittest_assert_equal+1
	ldy #0
	lda _test_cast_a1+0,y
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_15)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_15)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_3:
		.byte 97,115,32,105,110,116,0
_8:
		.byte 91,93,32,97,115,32,117,105,110,116,49,54,0
_15:
		.byte 97,115,32,117,105,110,116,42,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_cast"
.proc _main
	lda #.LOBYTE(_18)
	sta 0+<F_main+0
	lda #.HIBYTE(_18)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_cast_test_cast
	lda #.LOBYTE(_21)
	sta 0+<F_main+0
	lda #.HIBYTE(_21)
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
_18:
		.byte 116,101,115,116,95,99,97,115,116,58,0
_21:
		.byte 10,0
.endproc
_test_cast_main = _main
.segment "CHARS"
	.incbin "character.chr"
