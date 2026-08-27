	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_BASIC__ = 1
.segment "test_basic"
	.include "_unittest.inc"
	.include "_mem.inc"
	.include "test_basic.asm"
	.export _test_basic_hoge
.segment "BSS"
_test_basic_hoge: .res 1
	.export _test_basic_i1
.segment "BSS"
_test_basic_i1: .res 1
_test_basic_CONST = 2
	.export _test_basic_a1
.segment "BSS"
_test_basic_a1: .res 4
	.export _test_basic_a2
.segment "BSS"
_test_basic_a2: .res 2
	.export _test_basic_a3
.segment "test_basic"
_test_basic_a3:
	.byte 1,2
	.export _test_basic_s1
.segment "test_basic"
_test_basic_s1:
	.byte 104,111,103,101,0
_test_basic_c = 3
_test_basic_UINT = 1
_test_basic_SINT = -1
_test_basic_UINT16 = 256
_test_basic_SINT16 = -255
	.export _test_basic_ARRAY
.segment "test_basic"
_test_basic_ARRAY:
	.byte 0
	.export _test_basic_test_nesasm_limit
	;;;=============================
	;;; function _test_basic_test_nesasm_limit
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_nesasm_limit
	lda #0
	clc
	adc #1
	rts
_6:
		.byte 116,111,111,111,111,111,111,111,111,111,111,111,111,111,111,111
		.byte 111,111,111,111,111,111,111,111,111,111,111,111,111,111,111,111
		.byte 111,111,111,111,111,111,111,111,111,111,111,111,111,111,111,111
		.byte 111,111,111,95,108,111,110,103,95,115,116,114,105,110,103,0
.endproc
	.export _test_basic_add
	;;;=============================
	;;; function _test_basic_add
	;;;=============================
.segment "test_basic"
.proc _test_basic_add
	clc
	lda 0+<S+1,x
	adc 0+<S+2,x
	sta 0+<S+0,x
	rts
.endproc
	.export _test_basic_fib
	;;;=============================
	;;; function _test_basic_fib
	;;;=============================
.segment "test_basic"
.proc _test_basic_fib
	lda #1
	cmp 0+<S+1,x
	bcc @else_10
@then_9:
	lda #1
	sta 0+<S+0,x
	rts
	jmp @end_11
@else_10:
	sec
	lda 0+<S+1,x
	sbc #1
	sta <S+4,x
	inx
	inx
	inx
	jsr _test_basic_fib
	dex
	dex
	dex
	lda <0+S+3,x
	sta 0+<S+2,x
	sec
	lda 0+<S+1,x
	sbc #2
	sta <S+4,x
	inx
	inx
	inx
	jsr _test_basic_fib
	dex
	dex
	dex
	lda <0+S+3,x
	sta 0+<L+0
	clc
	lda 0+<S+2,x
	adc 0+<L+0
	sta 0+<S+0,x
	rts
@end_11:
.endproc
	.export _test_basic_test_function
	;;;=============================
	;;; function _test_basic_test_function
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_function
	lda #3
	sta <S+1,x
	lda #5
	sta <S+2,x
	jsr _test_basic_add
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #8
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_21)
	sta <S+4,x
	lda #.HIBYTE(_21)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <S+1,x
	jsr _test_basic_fib
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_25)
	sta <S+4,x
	lda #.HIBYTE(_25)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #3
	sta <S+1,x
	jsr _test_basic_fib
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #3
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_29)
	sta <S+4,x
	lda #.HIBYTE(_29)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #11
	sta <S+1,x
	jsr _test_basic_fib
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #144
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_33)
	sta <S+4,x
	lda #.HIBYTE(_33)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_21:
		.byte 97,100,100,40,41,0
_25:
		.byte 102,105,98,40,49,41,0
_29:
		.byte 102,105,98,40,51,41,0
_33:
		.byte 102,105,98,40,49,49,41,0
.endproc
	.export _test_basic_test_misc
	;;;=============================
	;;; function _test_basic_test_misc
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_misc
	lda #1
	sta 0+<S+0,x
	lda #0
	sta 0+<S+1,x
@begin_35:
	lda #1
	sta 0+<S+1,x
	lda 0+<S+0,x
	beq @else_38
@3:
@then_37:
	jmp @end_36
	jmp @end_39
@else_38:
@end_39:
	lda #0
	sta <S+2,x
	lda #.LOBYTE(_41)
	sta <S+3,x
	lda #.HIBYTE(_41)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	jmp @begin_35
