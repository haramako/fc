
	.importzp S
	.importzp L
	.importzp reg
	.importzp FC_FASTCALL_REG

	.import jsr_reg
	.import __mul_8
	.import __mul_8s
	.import __mul_8t16
	.import __mul_16
	.import __div_8
	.import __div_8s
	.import __div_16
	.import __mod_8
	.import __mod_16
	
	
;;; call subroutine
;;; use 25 cycle
.macro call addr, frame_size
	txa
	pha
	clc
	adc frame_size
	tax
	jsr addr
	pla
	tax
.endmacro
