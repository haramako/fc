;;; split.asm: ステータスバーの下で呼ばれる IRQ の呼び出し口 (statusbar.fc の mmc3.irq_hook)。
;;; fclib の mmc3 の IRQ の入口が受け取り ($E000) を書いてから来る (A / X / Y は runtime.asm が積んである)。
;;; 横のスクロールは描画の途中でも $2005 の 1 回目と $2000 のネームテーブルのビットで変えられる (次の走査線から)。

	.export split_irq

split_irq:
	bit $2002			; $2005 の 1 回目の書き込みに戻す
	lda split_x
	sta $2005
	lda #0
	sta $2005			; (縦のスクロールは描画の途中では効かない)
	lda split_ctrl
	sta $2000			; ネームテーブルの選択 (スクロールの 256 の位)
	rts
