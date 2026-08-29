;;; IRQの設定
	.include "./macro.asm"
	.import scroll
	
.segment "event_irq"
	
_event_irq_setup:
	irq_set #95

	ldx _mmc3_cbank_bak+0
	mmc3_cbank 0
	ldx _mmc3_cbank_bak+1
	mmc3_cbank 1

	loadw _ppu_irq_next, event_irq_1
	rts

;;; IRQ割り込み(上辺)
;;; ここに入ってくるまでに,24cycle使っている。１回めの h-blank まで 113 - 24 で 89cycle.
event_irq_1:
	xwait #5					; 26c
	irq_set #63					; 18c
	xwait #7					; 36c
	nop
	nop
	lda #%00010000				; nestopia(実機と同等)と他のエミュ(VirtuaNES, FCEUX)の挙動を合わせるためスプライトを消さない
	sta _nes_PPU_CTRL2			; 2+3+4 = 9c
	;; 89c

	;; xwait #14
	ldx #0
	ldy #0
	jsr scroll
	
	ldx #(_common_CBANK_TEXT+0)
	mmc3_cbank 0
	ldx #(_common_CBANK_TEXT+2)
	mmc3_cbank 1

	lda #%10100001
	sta _nes_PPU_CTRL1

	;; xwait #24
	;; nop
	;; nop
	lda _ppu_ctrl2_bak
	and #%11101111              ; スプライトを消す
	sta _nes_PPU_CTRL2
	
	loadw _ppu_irq_next, event_irq_2
	rts

;;; IRQ割り込み(下辺)
event_irq_2:
	sta _mmc3_IRQ_DISABLE		; 4c
	lda #%00010000				; 2c
	sta _nes_PPU_CTRL2			; 4c
	;; 10c

	;; xwait #4					; 21c
	ldx #0						; 3c
	ldy #159					; 3c
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
