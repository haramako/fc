	.setcpu "6502"
	.include "macro.inc"
__MODULE_MATH__ = 1
.segment "math"
	.export _math_atan_table
.segment "math"
_math_atan_table:
	.byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	.byte 63,32,18,13,9,8,6,5,5,4,4,3,3,3,2,2
	.byte 63,45,32,23,18,15,13,11,9,8,8,7,6,6,5,5
	.byte 63,50,40,32,26,22,18,16,14,13,11,10,9,9,8,8
	.byte 63,54,45,37,32,27,23,21,18,17,15,14,13,12,11,10
	.byte 63,55,48,41,36,32,28,25,22,20,18,17,16,14,13,13
	.byte 63,57,50,45,40,35,32,28,26,23,22,20,18,17,16,15
	.byte 63,58,52,47,42,38,35,32,29,26,24,23,21,20,18,17
	.byte 63,58,54,49,45,41,37,34,32,29,27,25,23,22,21,19
	.byte 63,59,55,50,46,43,40,37,34,32,29,27,26,24,23,22
	.byte 63,59,55,52,48,45,41,39,36,34,32,30,28,26,25,23
	.byte 63,60,56,53,49,46,43,40,38,36,33,32,30,28,27,25
	.byte 63,60,57,54,50,47,45,42,40,37,35,33,32,30,28,27
	.byte 63,60,57,54,51,49,46,43,41,39,37,35,33,32,30,29
	.byte 63,61,58,55,52,50,47,45,42,40,38,36,35,33,32,30
	.byte 63,61,58,55,53,50,48,46,44,41,40,38,36,34,33,32
	.export _math_sin_table
.segment "math"
_math_sin_table:
	.byte 0,3,6,9,12,15,18,21,24,28,31,34,37,40,43,46
	.byte 48,51,54,57,60,63,65,68,71,73,76,78,81,83,85,88
	.byte 90,92,94,96,98,100,102,104,106,108,109,111,112,114,115,117
	.byte 118,119,120,121,122,123,124,124,125,126,126,127,127,127,127,127
	.export _math_rand_table
.segment "math"
_math_rand_table:
	.byte 192,92,101,225,200,225,219,159,217,179,16,237,117,8,136,176
	.byte 69,66,11,11,149,215,228,250,230,50,62,71,102,145,190,229
	.byte 115,102,244,43,32,105,132,13,43,84,72,231,238,27,178,23
	.byte 180,84,133,43,81,223,252,182,170,234,216,57,177,43,1,234
	.byte 241,206,198,3,115,200,108,9,250,107,78,16,191,127,177,58
	.byte 31,46,52,204,14,236,48,6,4,87,67,180,191,25,235,119
	.byte 171,222,244,155,22,116,78,39,124,158,51,191,48,75,52,4
	.byte 182,184,38,78,76,59,176,239,100,143,150,71,143,112,49,255
	.byte 2,149,231,174,139,159,60,111,121,186,250,98,33,251,242,99
	.byte 35,66,74,19,163,12,132,129,6,165,217,139,79,113,2,66
	.byte 83,88,80,176,89,101,144,23,16,69,205,189,98,94,120,228
	.byte 6,136,217,212,103,25,101,178,5,200,14,147,67,153,89,137
	.byte 141,31,6,61,138,143,72,108,41,44,218,132,234,121,64,69
	.byte 239,29,183,191,152,12,212,149,176,208,176,51,133,250,129,5
	.byte 220,110,100,43,166,79,209,166,2,223,56,255,82,126,73,37
	.byte 77,58,116,11,237,92,43,202,33,108,29,60,228,47,206,114
	.export _math_sin
	;;;=============================
	;;; function _math_sin
	;;;=============================
.segment "math"
.proc _math_sin
	lda 0+<FC_FASTCALL_REG+1
	cmp #128
	bpl @else_5
@then_4:
	lda 0+<FC_FASTCALL_REG+1
	cmp #64
	bpl @else_9
@then_8:
	ldy 0+<FC_FASTCALL_REG+1
	lda _math_sin_table+0,y
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_10
@else_9:
	sec
	lda #127
	sbc 0+<FC_FASTCALL_REG+1
	sta 0+<FC_FASTCALL_REG+2
	ldy 0+<FC_FASTCALL_REG+2
	lda _math_sin_table+0,y
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_10:
	jmp @end_6
@else_5:
	lda 0+<FC_FASTCALL_REG+1
	cmp #192
	bpl @else_18
