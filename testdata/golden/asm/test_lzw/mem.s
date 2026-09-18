	.setcpu "6502"
	.include "macro.inc"
	.include "_frames.inc"
	.importzp FC_SP
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
	tay
	jmp @begin_1
@body_3:
	iny
@begin_1:
	lda (F_mem_strlen+1),y
	bne @body_3
	sty 0+<F_mem_strlen+3
	lda 0+<F_mem_strlen+3
	sta 0+<F_mem_strlen+0
	rts
.endproc
