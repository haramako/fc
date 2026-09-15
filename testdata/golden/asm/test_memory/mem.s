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
	jmp @begin_1
@body_3:
	inc 0+<FC_FASTCALL_REG+3
@begin_1:
	ldy 0+<FC_FASTCALL_REG+3
	lda (FC_FASTCALL_REG+1),y
	bne @body_3
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
@then_11:
	ldy 0+<FC_FASTCALL_REG+5
	lda (FC_FASTCALL_REG+3),y
	sta 0+<FC_FASTCALL_REG+6
	ldy 0+<FC_FASTCALL_REG+5
	lda 0+<FC_FASTCALL_REG+6
	sta (FC_FASTCALL_REG+1),y
	lda 0+<FC_FASTCALL_REG+6
	bne @end_19
	lda 0+<FC_FASTCALL_REG+5
	sta 0+<FC_FASTCALL_REG+0
	rts
@end_19:
	inc 0+<FC_FASTCALL_REG+5
	jmp @then_11
.endproc
	.import FC_FASTCALL_REG_SIZE
	.assert FC_FASTCALL_REG_SIZE >= 7, error, "fastcall functions of module mem need 7 bytes of FC_FASTCALL_REG (raise .res of FC_FASTCALL_REG and FC_FASTCALL_REG_SIZE in base.asm)"
