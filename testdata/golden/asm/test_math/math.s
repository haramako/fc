	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
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
	.export _math_sin
	.export _math_sin__frame
	;;;=============================
	;;; function _math_sin
	;;;=============================
.segment "math"
.proc _math_sin__frame
	lda <F_math_sin+1
	.endproc
.proc _math_sin
	sta <F_math_sin+1
	lda 0+<F_math_sin+1
	bmi @else_5
	sec
	sbc #64
	bvc @6
	eor #$80
@6:
	bpl @else_9
	ldy 0+<F_math_sin+1
	lda _math_sin_table+0,y
	sta 0+<F_math_sin+0
	rts
@else_9:
	sec
	lda #127
	sbc 0+<F_math_sin+1
	tay
	lda _math_sin_table+0,y
	sta 0+<F_math_sin+0
	rts
@else_5:
	lda 0+<F_math_sin+1
	sec
	sbc #192
	bvc @9
	eor #$80
@9:
	bpl @else_18
	sec
	lda 0+<F_math_sin+1
	sbc #128
	tay
	lda _math_sin_table+0,y
	sta 0+<F_math_sin+2
	sec
	lda #0
	sbc 0+<F_math_sin+2
	sta 0+<F_math_sin+0
	rts
@else_18:
	sec
	lda #255
	sbc 0+<F_math_sin+1
	tay
	lda _math_sin_table+0,y
	sta 0+<F_math_sin+2
	sec
	lda #0
	sbc 0+<F_math_sin+2
	sta 0+<F_math_sin+0
	rts
.endproc
	.export _math_atan
	.export _math_atan__frame
	.export _math_atan__a
	;;;=============================
	;;; function _math_atan
	;;;=============================
.segment "math"
.proc _math_atan__frame
	lda <F_math_atan+2
	.endproc
.proc _math_atan__a
	ldy <F_math_atan+1
	.endproc
.proc _math_atan
	sty <F_math_atan+1
	sta <F_math_atan+2
	lda 0+<F_math_atan+1
	bmi @else_30
	lda 0+<F_math_atan+2
	bmi @else_34
	lda 0+<F_math_atan+1
	asl a
	asl a
	asl a
	asl a
	clc
	adc 0+<F_math_atan+2
	tay
	lda _math_atan_table+0,y
	sta 0+<F_math_atan+0
	rts
@else_34:
	lda 0+<F_math_atan+1
	asl a
	asl a
	asl a
	asl a
	sec
	sbc 0+<F_math_atan+2
	tay
	sec
	lda #128
	sbc _math_atan_table+0,y
	sta 0+<F_math_atan+0
	rts
@else_30:
	lda 0+<F_math_atan+2
	bmi @else_47
	sec
	lda #0
	sbc 0+<F_math_atan+1
	sta 0+<F_math_atan+3
	asl a
	asl a
	asl a
	asl a
	clc
	adc 0+<F_math_atan+2
	tay
	lda _math_atan_table+0,y
	sta 0+<F_math_atan+3
	sec
	lda #0
	sbc 0+<F_math_atan+3
	sta 0+<F_math_atan+0
	rts
@else_47:
	sec
	lda #0
	sbc 0+<F_math_atan+1
	sta 0+<F_math_atan+3
	asl a
	asl a
	asl a
	asl a
	sec
	sbc 0+<F_math_atan+2
	tay
	lda _math_atan_table+0,y
	clc
	adc #128
	sta 0+<F_math_atan+0
	rts
.endproc
	.export _math_rand_idx
.segment "BSS"
_math_rand_idx: .res 1
