;;; IRQの設定
	.include "./macro.asm"
	.import scroll
	
.segment "title_irq"
	
_title_irq_setup:
	irq_set #127

	ldx _mmc3_cbank_bak+0
	mmc3_cbank 0
	ldx _mmc3_cbank_bak+1
	mmc3_cbank 1

	loadw _ppu_irq_next, title_irq_1
	rts

;;; IRQ割り込み(タイトル下)
title_irq_1:
	sta _mmc3_IRQ_DISABLE		; 4c
	xwait #10
	
	ldx #(_common_CBANK_MISC_TEXT+0)
	mmc3_cbank 0
	ldx #(_common_CBANK_MISC_TEXT+2)
	mmc3_cbank 1
	
	rts
