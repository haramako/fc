	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
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
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #3
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #4
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #5
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #6
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #3
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit+0
	lda 0+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit+0
	sta <F_unittest_assert_equal+1
	lda #7
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_12)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_12)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
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
	sta 0+<F_test_lzw_test_read_bit2+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit2+0
	lda 0+<F_test_lzw_test_read_bit2+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit2+0
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_20)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_20)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #16
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit2+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit2+0
	lda 0+<F_test_lzw_test_read_bit2+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit2+0
	sta <F_unittest_assert_equal+1
	lda #255
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_24)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_24)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	lda #4
	sta <FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_bit2+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_bit2+0
	lda 0+<F_test_lzw_test_read_bit2+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_bit2+0
	sta <F_unittest_assert_equal+1
	lda #0
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_28)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_28)
	sta <F_unittest_assert_equal+5
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
	sta 0+<F_test_lzw_test_read_vln+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #9
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_35)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_35)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	jsr _lzw_read_vln
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_vln+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	lda #129
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_39)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_39)
	sta <F_unittest_assert_equal+5
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
	sta 0+<F_test_lzw_test_read_vln16+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_vln16+0
	lda 0+<F_test_lzw_test_read_vln16+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_vln16+0
	sta <F_unittest_assert_equal+1
	lda #129
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_46)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_46)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	jsr _lzw_read_vln16
	lda <0+FC_FASTCALL_REG
	sta 0+<F_test_lzw_test_read_vln16+0
	lda <1+FC_FASTCALL_REG
	sta 1+<F_test_lzw_test_read_vln16+0
	lda 0+<F_test_lzw_test_read_vln16+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_read_vln16+0
	sta <F_unittest_assert_equal+1
	lda #1
	sta <F_unittest_assert_equal+2
	lda #128
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_50)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_50)
	sta <F_unittest_assert_equal+5
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
	ldx FC_SP
	lda #.LOBYTE(_test_lzw_buf)
	sta <S+2,x
	lda #.HIBYTE(_test_lzw_buf)
	sta <S+3,x
	lda #.LOBYTE(PACKED)
	sta <S+4,x
	lda #.HIBYTE(PACKED)
	sta <S+5,x
	ldx FC_SP
	jsr _lzw_unpack
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_lzw_test_unpack+0
	lda <1+S+0,x
	sta 1+<F_test_lzw_test_unpack+0
	lda #.LOBYTE(UNPACKED)
	sta <F_mem_strlen+1
	lda #.HIBYTE(UNPACKED)
	sta <F_mem_strlen+2
	jsr _mem_strlen
	sta 0+<F_test_lzw_test_unpack+2
	lda 0+<F_test_lzw_test_unpack+0
	sta <F_unittest_assert_equal+0
	lda 1+<F_test_lzw_test_unpack+0
	sta <F_unittest_assert_equal+1
	lda 0+<F_test_lzw_test_unpack+2
	sta <F_unittest_assert_equal+2
	lda #0
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_58)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_58)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	ldx FC_SP
	lda #.LOBYTE(_test_lzw_buf)
	sta <S+1,x
	lda #.HIBYTE(_test_lzw_buf)
	sta <S+2,x
	lda #.LOBYTE(UNPACKED)
	sta <S+3,x
	lda #.HIBYTE(UNPACKED)
	sta <S+4,x
	lda 0+<F_test_lzw_test_unpack+0
	sta <S+5,x
	ldx FC_SP
	jsr _mem_compare
	ldx FC_SP
	lda <0+S+0,x
	sta 0+<F_test_lzw_test_unpack+0
	sta <F_unittest_assert_equal+0
	lda #0
	sta <F_unittest_assert_equal+1
	sta <F_unittest_assert_equal+2
	sta <F_unittest_assert_equal+3
	lda #.LOBYTE(_62)
	sta <F_unittest_assert_equal+4
	lda #.HIBYTE(_62)
	sta <F_unittest_assert_equal+5
	jsr _unittest_assert_equal
	rts
PACKED:
		.byte 12,82,44,182,203,101,190,64,51,40,200,145,37,192
UNPACKED:
		.byte 72,101,108,108,111,32,72,101,108,108,111,32,72,101,108,108
		.byte 111,32,70,101,108,108,111,46,0
_58:
		.byte 108,101,110,0
_62:
		.byte 99,111,109,112,97,114,101,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_lzw"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_65)
	sta <F_stdio_print+0
	lda #.HIBYTE(_65)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_lzw_test_read_bit
	lda #.LOBYTE(_68)
	sta <F_stdio_print+0
	lda #.HIBYTE(_68)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_71)
	sta <F_stdio_print+0
	lda #.HIBYTE(_71)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_lzw_test_read_bit2
	lda #.LOBYTE(_74)
	sta <F_stdio_print+0
	lda #.HIBYTE(_74)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_77)
	sta <F_stdio_print+0
	lda #.HIBYTE(_77)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_lzw_test_read_vln
	lda #.LOBYTE(_80)
	sta <F_stdio_print+0
	lda #.HIBYTE(_80)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_83)
	sta <F_stdio_print+0
	lda #.HIBYTE(_83)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_lzw_test_read_vln16
	lda #.LOBYTE(_86)
	sta <F_stdio_print+0
	lda #.HIBYTE(_86)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #.LOBYTE(_89)
	sta <F_stdio_print+0
	lda #.HIBYTE(_89)
	sta <F_stdio_print+1
	jsr _stdio_print
	jsr _test_lzw_test_unpack
	lda #.LOBYTE(_92)
	sta <F_stdio_print+0
	lda #.HIBYTE(_92)
	sta <F_stdio_print+1
	jsr _stdio_print
	lda #0
	jsr _stdio_exit
	rts
_65:
		.byte 116,101,115,116,95,114,101,97,100,95,98,105,116,58,0
_68:
		.byte 10,0
_71:
		.byte 116,101,115,116,95,114,101,97,100,95,98,105,116,50,58,0
_74:
		.byte 10,0
_77:
		.byte 116,101,115,116,95,114,101,97,100,95,118,108,110,58,0
_80:
		.byte 10,0
_83:
		.byte 116,101,115,116,95,114,101,97,100,95,118,108,110,49,54,58
		.byte 0
_86:
		.byte 10,0
_89:
		.byte 116,101,115,116,95,117,110,112,97,99,107,58,0
_92:
		.byte 10,0
.endproc
_test_lzw_main = _main
