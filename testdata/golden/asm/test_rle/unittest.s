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
	beq @1
	lda #0
	sta 0+<L+0
	jmp @2
@1:
	lda #1
	sta 0+<L+0
@2:
	lda 0+<L+0
	beq @else_34
@3:
@then_33:
	lda #.LOBYTE(_39)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_39)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_41)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_41)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_35
@else_34:
	lda #.LOBYTE(_44)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_44)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_35:
	rts
_39:
		.byte 10,69,82,82,79,82,58,32,0
_41:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_44:
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
	bne @4
	lda 1+<S+0,x
	cmp 1+<S+2,x
@4:
	beq @else_47
@then_46:
	lda #.LOBYTE(_55)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_55)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_57)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_57)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_59)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_59)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_61)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_61)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_48
@else_47:
	lda #.LOBYTE(_64)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_64)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_48:
	rts
_55:
		.byte 10,69,82,82,79,82,58,32,0
_57:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_59:
		.byte 32,32,98,117,116,32,0
_61:
		.byte 10,0
_64:
		.byte 46,0
.endproc
