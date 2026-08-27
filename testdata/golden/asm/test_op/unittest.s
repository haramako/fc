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
	beq @else_647
@3:
@then_646:
	lda #.LOBYTE(_652)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_652)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+1,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+1,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_654)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_654)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_648
@else_647:
	lda #.LOBYTE(_657)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_657)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_648:
	rts
_652:
		.byte 10,69,82,82,79,82,58,32,0
_654:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_657:
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
	beq @else_660
@then_659:
	lda #.LOBYTE(_668)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_668)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+4,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+4,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #.LOBYTE(_670)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_670)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+2,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+2,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_672)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_672)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda 0+<S+0,x
	sta <FC_FASTCALL_REG+0
	lda 1+<S+0,x
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print_int16
	lda #.LOBYTE(_674)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_674)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
	lda #1
	sta <FC_FASTCALL_REG+0
	jsr _stdio_exit
	jmp @end_661
@else_660:
	lda #.LOBYTE(_677)
	sta <FC_FASTCALL_REG+0
	lda #.HIBYTE(_677)
	sta <FC_FASTCALL_REG+1
	jsr _stdio_print
@end_661:
	rts
_668:
		.byte 10,69,82,82,79,82,58,32,0
_670:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_672:
		.byte 32,32,98,117,116,32,0
_674:
		.byte 10,0
_677:
		.byte 46,0
.endproc
