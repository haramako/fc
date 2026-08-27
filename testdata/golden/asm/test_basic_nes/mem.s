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
@begin_138:
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
	beq @else_141
@1:
@then_140:
	clc
	lda 0+<FC_FASTCALL_REG+3
	adc #1
	sta 0+<FC_FASTCALL_REG+3
	jmp @end_142
@else_141:
	jmp @end_139
@end_142:
	jmp @begin_138
@end_139:
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
@begin_146:
	lda #1
	bne @6
	jmp @else_149
@6:
@2:
@then_148:
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
	beq @else_155
@5:
@then_154:
	lda 0+<FC_FASTCALL_REG+5
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_156
@else_155:
@end_156:
	clc
	lda 0+<FC_FASTCALL_REG+5
	adc #1
	sta 0+<FC_FASTCALL_REG+5
	jmp @end_150
@else_149:
	jmp @end_147
@end_150:
	jmp @begin_146
@end_147:
.endproc
