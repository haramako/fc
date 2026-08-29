;;; IRQの設定
	.include "./macro.asm"
	
.segment "menu_irq" 				; 割り込みのためにCODEセグメントに配置
	
_menu_irq_setup:
	irq_set #31
	
	ldx #(_common_CBANK_TEXT+0)
	mmc3_cbank 0
	ldx #(_common_CBANK_TEXT+2)
	mmc3_cbank 1
	
	;; scene == SCENE_MAP のときは、ここで終わり
	lda _menu_scene
	cmp #_menu_SCENE_MAP
	bne @else
	;; scene == SCENE_MAP
	loadw _ppu_irq_next, menu_irq_map_1
	rts
@else:
	cmp #_menu_SCENE_MEMORY
	bne @end
	;; scene == SCENE_MEMORY
	loadw _ppu_irq_next, menu_irq_memory_1
	rts
@end:
	;; SCENE == SCENE_ITEM
	loadw _ppu_irq_next, menu_irq_item_1
	rts

;;;===================================================
;;; アイテム画面
;;;===================================================

;;; IRQ割り込み(上辺)
menu_irq_item_1:
	irq_set #79
	xwait #2

	ldx _mmc3_cbank_bak+0
	mmc3_cbank 0
	ldx _mmc3_cbank_bak+1
	mmc3_cbank 1

	loadw _ppu_irq_next, menu_irq_item_2
	rts

;;; IRQ割り込み(下辺)
menu_irq_item_2:
	sta _mmc3_IRQ_DISABLE

	ldx #(_common_CBANK_ITEM_TEXT+0)
	mmc3_cbank 0
	ldx #(_common_CBANK_ITEM_TEXT+2)
	mmc3_cbank 1

	rts


;;;===================================================
;;; マップ画面
;;;===================================================

;;; IRQ割り込み(上辺)
menu_irq_map_1:
	sta _mmc3_IRQ_DISABLE

	ldx _mmc3_cbank_bak+0
	mmc3_cbank 0
	ldx _mmc3_cbank_bak+1
	mmc3_cbank 1
	rts
	
;;;===================================================
;;; 記憶画面
;;;===================================================

;;; IRQ割り込み(上辺)
menu_irq_memory_1:
	irq_set #63

	ldx _mmc3_cbank_bak+0
	mmc3_cbank 0
	ldx _mmc3_cbank_bak+1
	mmc3_cbank 1

	loadw _ppu_irq_next, menu_irq_memory_2
	rts

;;; IRQ割り込み(下辺)
menu_irq_memory_2:
	sta _mmc3_IRQ_DISABLE

	ldx #(_common_CBANK_MEMORY_TEXT+0)
	mmc3_cbank 0
	ldx #(_common_CBANK_MEMORY_TEXT+2)
	mmc3_cbank 1

	rts
	
