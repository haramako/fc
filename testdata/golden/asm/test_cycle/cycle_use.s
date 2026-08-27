	.setcpu "6502"
	.include "macro.inc"
__MODULE_CYCLE_USE__ = 1
.segment "cycle_use"
	.include "_test_cycle.inc"
_cycle_use_CYCLE_CONST = 99
	.export _cycle_use_hoge
	;;;=============================
	;;; function _cycle_use_hoge
	;;;=============================
.segment "cycle_use"
.proc _cycle_use_hoge
	lda 0+_test_cycle_cycle_var
	sta 0+<S+0,x
	rts
.endproc
