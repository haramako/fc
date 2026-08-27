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
	beq @else_135
@3:
@then_134:
	lda #.LOBYTE(_140)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_140)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_142)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_142)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_136
@else_135:
	lda #.LOBYTE(_145)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_145)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_136:
	rts
_140:
		.byte 10,69,82,82,79,82,58,32,0
_142:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_145:
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
	beq @else_148
@then_147:
	lda #.LOBYTE(_156)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_156)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_158)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_158)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_160)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_160)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_162)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_162)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_149
@else_148:
	lda #.LOBYTE(_165)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_165)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_149:
	rts
_156:
		.byte 10,69,82,82,79,82,58,32,0
_158:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_160:
		.byte 32,32,98,117,116,32,0
_162:
		.byte 10,0
_165:
		.byte 46,0
.endproc
