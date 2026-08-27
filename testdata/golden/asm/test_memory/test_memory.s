	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_MEMORY__ = 1
.segment "test_memory"
	.include "_unittest.inc"
	.include "_mem.inc"
	.export _test_memory_str
.segment "BSS"
_test_memory_str: .res 12
	.export _test_memory_buf1
.segment "BSS"
_test_memory_buf1: .res 512
	.export _test_memory_buf2
.segment "BSS"
_test_memory_buf2: .res 512
	.export _test_memory_test_copy
	;;;=============================
	;;; function _test_memory_test_copy
	;;;=============================
.segment "test_memory"
.proc _test_memory_test_copy
	ldy #0
	lda #1
	sta _test_memory_buf1+0,y
	ldy #1
	lda #2
	sta _test_memory_buf1+0,y
	ldy #255
	lda #3
	sta _test_memory_buf1+0,y
	lda #0
	sta <reg+0
	lda #1
	sta <reg+1
	lda <reg+0
	clc
	adc #.LOBYTE(_test_memory_buf1)
	sta 0+<L+0
	lda <reg+1
	adc #.HIBYTE(_test_memory_buf1)
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	lda #4
	ldy #0
	sta (reg),y
	lda #1
	sta <reg+0
	lda #1
	sta <reg+1
	lda <reg+0
	clc
	adc #.LOBYTE(_test_memory_buf1)
	sta 0+<L+0
	lda <reg+1
	adc #.HIBYTE(_test_memory_buf1)
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	lda #5
	ldy #0
	sta (reg),y
	lda #255
	sta <reg+0
	lda #1
	sta <reg+1
	lda <reg+0
	clc
	adc #.LOBYTE(_test_memory_buf1)
	sta 0+<L+0
	lda <reg+1
	adc #.HIBYTE(_test_memory_buf1)
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	lda #6
	ldy #0
	sta (reg),y
	lda #.LOBYTE(_test_memory_buf2)
	sta <S+0,x
	lda #.HIBYTE(_test_memory_buf2)
	sta <S+1,x
	lda #.LOBYTE(_test_memory_buf1)
	sta <S+2,x
	lda #.HIBYTE(_test_memory_buf1)
	sta <S+3,x
	lda #2
	sta <S+4,x
	lda #0
	sta <S+5,x
	jsr _mem_copy
	lda #.LOBYTE(_test_memory_buf1)
	sta <S+1,x
	lda #.HIBYTE(_test_memory_buf1)
	sta <S+2,x
	lda #.LOBYTE(_test_memory_buf2)
	sta <S+3,x
	lda #.HIBYTE(_test_memory_buf2)
	sta <S+4,x
	lda #2
	sta <S+5,x
	jsr _mem_compare
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_9)
	sta <S+4,x
	lda #.HIBYTE(_9)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_memory_buf2)
	sta <S+0,x
	lda #.HIBYTE(_test_memory_buf2)
	sta <S+1,x
	lda #.LOBYTE(_test_memory_buf1)
	sta <S+2,x
	lda #.HIBYTE(_test_memory_buf1)
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #2
	sta <S+5,x
	jsr _mem_copy
	ldy #255
	lda _test_memory_buf2+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #3
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_14)
	sta <S+4,x
	lda #.HIBYTE(_14)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #0
	sta <reg+0
	lda #1
	sta <reg+1
	lda <reg+0
	clc
	adc #.LOBYTE(_test_memory_buf2)
	sta 0+<L+0
	lda <reg+1
	adc #.HIBYTE(_test_memory_buf2)
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	ldy #0
	lda (reg),y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #4
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_19)
	sta <S+4,x
	lda #.HIBYTE(_19)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <reg+0
	lda #1
	sta <reg+1
	lda <reg+0
	clc
	adc #.LOBYTE(_test_memory_buf2)
	sta 0+<L+0
	lda <reg+1
	adc #.HIBYTE(_test_memory_buf2)
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	ldy #0
	lda (reg),y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #5
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_24)
	sta <S+4,x
	lda #.HIBYTE(_24)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <reg+0
	lda #1
	sta <reg+1
	lda <reg+0
	clc
	adc #.LOBYTE(_test_memory_buf2)
	sta 0+<L+0
	lda <reg+1
	adc #.HIBYTE(_test_memory_buf2)
	sta 1+<L+0
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	ldy #0
	lda (reg),y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #6
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_29)
	sta <S+4,x
	lda #.HIBYTE(_29)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_memory_buf1)
	sta <S+1,x
	lda #.HIBYTE(_test_memory_buf1)
	sta <S+2,x
	lda #.LOBYTE(_test_memory_buf2)
	sta <S+3,x
	lda #.HIBYTE(_test_memory_buf2)
	sta <S+4,x
	lda #0
	sta <S+5,x
	jsr _mem_compare
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_33)
	sta <S+4,x
	lda #.HIBYTE(_33)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #255
	lda #0
	sta _test_memory_buf2+0,y
	lda #.LOBYTE(_test_memory_buf1)
	sta <S+1,x
	lda #.HIBYTE(_test_memory_buf1)
	sta <S+2,x
	lda #.LOBYTE(_test_memory_buf2)
	sta <S+3,x
	lda #.HIBYTE(_test_memory_buf2)
	sta <S+4,x
	lda #0
	sta <S+5,x
	jsr _mem_compare
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
	lda #.LOBYTE(_38)
	sta <S+4,x
	lda #.HIBYTE(_38)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_9:
		.byte 99,111,112,121,0
