	.setcpu "6502"
	.include "macro.inc"
__MODULE_TEST_VAR__ = 1
.segment "test_var"
	.include "_unittest.inc"
	.export _test_var_array
.segment "BSS"
_test_var_array: .res 10
	.export _test_var_CONST
.segment "test_var"
_test_var_CONST:
	.byte 0,1,2,3,4,5,6,7,8,9
	.export _D2
	;;;=============================
	;;; function $2
	;;;=============================
.segment "test_var"
.proc _D2
	clc
	lda 0+<S+1,x
	adc #2
	sta 0+<S+0,x
	rts
.endproc
_test_var_add2 = _D2
	.export _test_var_test_const
	;;;=============================
	;;; function _test_var_test_const
	;;;=============================
.segment "test_var"
.proc _test_var_test_const
	lda #1
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_8)
	sta <S+4,x
	lda #.HIBYTE(_8)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #254
	sta <S+0,x
	lda #255
	sta <S+1,x
	lda #254
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_11)
	sta <S+4,x
	lda #.HIBYTE(_11)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #255
	sta <S+0,x
	lda #255
	sta <S+1,x
	lda #255
	sta <S+2,x
	lda #255
	sta <S+3,x
	lda #.LOBYTE(_14)
	sta <S+4,x
	lda #.HIBYTE(_14)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_17)
	sta <S+4,x
	lda #.HIBYTE(_17)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #2
	lda ARRAY+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #3
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_22)
	sta <S+4,x
	lda #.HIBYTE(_22)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #4
	lda STRING+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_27)
	sta <S+4,x
	lda #.HIBYTE(_27)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
ARRAY:
		.byte 1,2,3
STRING:
		.byte 104,111,103,101,0
_8:
		.byte 99,111,110,115,116,0
_11:
		.byte 99,111,110,115,116,50,0
_14:
		.byte 99,111,110,115,116,0
_17:
		.byte 99,111,110,115,116,50,0
_22:
		.byte 99,111,110,115,116,91,93,0
_27:
		.byte 115,116,114,105,110,103,91,93,0
.endproc
	.export _test_var_test_pointer
	;;;=============================
	;;; function _test_var_test_pointer
	;;;=============================
.segment "test_var"
.proc _test_var_test_pointer
	ldy #1
	lda _test_var_CONST+0,y
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #1
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_32)
	sta <S+5,x
	lda #.HIBYTE(_32)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	ldy #1
	lda #1
	sta _test_var_array+0,y
	ldy #1
	lda _test_var_array+0,y
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #1
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_38)
	sta <S+5,x
	lda #.HIBYTE(_38)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	lda #2
	sta 0+<S+0,x
	ldy 0+<S+0,x
	lda _test_var_CONST+0,y
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #2
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_43)
	sta <S+5,x
	lda #.HIBYTE(_43)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	ldy 0+<S+0,x
	lda #2
	sta _test_var_array+0,y
	ldy 0+<S+0,x
	lda _test_var_array+0,y
	sta <S+1,x
	lda #0
	sta <S+2,x
	lda #2
	sta <S+3,x
	lda #0
	sta <S+4,x
	lda #.LOBYTE(_49)
	sta <S+5,x
	lda #.HIBYTE(_49)
	sta <S+6,x
	inx
	jsr _unittest_assert_equal
	dex
	rts
_32:
		.byte 99,111,110,115,116,91,93,0
_38:
		.byte 97,114,114,97,121,91,93,0
_43:
		.byte 99,111,110,115,116,91,105,93,0
_49:
		.byte 97,114,114,97,121,91,105,93,0
.endproc
	.export _test_var_test_array
	;;;=============================
	;;; function _test_var_test_array
	;;;=============================
