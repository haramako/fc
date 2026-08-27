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
@begin_93:
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
	beq @else_96
@1:
@then_95:
	clc
	lda 0+<FC_FASTCALL_REG+3
	adc #1
	sta 0+<FC_FASTCALL_REG+3
	jmp @end_97
@else_96:
	jmp @end_94
@end_97:
	jmp @begin_93
@end_94:
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
@begin_101:
	lda #1
	bne @6
	jmp @else_104
@6:
@2:
@then_103:
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
	beq @else_110
@5:
@then_109:
	lda 0+<FC_FASTCALL_REG+5
	sta 0+<FC_FASTCALL_REG+0
	rts
	jmp @end_111
@else_110:
@end_111:
	clc
	lda 0+<FC_FASTCALL_REG+5
	adc #1
	sta 0+<FC_FASTCALL_REG+5
	jmp @end_105
@else_104:
	jmp @end_102
@end_105:
	jmp @begin_101
@end_102:
.endproc
