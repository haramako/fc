	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_MATH__ = 1
.segment "math"
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
	.export _math_rand_idx
.segment "BSS"
_math_rand_idx: .res 1
