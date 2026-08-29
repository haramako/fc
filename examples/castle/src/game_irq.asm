;;; IRQの設定
	.include "./macro.asm"
	.import scroll
	
.segment "game_irq"

_game_irq_setup:
	;lda #%10000000
	;sta _nes_PPU_CTRL1

    lda _subtext_subtext_height
    sta _subtext_subtext_irq_temp
	irq_set _subtext_subtext_irq_temp
	loadw _ppu_irq_next, game_irq_2

	ldx #(_common_CBANK_TEXT+0)
	mmc3_cbank 0    
	ldx #(_common_CBANK_TEXT+2)
	mmc3_cbank 1

	lda #%10100011
	sta _nes_PPU_CTRL1

	;; xwait #24
	;; nop
	;; nop
	lda _ppu_ctrl2_bak
	and #%11101111              ; スプライトを消す
	sta _nes_PPU_CTRL2

    lda _subtext_subtext_irq_temp  ; スクロール量を計算
    clc
    adc #1
    sta _subtext_subtext_irq_scroll
	
	rts

;;; IRQ割り込み(下辺)
game_irq_2:
	sta _mmc3_IRQ_DISABLE		; 4c
	lda #%00010000				; 2c
	sta _nes_PPU_CTRL2			; 4c
	;; 10c

	xwait #3					; 11c
	ldx #0						; 3c
    ldy _subtext_subtext_irq_scroll; 3c
	jsr scroll					; 62c
	;; 68c

	ldx _mmc3_cbank_bak+0		; 4c
	mmc3_cbank 0				; 11c
	ldx _mmc3_cbank_bak+1		; 4c
	mmc3_cbank 1				; 11c
	;; 30c

	lda _ppu_ctrl1_bak			; 4c
	sta _nes_PPU_CTRL1			; 4c
	;; 8c

	xwait #15					; 76c
	nop							; 1c
	nop							; 1c
	lda _ppu_ctrl2_bak			; 4c
	sta _nes_PPU_CTRL2			; 4c
	;; 86c
	;; total 202c (89c+113c = 202c)
	
	rts
