;;; vram.asm: 描画を止めている間に PPU へ直に書く write_raw / fill_raw (vram.fc の write_now / fill_now など。abi "frame")。
;;; 1 KB のネームテーブルを書くので速さを優先する (fc で書くと 1 バイト約 31 サイクル、ここでは 14〜16)。
;;; 注意: .segment は書かない (vram モジュールのセグメントに置かれる)。

;;; write_raw(a:u16, p:*const u8, n:u16, flags:u8): 描画を止めている間に p から n バイトを a から書く (flags が 0 でなければ縦に
;;; +32 ずつ。PPUCTRL の +32 のビットは終わったら戻す)。1 バイト約 13 サイクル
wr_a = F_vram_write_raw__a
wr_p = F_vram_write_raw__p
wr_n = F_vram_write_raw__n
wr_flags = F_vram_write_raw__flags

_vram_write_raw:
	lda _frame_ctrl
	and #<~_nes_CTRL_INC32
	ldy wr_flags
	beq :+
	ora #_nes_CTRL_INC32
:	sta _nes_PPUCTRL
	lda wr_a+1
	sta _nes_PPUADDR
	lda wr_a
	sta _nes_PPUADDR
	ldy #0
	ldx wr_n+1			; 256 バイトのページの数
	beq @part
@page:
	lda (wr_p),y
	sta _nes_PPUDATA
	iny
	bne @page
	inc wr_p+1
	dex
	bne @page
@part:
	ldx wr_n			; 端数 (Y = 0)
	beq @end
@rest:
	lda (wr_p),y
	sta _nes_PPUDATA
	iny
	dex
	bne @rest
@end:
	lda wr_flags
	beq :+
	lda _frame_ctrl
	and #<~_nes_CTRL_INC32
	sta _nes_PPUCTRL
:	rts

;;; fill_raw(a:u16, v:u8, n:u16): 描画を止めている間に v を n 個、a から横に書く
fr_a = F_vram_fill_raw__a
fr_v = F_vram_fill_raw__v
fr_n = F_vram_fill_raw__n

_vram_fill_raw:
	lda _frame_ctrl
	and #<~_nes_CTRL_INC32
	sta _nes_PPUCTRL
	lda fr_a+1
	sta _nes_PPUADDR
	lda fr_a
	sta _nes_PPUADDR
	lda fr_v
	ldy fr_n+1			; 256 個のまとまりの数
	ldx fr_n			; 端数
	beq @pages
@rest:
	sta _nes_PPUDATA
	dex
	bne @rest
@pages:
	cpy #0
	beq @end
@page:
	sta _nes_PPUDATA		; (X = 0 から 256 回)
	dex
	bne @page
	dey
	bne @page
@end:
	rts
