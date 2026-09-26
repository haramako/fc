	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
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
	ldx FC_SP
	lda #.LOBYTE(_test_rle_dest)
	sta <S+2,x
	lda #.HIBYTE(_test_rle_dest)
	sta <S+3,x
	lda #.LOBYTE(_test_rle_src)
	sta <S+4,x
	lda #.HIBYTE(_test_rle_src)
	sta <S+5,x
	ldx FC_SP
	jsr _rle_unpack
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_rle_test_rle+0
	lda <1+S+0,x
	sta 1+<F_test_rle_test_rle+0
	lda 0+<F_test_rle_test_rle+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_rle_test_rle+0
	sta <F_unittest_assert_equal+1
	lda #8
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_5)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_5)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_rle_dest)
	sta <FC_FASTCALL_REG+1
	lda #.HIBYTE(_test_rle_dest)
	sta <FC_FASTCALL_REG+2
	lda #.LOBYTE(_10)
	sta <FC_FASTCALL_REG+3
	lda #.HIBYTE(_10)
	sta <FC_FASTCALL_REG+4
	lda 0+<F_test_rle_test_rle+0
	sta <FC_FASTCALL_REG+5
	lda 1+<F_test_rle_test_rle+0
	sta <FC_FASTCALL_REG+6
	jsr _mem_compare
	lda <0+FC_FASTCALL_REG
	bne @1
	lda #1
	sta 0+<F_test_rle_test_rle+2
	jmp @2
@1:
	lda #0
	sta 0+<F_test_rle_test_rle+2
@2:
	lda 0+<F_test_rle_test_rle+2
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_13)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_13)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true__frame
	ldx FC_SP
	lda #.LOBYTE(_test_rle_dest)
	sta <S+2,x
	lda #.HIBYTE(_test_rle_dest)
	sta <S+3,x
	lda #.LOBYTE(_test_rle_src2)
	sta <S+4,x
	lda #.HIBYTE(_test_rle_src2)
	sta <S+5,x
	ldx FC_SP
	jsr _rle_unpack
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_rle_test_rle+0
	lda <1+S+0,x
	sta 1+<F_test_rle_test_rle+0
	lda 0+<F_test_rle_test_rle+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_rle_test_rle+0
	sta <F_unittest_assert_equal+1
	lda #8
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_17)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_17)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_rle_dest)
	sta <FC_FASTCALL_REG+1
	lda #.HIBYTE(_test_rle_dest)
	sta <FC_FASTCALL_REG+2
	lda #.LOBYTE(_22)
	sta <FC_FASTCALL_REG+3
	lda #.HIBYTE(_22)
	sta <FC_FASTCALL_REG+4
	lda 0+<F_test_rle_test_rle+0
	sta <FC_FASTCALL_REG+5
	lda 1+<F_test_rle_test_rle+0
	sta <FC_FASTCALL_REG+6
	jsr _mem_compare
	lda <0+FC_FASTCALL_REG
	bne @3
	lda #1
	sta 0+<F_test_rle_test_rle+0
	jmp @4
@3:
	lda #0
	sta 0+<F_test_rle_test_rle+0
@4:
	lda 0+<F_test_rle_test_rle+0
	sta <F_unittest_assert_true+0
	lda #.LOBYTE(_25)
	sta <F_unittest_assert_true+1
	lda #.HIBYTE(_25)
	sta <F_unittest_assert_true+2
	jsr _unittest_assert_true__frame
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
	lda #.LOBYTE(_28)
	sta 0+<F_main+0
	lda #.HIBYTE(_28)
	sta 1+<F_main+0
	lda 0+<F_main+0
	sta 0+_stdio_EMU_ADDR
	lda 1+<F_main+0
	sta 1+_stdio_EMU_ADDR
	lda #1
	sta 0+_stdio_EMU_PRINT
	jsr _test_rle_test_rle
	lda #.LOBYTE(_31)
	sta 0+<F_main+0
	lda #.HIBYTE(_31)
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
_28:
		.byte 116,101,115,116,95,114,108,101,58,0
_31:
		.byte 10,0
.endproc
_test_rle_main = _main
.segment "CHARS"
	.incbin "character.chr"
