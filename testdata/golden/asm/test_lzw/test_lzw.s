	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_LZW__ = 1
.segment "test_lzw"
	.include "_lzw.inc"
	.include "_mem.inc"
	.include "_unittest.inc"
	.include "_stdio.inc"
	.export _test_lzw_test_read_bit
	;;;=============================
	;;; function _test_lzw_test_read_bit
	;;;=============================
.segment "test_lzw"
.proc _test_lzw_test_read_bit
	lda #.LOBYTE(_2)
	sta 0+_lzw_addr
	lda #.HIBYTE(_2)
	sta 1+_lzw_addr
	lda #0
	sta 0+_lzw_bpos
	lda #0
	sta 0+<S+0,x
@begin_4:
	lda 0+<S+0,x
	cmp #8
	bcs @else_7
@then_6:
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda <1+FC_FASTCALL_REG
	sta 1+<L+0
	lda 0+<L+0
	sta <S+1,x
	lda 1+<L+0
	sta <S+2,x
	lda 0+<S+0,x
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_12)
	sta <S+5,x
	lda #.HIBYTE(_12)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	clc
	lda 0+<S+0,x
	adc #1
	sta 0+<S+0,x
	jmp @end_8
@else_7:
	jmp @end_5
@end_8:
	jmp @begin_4
@end_5:
	rts
_2:
		.byte 5,57,119
_12:
		.byte 114,101,97,100,95,98,105,116,0
.endproc
	.export _test_lzw_test_read_bit2
	;;;=============================
	;;; function _test_lzw_test_read_bit2
	;;;=============================
.segment "test_lzw"
.proc _test_lzw_test_read_bit2
	lda #.LOBYTE(_16)
	sta 0+_lzw_addr
	lda #.HIBYTE(_16)
	sta 1+_lzw_addr
	lda #0
	sta 0+_lzw_bpos
	lda #4
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda <1+FC_FASTCALL_REG
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_20)
	sta <S+4,x
	lda #.HIBYTE(_20)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #16
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda <1+FC_FASTCALL_REG
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	lda #255
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_24)
	sta <S+4,x
	lda #.HIBYTE(_24)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #4
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda <1+FC_FASTCALL_REG
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_28)
	sta <S+4,x
	lda #.HIBYTE(_28)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_16:
		.byte 15,255,240
_20:
		.byte 114,101,97,100,95,98,105,116,50,40,48,41,0
_24:
		.byte 114,101,97,100,95,98,105,116,50,40,49,41,0
_28:
		.byte 114,101,97,100,95,98,105,116,50,40,50,41,0
.endproc
	.export _test_lzw_test_read_vln
	;;;=============================
	;;; function _test_lzw_test_read_vln
	;;;=============================
.segment "test_lzw"
.proc _test_lzw_test_read_vln
	lda #.LOBYTE(_31)
	sta 0+_lzw_addr
	lda #.HIBYTE(_31)
	sta 1+_lzw_addr
	lda #0
	sta 0+_lzw_bpos
	jsr _lzw_read_vln
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #9
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_35)
	sta <S+4,x
	lda #.HIBYTE(_35)
	sta <S+5,x
	jsr _unittest_assert_equal
	jsr _lzw_read_vln
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #129
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_39)
	sta <S+4,x
	lda #.HIBYTE(_39)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_31:
		.byte 78,4
_35:
		.byte 114,101,97,100,95,118,108,110,40,48,41,0
_39:
		.byte 114,101,97,100,95,118,108,110,40,49,41,0
.endproc
	.export _test_lzw_test_read_vln16
	;;;=============================
	;;; function _test_lzw_test_read_vln16
	;;;=============================
