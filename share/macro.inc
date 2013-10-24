
	.importzp S
	.importzp L
	.importzp reg

	.import jsr_reg
	.import __mul_8
	.import __mul_8t16
	.import __mul_16
	.import __div_8
	.import __div_8s
	.import __div_16
	.import __mod_8
	.import __mod_16
	
	
;;; call subroutine
;;; use 25 cycle
;;; TODO: フレームサイズがが4以下のときは、inx/dexを使って小さくする
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
