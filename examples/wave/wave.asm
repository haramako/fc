;;; wave.asm: 8 ラインごとの IRQ の呼び出し口 (wave.fc の mmc3.irq_hook)。fclib の mmc3 の IRQ の入口が受け取り ($E000。IRQ も
;;; 止まる) を書いてから来るので、$E001 で続けて IRQ を許し、次の帯の横のスクロールを書く。

	.export wave_irq

wave_irq:
	sta $E001			; 続けて 8 ライン後にも IRQ
	ldx wave_band
	cpx #32
	bcs @end
	lda wave_x,x
	bit $2002			; $2005 の 1 回目の書き込みに戻す
	sta $2005
	lda #0
	sta $2005
	inc wave_band
@end:
	rts
