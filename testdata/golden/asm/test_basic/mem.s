	.setcpu "6502"
	.include "macro.inc"
__MODULE_MEM__ = 1
.segment "mem"
	.include "mem.asm"
	.export _mem_set
	.export _mem_zero
	.export _mem_copy
	.export _mem_compare
	.export _mem_strlen
	;;;=============================
	;;; function _mem_strlen
	;;;=============================
.segment "mem"
.proc _mem_strlen
	lda #0
	sta 0+<FC_FASTCALL_REG+3
@begin_126:
	ldy 0+<FC_FASTCALL_REG+3
	sty <reg+0
	clc
	lda 0+<FC_FASTCALL_REG+1
	adc <reg+0
	sta 0+<FC_FASTCALL_REG+5
	lda 1+<FC_FASTCALL_REG+1
	adc #0
	sta 1+<FC_FASTCALL_REG+5
	lda 0+<FC_FASTCALL_REG+5
	sta <reg+0
	lda 1+<FC_FASTCALL_REG+5
	sta <reg+1
	ldy #0
	lda (reg),y
	beq @else_129
@1:
@then_128:
	clc
	lda 0+<FC_FASTCALL_REG+3
	adc #1
	sta 0+<FC_FASTCALL_REG+3
	jmp @end_130
@else_129:
	jmp @end_127
@end_130:
	jmp @begin_126
@end_127:
	lda 0+<FC_FASTCALL_REG+3
	sta 0+<FC_FASTCALL_REG+0
	rts
.endproc
	.export _mem_strcpy
	;;;=============================
	;;; function _mem_strcpy
	;;;=============================
.segment "mem"
.proc _mem_strcpy
	lda #0
	sta 0+<FC_FASTCALL_REG+5
@begin_134:
	lda #1
	bne @6
	jmp @else_137
@6:
@2:
@then_136:
	ldy 0+<FC_FASTCALL_REG+5
	sty <reg+0
	clc
	lda 0+<FC_FASTCALL_REG+3
	adc <reg+0
	sta 0+<FC_FASTCALL_REG+7
	lda 1+<FC_FASTCALL_REG+3
	adc #0
	sta 1+<FC_FASTCALL_REG+7
	lda 0+<FC_FASTCALL_REG+7
	sta <reg+0
	lda 1+<FC_FASTCALL_REG+7
	sta <reg+1
	ldy #0
	lda (reg),y
	sta 0+<FC_FASTCALL_REG+7
	ldy 0+<FC_FASTCALL_REG+5
	sty <reg+0
	clc
	lda 0+<FC_FASTCALL_REG+1
	adc <reg+0
	sta 0+<FC_FASTCALL_REG+9
	lda 1+<FC_FASTCALL_REG+1
	adc #0
	sta 1+<FC_FASTCALL_REG+9
	lda 0+<FC_FASTCALL_REG+9
	sta <reg+0
	lda 1+<FC_FASTCALL_REG+9
	sta <reg+1
	lda 0+<FC_FASTCALL_REG+7
	ldy #0
	sta (reg),y
	lda 0+<FC_FASTCALL_REG+7
	beq @3
	lda #0
	sta 0+<FC_FASTCALL_REG+9
	jmp @4
@3:
	lda #1
	sta 0+<FC_FASTCALL_REG+9
@4:
	lda 0+<FC_FASTCALL_REG+9
	beq @else_143
@5:
@then_142:
	lda 0+<FC_FASTCALL_REG+5
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_144
@else_143:
@end_144:
	clc
	lda 0+<FC_FASTCALL_REG+5
	adc #1
	sta 0+<FC_FASTCALL_REG+5
	jmp @end_138
@else_137:
	jmp @end_135
@end_138:
	jmp @begin_134
@end_135:
.endproc
