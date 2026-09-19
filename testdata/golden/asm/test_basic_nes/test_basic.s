	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
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
	lda 0+<F_test_basic_add+1
	adc 0+<F_test_basic_add+2
	sta 0+<F_test_basic_add+0
	rts
.endproc
	.export _test_basic_fib
	;;;=============================
	;;; function _test_basic_fib
	;;;=============================
.segment "test_basic"
.proc _test_basic_fib
	txa
	clc
	adc #3
	sta FC_SP
	lda #1
	cmp 0+<S+1,x
	bcc @else_10
	lda #1
	sta 0+<S+0,x
	lda FC_SP
	sec
	sbc #3
	sta FC_SP
	rts
@else_10:
	sec
	lda 0+<S+1,x
	sbc #1
	sta <S+4,x
	ldx FC_SP
	jsr _test_basic_fib
	lda FC_SP
	sec
	sbc #3
	tax
	lda <0+S+3,x
	sta 0+<S+2,x
	sec
	lda 0+<S+1,x
	sbc #2
	sta <S+4,x
	ldx FC_SP
	jsr _test_basic_fib
	lda FC_SP
	sec
	sbc #3
	tax
	lda <0+S+3,x
	sta 0+<L+0
	clc
	adc 0+<S+2,x
	sta 0+<S+0,x
	lda FC_SP
	sec
	sbc #3
	sta FC_SP
	rts
.endproc
	.export _test_basic_test_function
	;;;=============================
	;;; function _test_basic_test_function
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_function
	lda #3
	sta <F_test_basic_add+1
	lda #5
	sta <F_test_basic_add+2
	jsr _test_basic_add
	lda <F_test_basic_add+0
	sta 0+<F_test_basic_test_function+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #8
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_20)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_20)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	ldx FC_SP
	lda #1
	sta <S+1,x
	ldx FC_SP
	jsr _test_basic_fib
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_basic_test_function+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_24)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_24)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	ldx FC_SP
	lda #3
	sta <S+1,x
	ldx FC_SP
	jsr _test_basic_fib
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_basic_test_function+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_28)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_28)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	ldx FC_SP
	lda #11
	sta <S+1,x
	ldx FC_SP
	jsr _test_basic_fib
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_basic_test_function+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #144
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_32)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_32)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_20:
		.byte 97,100,100,40,41,0
_24:
		.byte 102,105,98,40,49,41,0
_28:
		.byte 102,105,98,40,51,41,0
_32:
		.byte 102,105,98,40,49,49,41,0
.endproc
	.export _test_basic_test_misc
	;;;=============================
	;;; function _test_basic_test_misc
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_misc
	lda #1
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_43)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_43)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #0
	lda #255
	sta <F_unittest_assert_equal+0
	sta <F_unittest_assert_equal+1
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_47)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_47)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	ldx FC_SP
	lda #.LOBYTE(_53)
	sta <S+1,x
	lda #.HIBYTE(_53)
	sta <S+2,x
	lda #.LOBYTE(_55)
	sta <S+3,x
	lda #.HIBYTE(_55)
	sta <S+4,x
	lda #6
	sta <S+5,x
	ldx FC_SP
	jsr _mem_compare
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_basic_test_misc+0
	bne @4
	lda #1
	sta 0+<F_test_basic_test_misc+1
	jmp @5
@4:
	lda #0
	sta 0+<F_test_basic_test_misc+1
@5:
	lda 0+<F_test_basic_test_misc+1
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_58)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_58)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true
	rts
_40:
		.byte 108,111,111,112,32,98,114,101,97,107,0
_43:
		.byte 108,111,111,112,0
_47:
		.byte 99,97,115,116,0
_53:
		.byte 104,111,10,103,101,0
_55:
		.byte 104,111,10,103,101,0
_58:
		.byte 104,101,114,101,32,115,116,114,105,110,103,0
.endproc
	.export _test_basic_add_fastcall
	;;;=============================
	;;; function _test_basic_add_fastcall
	;;;=============================
.segment "test_basic"
.proc _test_basic_add_fastcall
	clc
	lda 0+<F_test_basic_add_fastcall+1
	adc 0+<F_test_basic_add_fastcall+2
	sta 0+<F_test_basic_add_fastcall+0
	rts
.endproc
	.export _test_basic_test_fastcall
	;;;=============================
	;;; function _test_basic_test_fastcall
	;;;=============================
.segment "test_basic"
.proc _test_basic_test_fastcall
	lda #1
	sta <F_test_basic_add_fastcall+1
	lda #2
	sta <F_test_basic_add_fastcall+2
	jsr _test_basic_add_fastcall
	lda <F_test_basic_add_fastcall+0
	sta 0+<F_test_basic_test_fastcall+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_63)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_63)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
_63:
		.byte 97,100,100,95,102,97,115,116,99,97,108,108,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_basic"
.proc _main
	jsr _stdio_init
	ldx FC_SP
	lda #.LOBYTE(_66)
	sta <S+0,x
	lda #.HIBYTE(_66)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	jsr _test_basic_test_nesasm_limit
	ldx FC_SP
	lda #.LOBYTE(_69)
	sta <S+0,x
	lda #.HIBYTE(_69)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	ldx FC_SP
	lda #.LOBYTE(_72)
	sta <S+0,x
	lda #.HIBYTE(_72)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	jsr _test_basic_test_function
	ldx FC_SP
	lda #.LOBYTE(_75)
	sta <S+0,x
	lda #.HIBYTE(_75)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	ldx FC_SP
	lda #.LOBYTE(_78)
	sta <S+0,x
	lda #.HIBYTE(_78)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	jsr _test_basic_test_misc
	ldx FC_SP
	lda #.LOBYTE(_81)
	sta <S+0,x
	lda #.HIBYTE(_81)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	ldx FC_SP
	lda #.LOBYTE(_84)
	sta <S+0,x
	lda #.HIBYTE(_84)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	jsr _test_basic_test_fastcall
	ldx FC_SP
	lda #.LOBYTE(_87)
	sta <S+0,x
	lda #.HIBYTE(_87)
	sta <S+1,x
	ldx FC_SP
	jsr _stdio_print
	lda #0
	sta <F_stdio_exit+0
	jsr _stdio_exit
	rts
_66:
		.byte 116,101,115,116,95,110,101,115,97,115,109,95,108,105,109,105
		.byte 116,58,0
_69:
		.byte 10,0
_72:
		.byte 116,101,115,116,95,102,117,110,99,116,105,111,110,58,0
_75:
		.byte 10,0
_78:
		.byte 116,101,115,116,95,109,105,115,99,58,0
_81:
		.byte 10,0
_84:
		.byte 116,101,115,116,95,102,97,115,116,99,97,108,108,58,0
_87:
		.byte 10,0
.endproc
_test_basic_main = _main
.segment "CHARS"
	.incbin "character.chr"
