	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_UNITTEST__ = 1
.segment "unittest"
	.include "_stdio.inc"
	.export _unittest_assert_equal
	;;;=============================
	;;; function _unittest_assert_equal
	;;;=============================
.segment "unittest"
.proc _unittest_assert_equal
	lda 0+<F_unittest_assert_equal+0
	cmp 0+<F_unittest_assert_equal+2
	bne @1
	lda 1+<F_unittest_assert_equal+0
	cmp 1+<F_unittest_assert_equal+2
@1:
	bne @3
	jmp @else_14
@3:
	lda #.LOBYTE(_21)
	sta 0+<F_unittest_assert_equal+6
	lda #.HIBYTE(_21)
	sta 1+<F_unittest_assert_equal+6
	lda 0+<F_unittest_assert_equal+6
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_unittest_assert_equal+6
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda 0+<F_unittest_assert_equal+4
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_unittest_assert_equal+4
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda #.LOBYTE(_23)
	sta 0+<F_unittest_assert_equal+6
	lda #.HIBYTE(_23)
	sta 1+<F_unittest_assert_equal+6
	lda 0+<F_unittest_assert_equal+6
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_unittest_assert_equal+6
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda 0+<F_unittest_assert_equal+2
	sta 0+_stdio_EMU_DATA
	lda 1+<F_unittest_assert_equal+2
	sta 1+_stdio_EMU_DATA
	lda #2
	sta 0+_stdio_EMU_PRINT
	lda #.LOBYTE(_25)
	sta 0+<F_unittest_assert_equal+6
	lda #.HIBYTE(_25)
	sta 1+<F_unittest_assert_equal+6
	lda 0+<F_unittest_assert_equal+6
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_unittest_assert_equal+6
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	lda 0+<F_unittest_assert_equal+0
	sta 0+_stdio_EMU_DATA
	lda 1+<F_unittest_assert_equal+0
	sta 1+_stdio_EMU_DATA
	lda #2
	sta 0+_stdio_EMU_PRINT
	lda #.LOBYTE(_27)
	sta 0+<F_unittest_assert_equal+6
	lda #.HIBYTE(_27)
	sta 1+<F_unittest_assert_equal+6
	lda 0+<F_unittest_assert_equal+6
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_unittest_assert_equal+6
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	sta 0+_stdio_EMU_EXIT
	jmp @end_15
@else_14:
	lda #.LOBYTE(_30)
	sta 0+<F_unittest_assert_equal+6
	lda #.HIBYTE(_30)
	sta 1+<F_unittest_assert_equal+6
	lda 0+<F_unittest_assert_equal+6
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_unittest_assert_equal+6
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
@end_15:
	rts
_21:
		.byte 10,69,82,82,79,82,58,32,0
_23:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_25:
		.byte 32,32,98,117,116,32,0
_27:
		.byte 10,0
_30:
		.byte 46,0
.endproc
