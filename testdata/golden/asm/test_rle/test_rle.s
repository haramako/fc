	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_RLE__ = 1
.segment "test_rle"
	.include "_unittest.inc"
	.include "_rle.inc"
	.include "_mem.inc"
	.export _test_rle_src
.segment "test_rle"
_test_rle_src:
	.byte 1,1,3,2,3,4,5,5,0
	.export _test_rle_src2
.segment "test_rle"
_test_rle_src2:
	.byte 97,97,2,98,98,1,99,99,1,100,100,0
	.export _test_rle_dest
.segment "BSS"
_test_rle_dest: .res 128
	.export _test_rle_test_rle
	;;;=============================
	;;; function _test_rle_test_rle
	;;;=============================
.segment "test_rle"
.proc _test_rle_test_rle
	lda #.LOBYTE(_test_rle_dest)
	sta <S+4,x
	lda #.HIBYTE(_test_rle_dest)
	sta <S+5,x
	lda #.LOBYTE(_test_rle_src)
	sta <S+6,x
	lda #.HIBYTE(_test_rle_src)
	sta <S+7,x
	inx
	inx
	jsr _rle_unpack
	dex
	dex
	lda <0+S+2,x
	sta 0+<L+0
	lda <1+S+2,x
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<S+0,x
	lda 1+<L+0
	sta 1+<S+0,x
	lda 0+<S+0,x
	sta <S+2,x
	lda 1+<S+0,x
	sta <S+3,x
	lda #8
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_5)
	sta <S+6,x
	lda #.HIBYTE(_5)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	lda #.LOBYTE(_test_rle_dest)
	sta <S+3,x
	lda #.HIBYTE(_test_rle_dest)
	sta <S+4,x
	lda #.LOBYTE(_10)
	sta <S+5,x
	lda #.HIBYTE(_10)
	sta <S+6,x
	lda 0+<S+0,x
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
	bne @1
	lda #1
	jmp @2
@1:
	lda #0
@2:
	sta <S+2,x
	lda #.LOBYTE(_13)
	sta <S+3,x
	lda #.HIBYTE(_13)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	lda #.LOBYTE(_test_rle_dest)
	sta <S+4,x
	lda #.HIBYTE(_test_rle_dest)
	sta <S+5,x
	lda #.LOBYTE(_test_rle_src2)
	sta <S+6,x
	lda #.HIBYTE(_test_rle_src2)
	sta <S+7,x
	inx
	inx
	jsr _rle_unpack
	dex
	dex
	lda <0+S+2,x
	sta 0+<L+0
	lda <1+S+2,x
	sta 1+<L+0
	lda 0+<L+0
	sta 0+<S+0,x
	lda 1+<L+0
	sta 1+<S+0,x
	lda 0+<S+0,x
	sta <S+2,x
	lda 1+<S+0,x
	sta <S+3,x
	lda #8
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_17)
	sta <S+6,x
	lda #.HIBYTE(_17)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	lda #.LOBYTE(_test_rle_dest)
	sta <S+3,x
	lda #.HIBYTE(_test_rle_dest)
	sta <S+4,x
	lda #.LOBYTE(_22)
	sta <S+5,x
	lda #.HIBYTE(_22)
	sta <S+6,x
	lda 0+<S+0,x
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
	bne @3
	lda #1
	jmp @4
@3:
	lda #0
@4:
	sta <S+2,x
	lda #.LOBYTE(_25)
	sta <S+3,x
	lda #.HIBYTE(_25)
	sta <S+4,x
	inx
	inx
	jsr _unittest_assert_true
	dex
	dex
	rts
_5:
		.byte 108,101,110,0
_10:
		.byte 1,1,1,1,2,3,4,5
_13:
		.byte 99,104,101,99,107,0
_17:
		.byte 108,101,110,0
_22:
		.byte 97,97,97,98,98,99,99,100,0
_25:
		.byte 99,104,101,99,107,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_rle"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_28)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_28)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_rle_test_rle
	lda #.LOBYTE(_31)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_31)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_28:
		.byte 116,101,115,116,95,114,108,101,58,0
_31:
		.byte 10,0
.endproc
_test_rle_main = _main
.segment "CHARS"
	.incbin "character.chr"
