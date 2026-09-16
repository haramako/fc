	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
__MODULE_TEST_OPTION__ = 1
.segment "test_option"
	.include "_unittest.inc"
	.include "_mem.inc"
	.export _test_option_base_data
.segment "test_option"
_test_option_base_data:
	.byte 1,2,3
	.export _test_option_test_address
	;;;=============================
	;;; function _test_option_test_address
	;;;=============================
.segment "test_option"
.proc _test_option_test_address
	ldy #1
	lda _test_option_base_data+0,y
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_5)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_5)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	ldy #1
	lda _test_option_base_data+0,y
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_10)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_10)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_5:
		.byte 98,97,115,101,95,100,97,116,97,0
_10:
		.byte 97,115,109,95,115,121,109,98,111,108,0
.endproc
	.export _main
	.export _main__direct
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_option"
_main:
.proc _main__direct
	jsr _stdio_init
	lda #.LOBYTE(_13)
	sta <F_stdio_print+0
	lda #.HIBYTE(_13)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_option_test_address
	lda #.LOBYTE(_16)
	sta <F_stdio_print+0
	lda #.HIBYTE(_16)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #0
	sta <F_stdio_exit+0
	jsr _stdio_exit
	rts
_13:
		.byte 116,101,115,116,95,97,100,100,114,101,115,115,58,0
_16:
		.byte 10,0
.endproc
_test_option_main = _main
.segment "CHARS"
	.incbin "character.chr"