@then_17:
	sec
	lda 0+<FC_FASTCALL_REG+1
	sbc #128
	sta 0+<FC_FASTCALL_REG+2
	ldy 0+<FC_FASTCALL_REG+2
	lda _math_sin_table+0,y
	sta 0+<FC_FASTCALL_REG+2
	sec
	lda #0
	sbc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_19
@else_18:
	sec
	lda #255
	sbc 0+<FC_FASTCALL_REG+1
	sta 0+<FC_FASTCALL_REG+2
	ldy 0+<FC_FASTCALL_REG+2
	lda _math_sin_table+0,y
	sta 0+<FC_FASTCALL_REG+2
	sec
	lda #0
	sbc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_19:
@end_6:
	lda #0
	sta 0+<FC_FASTCALL_REG+0
	rts
.endproc
	.export _math_atan
	;;;=============================
	;;; function _math_atan
	;;;=============================
.segment "math"
.proc _math_atan
	lda 0+<FC_FASTCALL_REG+1
	cmp #128
	bpl @else_30
@then_29:
	lda 0+<FC_FASTCALL_REG+2
	cmp #128
	bpl @else_34
@then_33:
	lda 0+<FC_FASTCALL_REG+1
	asl a
	asl a
	asl a
	asl a
	clc
	adc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+3
	ldy 0+<FC_FASTCALL_REG+3
	lda _math_atan_table+0,y
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_35
@else_34:
	lda 0+<FC_FASTCALL_REG+1
	asl a
	asl a
	asl a
	asl a
	sec
	sbc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+3
	ldy 0+<FC_FASTCALL_REG+3
	lda _math_atan_table+0,y
	sta 0+<FC_FASTCALL_REG+3
	sec
	lda #128
	sbc 0+<FC_FASTCALL_REG+3
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_35:
	jmp @end_31
@else_30:
	lda 0+<FC_FASTCALL_REG+2
	cmp #128
	bpl @else_47
@then_46:
	sec
	lda #0
	sbc 0+<FC_FASTCALL_REG+1
	sta 0+<FC_FASTCALL_REG+3
	lda 0+<FC_FASTCALL_REG+3
	asl a
	asl a
	asl a
	asl a
	clc
	adc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+3
	ldy 0+<FC_FASTCALL_REG+3
	lda _math_atan_table+0,y
	sta 0+<FC_FASTCALL_REG+3
	sec
	lda #0
	sbc 0+<FC_FASTCALL_REG+3
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_48
@else_47:
	sec
	lda #0
	sbc 0+<FC_FASTCALL_REG+1
	sta 0+<FC_FASTCALL_REG+3
	lda 0+<FC_FASTCALL_REG+3
	asl a
	asl a
	asl a
	asl a
	sec
	sbc 0+<FC_FASTCALL_REG+2
	sta 0+<FC_FASTCALL_REG+3
	ldy 0+<FC_FASTCALL_REG+3
	lda _math_atan_table+0,y
	sta 0+<FC_FASTCALL_REG+3
	clc
	lda #128
	adc 0+<FC_FASTCALL_REG+3
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_48:
@end_31:
	lda #0
	sta 0+<FC_FASTCALL_REG+0
	rts
.endproc
	.export _math_rand_idx
.segment "BSS"
_math_rand_idx: .res 1
	.export _math_rand
	;;;=============================
	;;; function _math_rand
	;;;=============================
.segment "math"
.proc _math_rand
	clc
	lda 0+_math_rand_idx
	adc #1
	sta 0+_math_rand_idx
	ldy 0+_math_rand_idx
	lda _math_rand_table+0,y
	sta 0+<FC_FASTCALL_REG+0
	rts
.endproc
	.export _math_sign
	;;;=============================
	;;; function _math_sign
	;;;=============================
.segment "math"
.proc _math_sign
	lda 0+<FC_FASTCALL_REG+1
	cmp #0
	bpl @else_66
@then_65:
	lda #255
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_67
@else_66:
	lda #0
	cmp 0+<FC_FASTCALL_REG+1
	bpl @else_70
@then_69:
	lda #1
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_71
@else_70:
	lda #0
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_71:
@end_67:
.endproc
	.export _math_abs
	;;;=============================
	;;; function _math_abs
	;;;=============================
.segment "math"
.proc _math_abs
	lda 0+<FC_FASTCALL_REG+1
	cmp #128
	bmi @else_74
@then_73:
	sec
	lda #0
	sbc 0+<FC_FASTCALL_REG+1
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_75
@else_74:
	lda 0+<FC_FASTCALL_REG+1
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_75:
.endproc