.segment "test_var"
.proc _test_var_test_array
	lda #0
	sta 0+<L+0
	ldy 0+<L+0
	sty <reg+0
	clc
	lda #.LOBYTE(_test_var_array)
	adc <reg+0
	sta 0+<L+2
	lda #.HIBYTE(_test_var_array)
	adc #0
	sta 1+<L+2
	clc
	lda 0+<L+0
	adc #1
	sta 0+<L+4
	lda 0+<L+2
	sta <reg+0
	lda 1+<L+2
	sta <reg+1
	lda 0+<L+4
	ldy #0
	sta (reg),y
	clc
	lda 0+<L+0
	adc #1
	sta 0+<L+0
	ldy 0+<L+0
	sty <reg+0
	clc
	lda #.LOBYTE(_test_var_array)
	adc <reg+0
	sta 0+<L+2
	lda #.HIBYTE(_test_var_array)
	adc #0
	sta 1+<L+2
	clc
	lda 0+<L+0
	adc #1
	sta 0+<L+4
	lda 0+<L+2
	sta <reg+0
	lda 1+<L+2
	sta <reg+1
	lda 0+<L+4
	ldy #0
	sta (reg),y
	clc
	lda 0+<L+0
	adc #1
	sta 0+<L+0
	ldy 0+<L+0
	sty <reg+0
	clc
	lda #.LOBYTE(_test_var_array)
	adc <reg+0
	sta 0+<L+2
	lda #.HIBYTE(_test_var_array)
	adc #0
	sta 1+<L+2
	clc
	lda 0+<L+0
	adc #1
	sta 0+<L+4
	lda 0+<L+2
	sta <reg+0
	lda 1+<L+2
	sta <reg+1
	lda 0+<L+4
	ldy #0
	sta (reg),y
	ldy #0
	lda _test_var_array+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #1
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_62)
	sta <S+4,x
	lda #.HIBYTE(_62)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #1
	lda _test_var_array+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #2
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_67)
	sta <S+4,x
	lda #.HIBYTE(_67)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #2
	lda _test_var_array+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #3
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_72)
	sta <S+4,x
	lda #.HIBYTE(_72)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_62:
		.byte 97,91,48,93,0
_67:
		.byte 97,91,49,93,0
_72:
		.byte 97,91,50,93,0
.endproc
	.export _test_var_add1
	;;;=============================
	;;; function _test_var_add1
	;;;=============================
.segment "test_var"
.proc _test_var_add1
	clc
	lda 0+<S+1,x
	adc #1
	sta 0+<S+0,x
	rts
.endproc
	.export _test_var_mul2
	;;;=============================
	;;; function _test_var_mul2
	;;;=============================
.segment "test_var"
.proc _test_var_mul2
	lda 0+<S+1,x
	asl a
	sta 0+<S+0,x
	rts
.endproc
	.export _test_var_FUNC_TABLE
.segment "test_var"
_test_var_FUNC_TABLE:
	.word _test_var_add1,_test_var_mul2
	.export _test_var_test_func_pointer
	;;;=============================
	;;; function _test_var_test_func_pointer
	;;;=============================
.segment "test_var"
.proc _test_var_test_func_pointer
	lda #.LOBYTE(_test_var_add1)
	sta 0+<L+0
	lda #.HIBYTE(_test_var_add1)
	sta 1+<L+0
	lda #10
	sta <S+1,x
	lda 0+<L+0
	sta <reg+0
	lda 1+<L+0
	sta <reg+1
	jsr jsr_reg
	lda <0+S+0,x
	sta 0+<L+2
	lda 0+<L+2
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #11
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_78)
	sta <S+4,x
	lda #.HIBYTE(_78)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #0
	asl a
	tay
	lda _test_var_FUNC_TABLE+0,y
	sta 0+<L+2
	lda _test_var_FUNC_TABLE+1,y
	sta 1+<L+2
	lda #10
	sta <S+1,x
	lda 0+<L+2
	sta <reg+0
	lda 1+<L+2
	sta <reg+1
	jsr jsr_reg
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #11
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_84)
	sta <S+4,x
	lda #.HIBYTE(_84)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	asl a
	tay
	lda _test_var_FUNC_TABLE+0,y
	sta 0+<L+2
	lda _test_var_FUNC_TABLE+1,y
	sta 1+<L+2
	lda #10
	sta <S+1,x
	lda 0+<L+2
	sta <reg+0
	lda 1+<L+2
	sta <reg+1
	jsr jsr_reg
	lda <0+S+0,x
	sta 0+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #20
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_90)
	sta <S+4,x
	lda #.HIBYTE(_90)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <S+1,x
	jsr _D2
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
	lda #.LOBYTE(_94)
	sta <S+4,x
	lda #.HIBYTE(_94)
	sta <S+5,x
	jsr _unittest_assert_equal
	lda #1
	sta <S+1,x
	jsr _D96
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
	lda #.LOBYTE(_99)
	sta <S+4,x
	lda #.HIBYTE(_99)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_78:
		.byte 102,40,41,0