_14:
		.byte 99,111,112,121,0
_19:
		.byte 99,111,112,121,0
_24:
		.byte 99,111,112,121,0
_29:
		.byte 99,111,112,121,0
_33:
		.byte 99,111,109,112,97,114,101,32,61,61,32,48,0
_38:
		.byte 99,111,109,97,112,114,101,32,61,61,32,49,0
.endproc
	.export _test_memory_test_strcpy
	;;;=============================
	;;; function _test_memory_test_strcpy
	;;;=============================
.segment "test_memory"
.proc _test_memory_test_strcpy
	lda #.LOBYTE(_test_memory_str)
	sta <FC_FASTCALL_REG+1
	lda #.HIBYTE(_test_memory_str)
	sta <FC_FASTCALL_REG+2
	lda #.LOBYTE(_42)
	sta <FC_FASTCALL_REG+3
	lda #.HIBYTE(_42)
	sta <FC_FASTCALL_REG+4
	jsr _mem_strcpy
	lda <0+FC_FASTCALL_REG
	sta 0+<L+0
	lda 0+<L+0
	sta 0+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #5
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_45)
	sta <S+4,x
	lda #.HIBYTE(_45)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_42:
		.byte 65,66,67,68,69,0
_45:
		.byte 115,116,114,99,112,121,0
.endproc
	.export _test_memory_test_set
	;;;=============================
	;;; function _test_memory_test_set
	;;;=============================
.segment "test_memory"
.proc _test_memory_test_set
	lda #.LOBYTE(_test_memory_buf1)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_test_memory_buf1)
	sta <FC_FASTCALL_REG+1
	lda #0
	sta <FC_FASTCALL_REG+2
	lda #8
	sta <FC_FASTCALL_REG+3
	jsr _mem_set
	ldy #0
	lda _test_memory_buf1+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_50)
	sta <S+4,x
	lda #.HIBYTE(_50)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #7
	lda _test_memory_buf1+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_55)
	sta <S+4,x
	lda #.HIBYTE(_55)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #.LOBYTE(_test_memory_buf1)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_test_memory_buf1)
	sta <FC_FASTCALL_REG+1
	lda #1
	sta <FC_FASTCALL_REG+2
	lda #7
	sta <FC_FASTCALL_REG+3
	jsr _mem_set
	ldy #0
	lda _test_memory_buf1+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_60)
	sta <S+4,x
	lda #.HIBYTE(_60)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #6
	lda _test_memory_buf1+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_65)
	sta <S+4,x
	lda #.HIBYTE(_65)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #7
	lda _test_memory_buf1+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_70)
	sta <S+4,x
	lda #.HIBYTE(_70)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_50:
		.byte 91,48,93,61,48,0
_55:
		.byte 91,55,93,61,48,0
_60:
		.byte 91,48,93,61,49,0
_65:
		.byte 91,54,93,61,49,0
_70:
		.byte 91,55,93,61,48,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_memory"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_73)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_73)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_memory_test_copy
	lda #.LOBYTE(_76)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_76)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_79)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_79)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_memory_test_strcpy
	lda #.LOBYTE(_82)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_82)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_85)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_85)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_memory_test_set
	lda #.LOBYTE(_88)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_88)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_73:
		.byte 116,101,115,116,95,99,111,112,121,58,0
_76:
		.byte 10,0
_79:
		.byte 116,101,115,116,95,115,116,114,99,112,121,58,0
_82:
		.byte 10,0
_85:
		.byte 116,101,115,116,95,115,101,116,58,0
_88:
		.byte 10,0
.endproc
_test_memory_main = _main
.segment "CHARS"
	.incbin "character.chr"
