	.setcpu "6502"
	.include "macro.inc"
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
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #99
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_3)
	sta <S+4,x
	lda #.HIBYTE(_3)
	sta <S+5,x
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
	jsr _stdio_init
	lda #.LOBYTE(_6)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_6)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_cycle_test_cycle_use
	lda #.LOBYTE(_9)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_9)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
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
