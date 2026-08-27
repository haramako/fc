	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_CAST__ = 1
.segment "test_cast"
	.include "_unittest.inc"
	.include "_stdio.inc"
	.export _test_cast_a1
.segment "test_cast"
_test_cast_a1:
	.byte 0,1,2,3
	.export _test_cast_test_cast
	;;;=============================
	;;; function _test_cast_test_cast
	;;;=============================
.segment "test_cast"
.proc _test_cast_test_cast
	lda #255
	sta 0+<L+0
	lda #255
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #255
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_3)
	sta <S+4,x
	lda #.HIBYTE(_3)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_cast_a1)
	sta 0+<L+0
	lda #.HIBYTE(_test_cast_a1)
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<L+2
	lda 1+<L+0
	sta 1+<L+2
	lda #0
	asl a
	tay
	sty <reg+0
	clc
	lda 0+<L+2
	adc <reg+0
	sta 0+<L+0
	lda 1+<L+2
	adc #0
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	ldy #0
	lda (reg),y
	sta 0+<L+2
	ldy #1
	lda (reg),y
	sta 1+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda 1+<L+2
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #1
	sta <S+3,x
	lda #.LOBYTE(_8)
	sta <S+4,x
	lda #.HIBYTE(_8)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda 0+_test_cast_a1
	sta 0+<L+0
	lda 1+_test_cast_a1
	sta 1+<L+0
	lda #0
	asl a
	tay
	sty <reg+0
	clc
	lda 0+<L+0
	adc <reg+0
	sta 0+<L+2
	lda 1+<L+0
	adc #0
	sta 1+<L+2
	lda 0+<L+2
	sta <reg+0
	lda 1+<L+2
	sta <reg+1
	ldy #0
	lda (reg),y
	sta 0+<L+0
	ldy #1
	lda (reg),y
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	ldy #0
	lda _test_cast_a1+0,y
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_15)
	sta <S+4,x
	lda #.HIBYTE(_15)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_3:
		.byte 97,115,32,105,110,116,0
_8:
		.byte 91,93,32,97,115,32,117,105,110,116,49,54,0
_15:
		.byte 97,115,32,117,105,110,116,42,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_cast"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_18)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_18)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_cast_test_cast
	lda #.LOBYTE(_21)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_21)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_18:
		.byte 116,101,115,116,95,99,97,115,116,58,0
_21:
		.byte 10,0
.endproc
_test_cast_main = _main
.segment "CHARS"
	.incbin "character.chr"
