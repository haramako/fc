	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
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
	jsr _cycle_use_hoge
	lda <F_cycle_use_hoge+0
	sta 0+<F_test_cycle_test_cycle_use+0
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
	.export _main__direct
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_cycle"
_main:
.proc _main__direct
	jsr _stdio_init
	lda #.LOBYTE(_6)
	sta <F_stdio_print+0
	lda #.HIBYTE(_6)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_cycle_test_cycle_use
	lda #.LOBYTE(_9)
	sta <F_stdio_print+0
	lda #.HIBYTE(_9)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #0
	sta <F_stdio_exit+0
	jsr _stdio_exit
	rts
_6:
		.byte 116,101,115,116,95,99,121,99,108,101,95,117,115,101,58,0
_9:
		.byte 10,0
.endproc
_test_cycle_main = _main
.segment "CHARS"
	.incbin "character.chr"
