;;; mem.asm: mem.fc の fill / copy / move の本体 (abi "frame"、ほかの関数を呼ばない)。256 バイトずつのページは Y を一回りさせ、
;;; 端数は X で数える (1 バイトあたり fill 約 11、copy 約 16、後ろからの move 約 18 サイクル)。

;;; fill_raw(dst:*u8, n:u16, v:u8): dst から n バイトを v にする
_mem_fill_raw:
	lda F_mem_fill_raw__v
	ldy #0
	ldx F_mem_fill_raw__n+1		; 256 バイトのページの数
	beq @part
@page:
	sta (F_mem_fill_raw__dst),y
	iny
	bne @page
	inc F_mem_fill_raw__dst+1
	dex
	bne @page
@part:
	ldx F_mem_fill_raw__n		; 端数 (Y = 0)
	beq @done
@rest:
	sta (F_mem_fill_raw__dst),y
	iny
	dex
	bne @rest
@done:
	rts

;;; copy_raw(dst:*u8, src:*const u8, n:u16): src から dst へ前から n バイト写す (dst が src より後ろで重なると壊れる)
_mem_copy_raw:
	ldy #0
	ldx F_mem_copy_raw__n+1
	beq @part
@page:
	lda (F_mem_copy_raw__src),y
	sta (F_mem_copy_raw__dst),y
	iny
	bne @page
	inc F_mem_copy_raw__src+1
	inc F_mem_copy_raw__dst+1
	dex
	bne @page
@part:
	ldx F_mem_copy_raw__n
	beq @done
@rest:
	lda (F_mem_copy_raw__src),y
	sta (F_mem_copy_raw__dst),y
	iny
	dex
	bne @rest
@done:
	rts

;;; move_back_raw(dst:*u8, src:*const u8, n:u16): 後ろから n バイト写す (dst が src より後ろで重なるとき)。
;;; 最後のページの端数を先に、残りのページを後ろから
_mem_move_back_raw:
	clc				; ポインタの上位をページの数だけ進める (端数はその上から Y で)
	lda F_mem_move_back_raw__src+1
	adc F_mem_move_back_raw__n+1
	sta F_mem_move_back_raw__src+1
	clc
	lda F_mem_move_back_raw__dst+1
	adc F_mem_move_back_raw__n+1
	sta F_mem_move_back_raw__dst+1
	ldy F_mem_move_back_raw__n
	beq @pages
@rest:
	dey
	lda (F_mem_move_back_raw__src),y
	sta (F_mem_move_back_raw__dst),y
	tya
	bne @rest
@pages:
	ldx F_mem_move_back_raw__n+1	; (Y = 0)
	beq @done
@page:
	dec F_mem_move_back_raw__src+1
	dec F_mem_move_back_raw__dst+1
@back:
	dey
	lda (F_mem_move_back_raw__src),y
	sta (F_mem_move_back_raw__dst),y
	tya
	bne @back
	dex
	bne @page
@done:
	rts
