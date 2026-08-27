	.setcpu "6502"
	.include "macro.inc"
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
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #2
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_5)
	sta <S+4,x
	lda #.HIBYTE(_5)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #1
	lda _test_option_base_data+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #2
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_10)
	sta <S+4,x
	lda #.HIBYTE(_10)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_5:
		.byte 98,97,115,101,95,100,97,116,97,0
_10:
		.byte 97,115,109,95,115,121,109,98,111,108,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_option"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_13)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_13)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_option_test_address
	lda #.LOBYTE(_16)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_16)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
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
