	.setcpu "6502"
	.include "macro.inc"
__MODULE_UNITTEST__ = 1
.segment "unittest"
	.include "_stdio.inc"
	.export _unittest_assert_true
	;;;=============================
	;;; function _unittest_assert_true
	;;;=============================
.segment "unittest"
.proc _unittest_assert_true
	lda 0+<S+0,x
	bne @else_2
	lda #.LOBYTE(_6)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_6)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_8)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_8)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_3
@else_2:
	lda #.LOBYTE(_11)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_11)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_3:
	rts
_6:
		.byte 10,69,82,82,79,82,58,32,0
_8:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_11:
		.byte 46,0
.endproc
	.export _unittest_assert_equal
	;;;=============================
	;;; function _unittest_assert_equal
	;;;=============================
.segment "unittest"
.proc _unittest_assert_equal
	lda 0+<S+0,x
	cmp 0+<S+2,x
	bne @1
	lda 1+<S+0,x
	cmp 1+<S+2,x
@1:
	beq @else_14
	lda #.LOBYTE(_21)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_21)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_23)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_23)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_25)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_25)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_27)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_27)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_15
@else_14:
	lda #.LOBYTE(_30)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_30)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_15:
	rts
_21:
		.byte 10,69,82,82,79,82,58,32,0
_23:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_25:
		.byte 32,32,98,117,116,32,0
_27:
		.byte 10,0
_30:
		.byte 46,0
.endproc
