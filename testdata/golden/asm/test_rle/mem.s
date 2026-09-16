	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
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
	sta 0+<F_mem_strlen+3
	jmp @begin_1
@body_3:
	inc 0+<F_mem_strlen+3
@begin_1:
	ldy 0+<F_mem_strlen+3
	lda (F_mem_strlen+1),y
	bne @body_3
	lda 0+<F_mem_strlen+3
	sta 0+<F_mem_strlen+0
	rts
.endproc
	.export _mem_strcpy
	;;;=============================
	;;; function _mem_strcpy
	;;;=============================
.segment "mem"
.proc _mem_strcpy
	lda #0
	sta 0+<F_mem_strcpy+6
@then_11:
	ldy 0+<F_mem_strcpy+6
	lda (F_mem_strcpy+3),y
	sta (F_mem_strcpy+1),y
	bne @end_19
	lda 0+<F_mem_strcpy+6
	sta 0+<F_mem_strcpy+0
	rts
@end_19:
	inc 0+<F_mem_strcpy+6
	jmp @then_11
.endproc
