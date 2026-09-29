;;; frame.asm: NMI (fclib/nes/frame.fc、Agent/wiki/plans/v4-stdlib.md §6.2)
;;;
;;; share/runtime.asm の interrupt が A / X / Y を積んでから jsr _interrupt する。順に:
;;;   1. 主の側が待っているとき (_frame_ready != 0) だけ: OAM の DMA (0x80 で、oam を使っていれば) → VRAM のキュー
;;;   2. PPUCTRL / スクロール / PPUMASK (描画を止めていて誰も待っていないときは触らない: 主の側が直に書いているかもしれない)
;;;   3. フレームの数 → 呼び出し口 (_frame_hook)
;;; キューの項目: [番地の上位 | $80 縦 (+32 ずつ) | $40 埋める, 番地の下位, 長さ (1〜255), データ…] (埋めるならデータは 1 バイト)
;;; 注意: .segment は書かない (frame モジュールの固定のバンクに置かれる)。

	.export _interrupt

_interrupt:
	lda _frame_ready
	bne @update
	lda _frame_mask
	and #$18			; 描画を止めていて誰も待っていない: PPU に触らない
	bne @lag
	jmp @count
@lag:
	bit $2002			; (処理落ちのフレーム: レジスタだけ)
	jmp @regs
@update:
	bit $2002			; ラッチを戻し、vblank の印を消す (PPUCTRL の NMI のビットを立て直しても NMI が重ならない)
	lda _frame_ready
	bpl @queue			; $40: キューだけ
	lda _frame_oam_page
	beq @queue
	ldx #0
	stx $2003
	sta $4014
@queue:
	ldx #0
	cpx _frame_queue_len
	bne @entry
	jmp @queue_end
@entry:
	ldy _frame_queue,x		; 番地の上位 | 印
	lda _frame_ctrl
	and #$fb
	cpy #$80
	bcc @horizontal
	ora #$04			; 縦: PPUDATA の後に +32
@horizontal:
	sta $2000
	tya
	and #$3f
	sta $2006
	lda _frame_queue+1,x
	sta $2006
	tya
	and #$40
	beq @copy
	ldy _frame_queue+2,x		; 埋める: 長さ
	lda _frame_queue+3,x		; 値
	inx
	inx
	inx
	inx
	;; 8 バイトずつ展開 (1 バイト約 6.8 サイクル)
@fill8:
	cpy #8
	bcc @fill1
	sta $2007
	sta $2007
	sta $2007
	sta $2007
	sta $2007
	sta $2007
	sta $2007
	sta $2007
	pha
	tya
	sbc #8				; (cpy で C = 1)
	tay
	pla
	cpy #0
	bne @fill8
	beq @next
@fill1:
	cpy #0
	beq @next
@fill1_loop:
	sta $2007
	dey
	bne @fill1_loop
	beq @next
@copy:
	ldy _frame_queue+2,x		; 長さ (1〜255)
	inx
	inx
	inx
	;; 8 バイトずつ展開して写す (1 バイト約 10.4 サイクル。ループの 1 バイトずつは 15)
@copy8:
	cpy #8
	bcc @copy1
	lda _frame_queue,x
	sta $2007
	lda _frame_queue+1,x
	sta $2007
	lda _frame_queue+2,x
	sta $2007
	lda _frame_queue+3,x
	sta $2007
	lda _frame_queue+4,x
	sta $2007
	lda _frame_queue+5,x
	sta $2007
	lda _frame_queue+6,x
	sta $2007
	lda _frame_queue+7,x
	sta $2007
	txa
	adc #7				; (cpy で C = 1 なので +8)
	tax
	tya
	sbc #7				; (adc で C = 0 なので -8)
	tay
	bne @copy8
	beq @next
@copy1:
	cpy #0
	beq @next
@copy1_loop:
	lda _frame_queue,x
	sta $2007
	inx
	dey
	bne @copy1_loop
@next:
	cpx _frame_queue_len
	beq @queue_end
	jmp @entry
@queue_end:
	lda #0
	sta _frame_queue_len
	sta _frame_queue_cost
	sta _frame_ready
	sta $2006			; PPUADDR = 0 (描画を止めているとき、パレットの番地のままだとその色が画面に出る)
	sta $2006
@regs:
	lda _frame_ctrl
	ora #$80
	sta $2000
	lda _frame_scroll_x
	sta $2005
	lda _frame_scroll_y
	sta $2005
	lda _frame_mask
	sta $2001
@count:
	inc _frame_count
	bne @hook
	inc _frame_count+1
@hook:
	lda _frame_hook+1
	beq @end
	;; jmp (_frame_hook) は番地が $xxFF だと上位を読み違える (6502 の不具合) ので、rti で飛ぶ。呼び出し口の rts が
	;; runtime.asm の interrupt へ戻る
	pha
	lda _frame_hook
	pha
	php
	rti
@end:
	rts
