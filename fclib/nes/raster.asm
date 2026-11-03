;;; raster.asm: raster.fc の NMI の仕掛けと IRQ (MMC3 の走査線のカウンタで画面を分ける)。
;;; 表 _raster_cur の境目ごとの 5 バイト: [走査線 B, $2006 の 1 回目 (ネームテーブル × 4), Y, X, $2006 の 2 回目]
;;; IRQ の中で $2006 / $2005 (Y) をすぐ書き、境目の 1 本上の走査線の hblank (画素 256〜320) まで待って $2005 (X) / $2006 を書く
;;; (nesdev wiki の Split X/Y scroll。後ろの 2 回で番地がその場で入れ替わる。hblank より前だと、その走査線の右の端の細かい横の位置が
;;; 変わり、走査線の終わりで縦の位置が 1 進む)。最初の境目は NMI で $C000 = B - 1、続く境目までの d 走査線は IRQ の中で d - 2
;;; (docs の「走査線の IRQ」)。この数と待ちの長さは、境目の前後の走査線を Mesen と QuickNES の画面で分割なしの画面と画素で比べて
;;; 合わせた (internal/nes の TestRasterSplit。待ちが 4 以下だと Mesen で境目の 1 本上の右の端、7 以上だと境目の左の端が乱れる。
;;; IRQ に入るまでのぶれは遅れる側にしか出ないので、早い側の 5)。
;;; 関数は .if FC_USED_… で包む (使わなければ ROM に入らない)。注意: .segment は書かない (raster モジュールの固定のバンク)。

RASTER_WAIT = 5			; 待ちの回数 (dey / bne の 1 回が 5 サイクル)

.if FC_USED_raster_nmi
;;; frame の NMI がスクロールを書いた後 (frame.raster)。X の bit 7 = 主の側が wait まで済ませたフレーム: next を cur に写す
_raster_nmi:
	sta $E000			; 前のフレームの IRQ を止める
	txa
	bpl @arm
.repeat 20, I
	lda _raster_next+I
	sta _raster_cur+I
.endrepeat
@arm:
	lda _raster_cur			; 最初の境目
	cmp #2				; (RASTER_MIN)
	bcc @none
	cmp #240
	bcs @none
	sec
	sbc #1				; B - 1: 上から B - 1 本を描き終わる所 (B - 2 の走査線の終わり) で IRQ。IRQ の中の待ちで B - 1 の hblank に書く
	sta $C000
	sta $C001
	sta $E001
	lda #0
	sta _raster_at
@none:
	rts
.endif

.if FC_USED_raster_irq
;;; mmc3 の IRQ の入口 ($E000 で受け取り済み。A / X / Y は runtime.asm が積んである) から (mmc3.irq_hook)
_raster_irq:
	ldx _raster_at
	bit $2002			; $2005 / $2006 の書き込みを 1 回目に戻す
	lda _raster_cur+1,x
	sta $2006			; ネームテーブル × 4
	lda _raster_cur+2,x
	sta $2005			; Y
	ldy #RASTER_WAIT
@wait:
	dey
	bne @wait
	lda _raster_cur+3,x
	sta $2005			; X (hblank)
	lda _raster_cur+4,x
	sta $2006			; ((Y & $F8) << 2) | (X >> 3): ここで番地が入れ替わる
	;; 次の境目 (無いか 3 走査線より近ければ止める)
	inx
	inx
	inx
	inx
	inx
	cpx #20
	bcs @last
	stx _raster_at
	lda _raster_cur,x
	beq @last
	cmp #240
	bcs @last
	sec
	sbc _raster_cur-5,x		; d = 次の境目 - この境目
	bcc @last
	cmp #3
	bcc @last
	sbc #2				; (C = 1) d - 2
	sta $C000
	sta $C001
	sta $E001
@last:
	rts
.endif
