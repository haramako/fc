;;; oam.asm: スプライトを OAM の写し (oam.buf) に置く spr / meta / meta_pal (oam.fc。abi "frame")。
;;; メタスプライトは 1 体で何枚も置くので速さを優先する (16×16 の 4 枚の 1 体で、fc で書いた版は約 1520 サイクル、これは約 820。GitHub の Issue #4)。
;;; 作業域は reg (掛け算・割り算のランタイムと同じく呼び出しをまたいで残さない。割り込みから保護しない)。
;;; 注意: .segment は書かない (oam モジュールのセグメントに置かれる)。

;;; spr(x:u8, y:u8, tile:u8, attr:u8):bool — 満杯 (64 枚) なら false
_oam_spr:
	ldx _oam_n
	cpx #64
	bcs @full
	txa
	asl a
	asl a
	tax
	lda F_oam_spr__y
	sta _oam_buf,x
	lda F_oam_spr__tile
	sta _oam_buf+1,x
	lda F_oam_spr__attr
	sta _oam_buf+2,x
	lda F_oam_spr__x
	sta _oam_buf+3,x
	inc _oam_n
	lda #1
	sta F_oam_spr
	rts
@full:	lda #0
	sta F_oam_spr
	rts

;;; meta / meta_pal の作業域
om_x = reg+0			; x (i16。左右の反転なら x - 7)
om_y = reg+2			; y (i16。上下の反転なら y + 1 - h)
om_p = reg+4			; m の先頭 (ゼロページのポインタ)
om_len = reg+6			; m の長さ (om_place の中では長さ - 3)
om_flip = reg+7
om_keep = reg+8			; 枚ごとの attr = (m の attr & keep | add) ^ flip
om_add = reg+9
om_ex = reg+10			; dx に eor する値 (左右の反転なら $ff)
om_ey = reg+11			; dy に eor する値 (上下の反転なら $ff)
om_ok = reg+12			; 全部置けたか
om_k = reg+13			; m の中の位置
om_sx = reg+14			; 置く x
om_sy = reg+15			; 置く y

;;; meta(x:i16, y:i16, m:[]const u8, flip:u8):bool
_oam_meta:
	lda #$ff
	sta om_keep
	lda #0
	sta om_add
.repeat 8, I				; x, y, m (ptr・len), flip を写す (フレームの並びのまま 8 バイト)
	lda F_oam_meta__x+I
	sta om_x+I
.endrepeat
	jsr om_place
	sta F_oam_meta
	rts

;;; meta_pal(x:i16, y:i16, m:[]const u8, flip:u8, pal:u8):bool
_oam_meta_pal:
	lda #$fc
	sta om_keep
	lda F_oam_meta_pal__pal
	and #3
	sta om_add
.repeat 8, I
	lda F_oam_meta_pal__x+I
	sta om_x+I
.endrepeat
	jsr om_place
	sta F_oam_meta_pal
	rts

;;; m の {dx:i8, dy:i8, tile, attr} の並びを (x, y) からずらして置く。画面 (0〜255、0〜239) の外のものは置かない。
;;; flip の左右 ($40) は dx を -dx - 8 に、上下 ($80) は dy を -dy - h に (h はスプライトの高さ)。A = 全部置けたら 1
;;; 反転はループの前に済ませる: x + (-dx - 8) = (x - 7) + ~dx、y + (-dy - h) = (y + 1 - h) + ~dy
om_place:
	ldx #0
	bit om_flip
	bvc :+
	dex
	lda om_x
	sec
	sbc #7
	sta om_x
	bcs :+
	dec om_x+1
:	stx om_ex
	ldx #0
	bit om_flip
	bpl @setv
	ldy #7				; h - 1
	lda _frame_ctrl
	and #_nes_CTRL_SPR_8X16
	beq :+
	ldy #15
:	sty om_sy
	lda om_y
	sec
	sbc om_sy
	sta om_y
	bcs :+
	dec om_y+1
:	dex
@setv:	stx om_ey
	lda #1
	sta om_ok
	lda om_len			; k + 3 < len の間 (k < len - 3)
	sec
	sbc #3
	bcc @done
	beq @done
	sta om_len
	ldy #0
@one:	sty om_k
	;; sx = x + dx。16 ビットで、上位が 0 なら画面の中
	lda (om_p),y
	eor om_ex
	bmi @nx
	clc
	adc om_x
	sta om_sx
	lda om_x+1
	adc #0
	bne @next			; 0〜255 の外
	beq @dy
@nx:	clc
	adc om_x
	sta om_sx
	lda om_x+1
	adc #$ff
	bne @next
	;; sy = y + dy
@dy:	iny
	lda (om_p),y
	eor om_ey
	bmi @ny
	clc
	adc om_y
	sta om_sy
	lda om_y+1
	adc #0
	bne @next			; 0〜239 の外
	beq @chk
@ny:	clc
	adc om_y
	sta om_sy
	lda om_y+1
	adc #$ff
	bne @next
@chk:	lda om_sy
	cmp #240
	bcs @next
	;; 置く (spr と同じ。満杯なら置かずに ok = 0)
	ldx _oam_n
	cpx #64
	bcs @full
	txa
	asl a
	asl a
	tax
	lda om_sy
	sta _oam_buf,x
	iny
	lda (om_p),y			; tile
	sta _oam_buf+1,x
	iny
	lda (om_p),y			; attr
	and om_keep
	ora om_add
	eor om_flip
	sta _oam_buf+2,x
	lda om_sx
	sta _oam_buf+3,x
	inc _oam_n
@next:	lda om_k
	clc
	adc #4
	tay
	cmp om_len
	bcc @one
@done:	lda om_ok
	rts
@full:	lda #0
	sta om_ok
	beq @next
