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
	beq @else_91
@3:
@then_90:
	lda #.LOBYTE(_96)
	sta <S+3,x
	lda #.HIBYTE(_96)
	sta <S+4,x
	inx
	inx
	inx
	jsr _stdio_print
	dex
	dex
	dex
	lda 0+<S+1,x
	sta <S+3,x
	lda 1+<S+1,x
	sta <S+4,x
	inx
	inx
	inx
	jsr _stdio_print
	dex
	dex
	dex
	lda #.LOBYTE(_98)
	sta <S+3,x
	lda #.HIBYTE(_98)
	sta <S+4,x
	inx
	inx
	inx
	jsr _stdio_print
	dex
	dex
	dex
	lda #1
	sta <S+3,x
	inx
	inx
	inx
	jsr _stdio_exit
	dex
	dex
	dex
	jmp @end_92
@else_91:
	lda #.LOBYTE(_101)
	sta <S+3,x
	lda #.HIBYTE(_101)
	sta <S+4,x
	inx
	inx
	inx
	jsr _stdio_print
	dex
	dex
	dex
@end_92:
	rts
_96:
		.byte 10,69,82,82,79,82,58,32,0
_98:
		.byte 32,32,101,120,112,101,99,116,115,32,116,114,117,101,32,98
		.byte 117,116,32,102,97,108,115,101,10,0
_101:
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
	bne @6
	jmp @else_104
@6:
@then_103:
	lda #.LOBYTE(_112)
	sta <S+6,x
	lda #.HIBYTE(_112)
	sta <S+7,x
	call _stdio_print, #6
	lda 0+<S+4,x
	sta <S+6,x
	lda 1+<S+4,x
	sta <S+7,x
	call _stdio_print, #6
	lda #.LOBYTE(_114)
	sta <S+6,x
	lda #.HIBYTE(_114)
	sta <S+7,x
	call _stdio_print, #6
	lda 0+<S+2,x
	sta <S+6,x
	lda 1+<S+2,x
	sta <S+7,x
	call _stdio_print_int16, #6
	lda #.LOBYTE(_116)
	sta <S+6,x
	lda #.HIBYTE(_116)
	sta <S+7,x
	call _stdio_print, #6
	lda 0+<S+0,x
	sta <S+6,x
	lda 1+<S+0,x
	sta <S+7,x
	call _stdio_print_int16, #6
	lda #.LOBYTE(_118)
	sta <S+6,x
	lda #.HIBYTE(_118)
	sta <S+7,x
	call _stdio_print, #6
	lda #1
	sta <S+6,x
	call _stdio_exit, #6
	jmp @end_105
@else_104:
	lda #.LOBYTE(_121)
	sta <S+6,x
	lda #.HIBYTE(_121)
	sta <S+7,x
	call _stdio_print, #6
@end_105:
	rts
_112:
		.byte 10,69,82,82,79,82,58,32,0
_114:
		.byte 32,32,101,120,112,101,99,116,115,32,0
_116:
		.byte 32,32,98,117,116,32,0
_118:
		.byte 10,0
_121:
		.byte 46,0
.endproc
