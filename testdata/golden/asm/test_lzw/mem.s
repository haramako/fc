	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
__MODULE_MEM__ = 1
.segment "mem"
FC_USED_mem_set = 0
FC_USED_mem_zero = 0
FC_USED_mem_copy = 0
FC_USED_mem_compare = 1
	.include "mem.asm"
	.global _mem_compare
	.export _mem_strlen
	;;;=============================
	;;; function _mem_strlen
	;;;=============================
.segment "mem"
.proc _mem_strlen
	lda #0
	sta 0+<F_mem_strlen+3
	tay
	jmp @begin_1
@body_3:
	iny
@begin_1:
	lda (F_mem_strlen+1),y
	bne @body_3
	sty 0+<F_mem_strlen+3
	lda 0+<F_mem_strlen+3
	rts
.endproc
