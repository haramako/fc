	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_ALLOC__ = 1
.segment "test_alloc"
	.include "_unittest.inc"
	.export _test_alloc_test_a_alloc
	;;;=============================
	;;; function _test_alloc_test_a_alloc
	;;;=============================
.segment "test_alloc"
.proc _test_alloc_test_a_alloc
	lda #1
	sta 0+<S+0,x
	lda #2
	sta 0+<S+1,x
	lda #3
	sta 0+<S+2,x
	clc
	lda 0+<S+0,x
	adc 0+<S+1,x
	sec
	sbc 0+<S+2,x
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #.LOBYTE(_4)
	sta <S+7,x
	lda #.HIBYTE(_4)
	sta <S+8,x
	inx
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	dex
	sec
	lda 0+<S+0,x
	sbc 0+<S+1,x
	clc
	adc 0+<S+2,x
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #2
	sta <S+5,x
	lda #0
	sta <S+6,x
	lda #.LOBYTE(_9)
	sta <S+7,x
	lda #.HIBYTE(_9)
	sta <S+8,x
	inx
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	dex
	clc
	lda 0+<S+0,x
	adc 0+<S+1,x
	cmp #3
	bne @1
	lda #1
	jmp @2
@1:
	lda #0
@2:
	sta <S+3,x
	lda #.LOBYTE(_14)
	sta <S+4,x
	lda #.HIBYTE(_14)
	sta <S+5,x
	inx
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	dex
	clc
	lda 0+<S+0,x
	adc 0+<S+1,x
	cmp #3
	lda #0
	rol a
	eor #1
	beq @5
	lda #0
	sta 0+<L+0
	jmp @6
@5:
	lda #1
	sta 0+<L+0
@6:
	lda 0+<L+0
	sta <S+3,x
	lda #.LOBYTE(_20)
	sta <S+4,x
	lda #.HIBYTE(_20)
	sta <S+5,x
	inx
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	dex
	clc
	lda 0+<S+0,x
	adc 0+<S+1,x
	sta 0+<L+0
	lda #3
	cmp 0+<L+0
	lda #0
	rol a
	eor #1
	beq @9
	lda #0
	sta 0+<L+0
	jmp @10
@9:
	lda #1
	sta 0+<L+0
@10:
	lda 0+<L+0
	sta <S+3,x
	lda #.LOBYTE(_26)
	sta <S+4,x
	lda #.HIBYTE(_26)
	sta <S+5,x
	inx
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	dex
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
	jsr _stdio_init
	lda #.LOBYTE(_29)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_29)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_alloc_test_a_alloc
	lda #.LOBYTE(_32)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_32)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_29:
		.byte 116,101,115,116,95,97,95,97,108,108,111,99,58,0
_32:
		.byte 10,0
.endproc
_test_alloc_main = _main
.segment "CHARS"
	.incbin "character.chr"
