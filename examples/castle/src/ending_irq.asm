;;; IRQの設定
	.include "./macro.asm"
	.import scroll
	
.segment "ending_irq"
	
_ending_irq_setup:
	irq_set #96

	ldx _mmc3_cbank_bak+0
	mmc3_cbank 0
	ldx _mmc3_cbank_bak+1
	mmc3_cbank 1

	loadw _ppu_irq_next, ending_irq_1
	rts

;;; IRQ割り込み(エンディング下)
ending_irq_1:
	sta _mmc3_IRQ_DISABLE		; 4c
	lda #%00011110				; 2c
	sta _nes_PPU_CTRL2			; 4c
	;; 10c

	xwait #2					; 21c
	nop
	nop
	ldx _ending_irq_scroll		; 4c
	ldy #97						; 3c
	jsr scroll					; 62c
	;; 68c
	
	ldx #(_common_CBANK_BG_COMMON)
	mmc3_cbank 0
	ldx #(_common_CBANK_BG+2)
	mmc3_cbank 1
	
	rts
