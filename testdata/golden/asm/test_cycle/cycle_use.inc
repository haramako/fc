.ifndef __MODULE_CYCLE_USE__
__MODULE_CYCLE_USE__ = 1
	.include "_test_cycle.inc"
_cycle_use_CYCLE_CONST = 99
	.import _cycle_use_hoge
.endif
