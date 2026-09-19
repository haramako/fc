	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_TEST_ALLOC__ = 1
.segment "test_alloc"
	.include "_unittest.inc"
	.export _test_alloc_test_a_alloc
	;;;=============================
	;;; function _test_alloc_test_a_alloc
	;;;=============================
.segment "test_alloc"
.proc _test_alloc_test_a_alloc
	lda #0
	sta <F_unittest_assert_equal+0
	sta <F_unittest_assert_equal+1
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_4)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_4)
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
	lda #.LOBYTE(_9)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_9)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_14)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_14)
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
	lda #.LOBYTE(_26)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_26)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	rts
_4:
		.byte 97,100,100,47,115,117,98,0
_9:
		.byte 115,117,98,47,97,100,100,0
_14:
		.byte 97,100,100,47,61,61,0
_20:
		.byte 97,100,100,47,62,61,0
_26:
		.byte 97,100,100,47,60,61,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_alloc"
.proc _main
	lda #.LOBYTE(_29)
	sta 0+<F_main+0
	lda #.HIBYTE(_29)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_alloc_test_a_alloc
	lda #.LOBYTE(_32)
	sta 0+<F_main+0
	lda #.HIBYTE(_32)
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
_29:
		.byte 116,101,115,116,95,97,95,97,108,108,111,99,58,0
_32:
		.byte 10,0
.endproc
_test_alloc_main = _main
.segment "CHARS"
	.incbin "character.chr"