.segment "test_lzw"
.proc _test_lzw_test_read_vln16
	lda #.LOBYTE(_42)
	sta 0+_lzw_addr
	lda #.HIBYTE(_42)
	sta 1+_lzw_addr
	lda #0
	sta 0+_lzw_bpos
	jsr _lzw_read_vln16
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda <1+FC_FASTCALL_REG
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	lda #129
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_46)
	sta <S+4,x
	lda #.HIBYTE(_46)
	sta <S+5,x
	jsr _unittest_assert_equal
	jsr _lzw_read_vln16
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda <1+FC_FASTCALL_REG
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #128
	sta <S+3,x
	lda #.LOBYTE(_50)
	sta <S+4,x
	lda #.HIBYTE(_50)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_42:
		.byte 64,224,0,64
_46:
		.byte 114,101,97,100,95,118,108,110,49,54,40,48,41,0
_50:
		.byte 114,101,97,100,95,118,108,110,49,54,40,49,41,0
.endproc
	.export _test_lzw_buf
.segment "BSS"
_test_lzw_buf: .res 256
	.export _test_lzw_test_unpack
	;;;=============================
	;;; function _test_lzw_test_unpack
	;;;=============================
.segment "test_lzw"
.proc _test_lzw_test_unpack
	lda #.LOBYTE(_test_lzw_buf)
	sta <S+4,x
	lda #.HIBYTE(_test_lzw_buf)
	sta <S+5,x
	lda #.LOBYTE(PACKED)
	sta <S+6,x
	lda #.HIBYTE(PACKED)
	sta <S+7,x
	inx
	inx
	jsr _lzw_unpack
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
	lda #.LOBYTE(UNPACKED)
	sta <FC_FASTCALL_REG+1
	lda #.HIBYTE(UNPACKED)
	sta <FC_FASTCALL_REG+2
	jsr _mem_strlen
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_57)
	sta <S+6,x
	lda #.HIBYTE(_57)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	lda #.LOBYTE(_test_lzw_buf)
	sta <S+3,x
	lda #.HIBYTE(_test_lzw_buf)
	sta <S+4,x
	lda #.LOBYTE(UNPACKED)
	sta <S+5,x
	lda #.HIBYTE(UNPACKED)
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
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #0
	sta <S+5,x
	lda #.LOBYTE(_61)
	sta <S+6,x
	lda #.HIBYTE(_61)
	sta <S+7,x
	inx
	inx
	jsr _unittest_assert_equal
	dex
	dex
	rts
PACKED:
		.byte 12,82,44,182,203,101,190,64,51,40,200,145,37,192
UNPACKED:
		.byte 72,101,108,108,111,32,72,101,108,108,111,32,72,101,108,108
		.byte 111,32,70,101,108,108,111,46,0
_57:
		.byte 108,101,110,0
_61:
		.byte 99,111,109,112,97,114,101,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_lzw"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_64)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_64)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_lzw_test_read_bit
	lda #.LOBYTE(_67)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_67)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_70)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_70)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_lzw_test_read_bit2
	lda #.LOBYTE(_73)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_73)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_76)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_76)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_lzw_test_read_vln
	lda #.LOBYTE(_79)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_79)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_82)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_82)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_lzw_test_read_vln16
	lda #.LOBYTE(_85)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_85)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_88)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_88)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_lzw_test_unpack
	lda #.LOBYTE(_91)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_91)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_64:
		.byte 116,101,115,116,95,114,101,97,100,95,98,105,116,58,0
_67:
		.byte 10,0
_70:
		.byte 116,101,115,116,95,114,101,97,100,95,98,105,116,50,58,0
_73:
		.byte 10,0
_76:
		.byte 116,101,115,116,95,114,101,97,100,95,118,108,110,58,0
_79:
		.byte 10,0
_82:
		.byte 116,101,115,116,95,114,101,97,100,95,118,108,110,49,54,58
		.byte 0
_85:
		.byte 10,0
_88:
		.byte 116,101,115,116,95,117,110,112,97,99,107,58,0
_91:
		.byte 10,0
.endproc
_test_lzw_main = _main
