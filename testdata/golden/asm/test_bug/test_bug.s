	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_TEST_BUG__ = 1
.segment "test_bug"
	.include "_unittest.inc"
	.include "_math.inc"
	.export _test_bug_test_pointer_access
	;;;=============================
	;;; function _test_bug_test_pointer_access
	;;;=============================
.segment "test_bug"
.proc _test_bug_test_pointer_access
	lda #10
	jsr _math_sin
	rts
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_bug"
.proc _main
	lda #.LOBYTE(_3)
	sta 0+<F_main+0
	lda #.HIBYTE(_3)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_bug_test_pointer_access
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
	lda #0
	sta 0+_stdio_EMU_EXIT
	rts
_3:
		.byte 116,101,115,116,95,112,111,105,110,116,101,114,95,97,99,99
		.byte 101,115,115,58,0
_6:
		.byte 10,0
.endproc
_test_bug_main = _main
.segment "CHARS"
	.incbin "character.chr"
