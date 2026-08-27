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
	beq @else_35
@3:
@then_34:
	lda #.LOBYTE(_40)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_40)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_42)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_42)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_36
@else_35:
	lda #.LOBYTE(_45)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_45)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_36:
	rts
_40:
		.byte 10,69,82,82,79,82,58,32,0
_42:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_45:
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
	beq @else_48
@then_47:
	lda #.LOBYTE(_56)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_56)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_58)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_58)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_60)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_60)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_62)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_62)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_49
@else_48:
	lda #.LOBYTE(_65)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_65)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_49:
	rts
_56:
		.byte 10,69,82,82,79,82,58,32,0
_58:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_60:
		.byte 32,32,98,117,116,32,0
_62:
		.byte 10,0
_65:
		.byte 46,0
.endproc
