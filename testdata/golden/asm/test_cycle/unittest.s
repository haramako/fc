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
	beq @else_12
@3:
@then_11:
	lda #.LOBYTE(_17)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_17)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_19)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_19)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_13
@else_12:
	lda #.LOBYTE(_22)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_22)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_13:
	rts
_17:
		.byte 10,69,82,82,79,82,58,32,0
_19:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_22:
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
	beq @else_25
@then_24:
	lda #.LOBYTE(_33)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_33)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_35)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_35)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_37)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_37)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_39)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_39)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_26
@else_25:
	lda #.LOBYTE(_42)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_42)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_26:
	rts
_33:
		.byte 10,69,82,82,79,82,58,32,0
_35:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_37:
		.byte 32,32,98,117,116,32,0
_39:
		.byte 10,0
_42:
		.byte 46,0
.endproc