@end_36:
	lda 0+<S+1,x
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #1
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_44)
	sta <S+6,x
	lda #.HIBYTE(_44)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	lda #0
	lda #255
	sta 0+<L+0
	lda #255
	sta 1+<L+0
	lda 0+<L+0
	sta <S+2,x
	lda 1+<L+0
	sta <S+3,x
	lda #255
	sta <S+4,x
	lda #255
	sta <S+5,x
	lda #.LOBYTE(_48)
	sta <S+6,x
	lda #.HIBYTE(_48)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	lda #.LOBYTE(_54)
	sta <S+3,x
	lda #.HIBYTE(_54)
	sta <S+4,x
	lda #.LOBYTE(_56)
	sta <S+5,x
	lda #.HIBYTE(_56)
	sta <S+6,x
	lda #6
	sta <S+7,x
	inx
	inx
	jsr _mem_compare
	dex
	dex
	lda <0+S+2,x
	sta 0+<L+0
	lda 0+<L+0
	cmp #0
	bne @4
	lda #1
	jmp @5
@4:
	lda #0
@5:
	sta <S+2,x
	lda #.LOBYTE(_59)
	sta <S+3,x
	lda #.HIBYTE(_59)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	rts
_41:
		.byte 108,111,111,112,32,98,114,101,97,107,0
_44:
		.byte 108,111,111,112,0
_48:
		.byte 99,97,115,116,0
_54:
		.byte 104,111,10,103,101,0
_56:
		.byte 104,111,10,103,101,0
_59:
		.byte 104,101,114,101,32,115,116,114,105,110,103,0
.endproc
	.export _test_basic_add_fastcall
	;;;=============================
	;;; function _test_basic_add_fastcall
	;;;=============================
.segment "test_basic"
.proc _test_basic_add_fastcall
	clc
	lda 0+<FC_FASTCALL_REG+1
	adc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+0
	rts
.endproc
	.export _test_basic_test_fastcall
	;;;=============================
	;;; function _test_basic_test_fastcall
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_fastcall
	lda #1
	sta <FC_FASTCALL_REG+1
	lda #2
	sta <FC_FASTCALL_REG+2
	jsr _test_basic_add_fastcall
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #3
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_64)
	sta <S+4,x
	lda #.HIBYTE(_64)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_64:
		.byte 97,100,100,95,102,97,115,116,99,97,108,108,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_basic"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_67)
	sta <S+0,x
	lda #.HIBYTE(_67)
	sta <S+1,x
	jsr _stdio_print
	jsr _test_basic_test_nesasm_limit
	lda #.LOBYTE(_70)
	sta <S+0,x
	lda #.HIBYTE(_70)
	sta <S+1,x
	jsr _stdio_print
	lda #.LOBYTE(_73)
	sta <S+0,x
	lda #.HIBYTE(_73)
	sta <S+1,x
	jsr _stdio_print
	jsr _test_basic_test_function
	lda #.LOBYTE(_76)
	sta <S+0,x
	lda #.HIBYTE(_76)
	sta <S+1,x
	jsr _stdio_print
	lda #.LOBYTE(_79)
	sta <S+0,x
	lda #.HIBYTE(_79)
	sta <S+1,x
	jsr _stdio_print
	jsr _test_basic_test_misc
	lda #.LOBYTE(_82)
	sta <S+0,x
	lda #.HIBYTE(_82)
	sta <S+1,x
	jsr _stdio_print
	lda #.LOBYTE(_85)
	sta <S+0,x
	lda #.HIBYTE(_85)
	sta <S+1,x
	jsr _stdio_print
	jsr _test_basic_test_fastcall
	lda #.LOBYTE(_88)
	sta <S+0,x
	lda #.HIBYTE(_88)
	sta <S+1,x
	jsr _stdio_print
	lda #0
	sta <S+0,x
	jsr _stdio_exit
	rts
_67:
		.byte 116,101,115,116,95,110,101,115,97,115,109,95,108,105,109,105
		.byte 116,58,0
_70:
		.byte 10,0
_73:
		.byte 116,101,115,116,95,102,117,110,99,116,105,111,110,58,0
_76:
		.byte 10,0
_79:
		.byte 116,101,115,116,95,109,105,115,99,58,0
_82:
		.byte 10,0
_85:
		.byte 116,101,115,116,95,102,97,115,116,99,97,108,108,58,0
_88:
		.byte 10,0
.endproc
_test_basic_main = _main
.segment "CHARS"
	.incbin "character.chr"
