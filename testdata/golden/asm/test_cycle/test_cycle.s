	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_TEST_CYCLE__ = 1
.segment "test_cycle"
	.include "_unittest.inc"
	.include "_cycle_use.inc"
	.export _test_cycle_cycle_var
.segment "BSS"
_test_cycle_cycle_var: .res 1
	.export _test_cycle_test_cycle_use
	;;;=============================
	;;; function _test_cycle_test_cycle_use
	;;;=============================
.segment "test_cycle"
.proc _test_cycle_test_cycle_use
	lda #99
	sta 0+_test_cycle_cycle_var
	lda 0+_test_cycle_cycle_var
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #99
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_3)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_3)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_3:
		.byte 99,121,99,108,101,32,117,115,101,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_cycle"
.proc _main
	lda #.LOBYTE(_6)
	sta 0+<F_main+0
	lda #.HIBYTE(_6)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_cycle_test_cycle_use
	lda #.LOBYTE(_9)
	sta 0+<F_main+0
	lda #.HIBYTE(_9)
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
_6:
		.byte 116,101,115,116,95,99,121,99,108,101,95,117,115,101,58,0
_9:
		.byte 10,0
.endproc
_test_cycle_main = _main
.segment "CHARS"
	.incbin "character.chr"