_84:
		.byte 70,85,78,67,95,84,65,66,76,69,91,48,93,40,41,0
_90:
		.byte 70,85,78,67,95,84,65,66,76,69,91,49,93,40,41,0
_94:
		.byte 108,97,109,98,100,97,0
_99:
		.byte 108,97,109,98,100,97,32,108,105,116,101,114,97,108,0
.endproc
	.export _test_var_test_escape
	;;;=============================
	;;; function _test_var_test_escape
	;;;=============================
.segment "test_var"
.proc _test_var_test_escape
	ldy #0
	lda STR+0,y
	sta 0+<L+0
	bpl @1
	lda #255
	jmp @2
@1:
	lda #0
@2:
	sta 1+<L+0
	lda 0+<L+0
	sta <S+0,x
	lda 1+<L+0
	sta <S+1,x
	lda #10
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_106)
	sta <S+4,x
	lda #.HIBYTE(_106)
	sta <S+5,x
	jsr _unittest_assert_equal
	ldy #1
	lda STR+0,y
	sta <S+0,x
	lda #0
	sta <S+1,x
	lda #255
	sta <S+2,x
	lda #0
	sta <S+3,x
	lda #.LOBYTE(_111)
	sta <S+4,x
	lda #.HIBYTE(_111)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
STR:
		.byte 10,255,0
_106:
		.byte 101,115,99,97,112,101,32,110,0
_111:
		.byte 101,115,99,97,112,101,32,120,0
.endproc
	.export _test_var_segmented_function
	;;;=============================
	;;; function _test_var_segmented_function
	;;;=============================
.segment "CODE"
.proc _test_var_segmented_function
	lda #1
	sta 0+<S+0,x
	rts
.endproc
	.export _test_var_test_segment_option
	;;;=============================
	;;; function _test_var_test_segment_option
	;;;=============================
.segment "test_var"
.proc _test_var_test_segment_option
	jsr _test_var_segmented_function
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
	lda #.LOBYTE(_115)
	sta <S+4,x
	lda #.HIBYTE(_115)
	sta <S+5,x
	jsr _unittest_assert_equal
	rts
_115:
		.byte 115,101,103,109,101,110,116,101,100,32,102,117,110,99,116,105
		.byte 111,110,0
.endproc
	.export _main
	;;;=============================
	;;; function _main
	;;;=============================
.segment "test_var"
.proc _main
	jsr _stdio_init
	lda #.LOBYTE(_118)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_118)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_var_test_const
	lda #.LOBYTE(_121)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_121)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_124)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_124)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_var_test_pointer
	lda #.LOBYTE(_127)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_127)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_130)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_130)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_var_test_array
	lda #.LOBYTE(_133)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_133)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_136)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_136)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_var_test_func_pointer
	lda #.LOBYTE(_139)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_139)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_142)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_142)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_var_test_escape
	lda #.LOBYTE(_145)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_145)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_148)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_148)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	jsr _test_var_test_segment_option
	lda #.LOBYTE(_151)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_151)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #0
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	rts
_118:
		.byte 116,101,115,116,95,99,111,110,115,116,58,0
_121:
		.byte 10,0
_124:
		.byte 116,101,115,116,95,112,111,105,110,116,101,114,58,0
_127:
		.byte 10,0
_130:
		.byte 116,101,115,116,95,97,114,114,97,121,58,0
_133:
		.byte 10,0
_136:
		.byte 116,101,115,116,95,102,117,110,99,95,112,111,105,110,116,101
		.byte 114,58,0
_139:
		.byte 10,0
_142:
		.byte 116,101,115,116,95,101,115,99,97,112,101,58,0
_145:
		.byte 10,0
_148:
		.byte 116,101,115,116,95,115,101,103,109,101,110,116,95,111,112,116
		.byte 105,111,110,58,0
_151:
		.byte 10,0
.endproc
_test_var_main = _main
	.export _D96
	;;;=============================
	;;; function $96
	;;;=============================
.segment "test_var"
.proc _D96
	clc
	lda 0+<S+1,x
	adc #2
	sta 0+<S+0,x
	rts
.endproc
.segment "CHARS"
	.incbin "character.chr"
