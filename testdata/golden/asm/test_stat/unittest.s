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
	beq @else_126
@3:
@then_125:
	lda #.LOBYTE(_131)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_131)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_133)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_133)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_127
@else_126:
	lda #.LOBYTE(_136)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_136)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_127:
	rts
_131:
		.byte 10,69,82,82,79,82,58,32,0
_133:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_136:
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
	beq @else_139
@then_138:
	lda #.LOBYTE(_147)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_147)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_149)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_149)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_151)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_151)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_153)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_153)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_140
@else_139:
	lda #.LOBYTE(_156)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_156)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_140:
	rts
_147:
		.byte 10,69,82,82,79,82,58,32,0
_149:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_151:
		.byte 32,32,98,117,116,32,0
_153:
		.byte 10,0
_156:
		.byte 46,0
.endproc
