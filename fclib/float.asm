;;; float.asm: f32 (IEEE 754 binary32) の演算 (float.fc が abi "frame" で宣言する。Agent/wiki/design/float-fixed.md)。
;;; float.fc の fc で書いた版と同じ手順 (仮数は 27 ビット: 上の 24 ビットが仮数、下の 3 ビットが丸めの印。最近接偶数に丸める、
;;; 非正規化数・±0・±無限大、NaN を作る結果は 0x7fc00000)。TestFloatLib が Go の float32 とビットで比べる。
;;; 作業域は reg (FC_REG の 16 バイト。掛け算・割り算のランタイムと同じく呼び出しをまたいで残さない。割り込みから保護しない) と、
;;; 関数ごとのフレームの scratch。公開の関数どうしは呼び合わない (同じモジュールの abi "frame" の関数を呼ぶとフレームが重なる)。
;;; 中の処理は reg の 2 つの演算数 (a: X = 0、b: X = FB) を読み書きするラベル。
;;; 注意: .segment は書かない (float モジュールのセグメントに置かれる)。

	.import __mul_tbl_l0, __mul_tbl_l1, __mul_tbl_h0, __mul_tbl_h1

fa_s = reg+0			; a の符号 ($80 か 0)
fa_e = reg+1			; a の指数 (バイアス込み、符号付きの 2 バイト)
fa_m = reg+3			; a の仮数 (4 バイト。読み込んだときは f32 のビットの並びそのまま)
fb_s = reg+7
fb_e = reg+8
fb_m = reg+10
ft = reg+14			; 作業用の 2 バイト
FB = 7				; b の X の位置 (fb_s - fa_s)

;;; 4 バイトを写す (展開する: ループより 1 回 約 30 サイクル速い)
.macro FL_LOAD src, dst
	lda src
	sta dst
	lda src+1
	sta dst+1
	lda src+2
	sta dst+2
	lda src+3
	sta dst+3
.endmacro

;;; a と b の指数のビットがどちらも 1〜254 (0 でも非正規化数でも無限大・NaN でもない) なら fast へ飛ぶ (A を壊す)
;;; (中は無名のラベル: .local のラベルは @ のラベルの範囲を切る)
.macro FL_IF_NORMAL fast
	lda fa_m+2
	asl a
	lda fa_m+3
	rol a
	beq :+
	cmp #$ff
	beq :+
	lda fb_m+2
	asl a
	lda fb_m+3
	rol a
	beq :+
	cmp #$ff
	bne fast
:
.endmacro

;;; ---------------------------------------------------------------
;;; 分類 (X = 0 なら a、FB なら b のビットの並び)
;;; ---------------------------------------------------------------

;;; 指数のビットが 255 (無限大か NaN) なら C = 1
fl_expmax:
	lda fa_m+3,x
	and #$7f
	cmp #$7f
	bne @no
	lda fa_m+2,x
	asl a
	rts
@no:	clc
	rts

;;; NaN なら C = 1
fl_isnan:
	jsr fl_expmax
	bcc @r
	lda fa_m+2,x
	and #$7f
	ora fa_m+1,x
	ora fa_m,x
	beq @no			; 無限大
	sec
@r:	rts
@no:	clc
	rts

;;; ±0 なら Z = 1
fl_iszero:
	lda fa_m+3,x
	and #$7f
	ora fa_m+2,x
	ora fa_m+1,x
	ora fa_m,x
	rts

;;; a = NaN (0x7fc00000)
fl_setnan:
	lda #$c0
	ldx #$7f
	bne fl_set_hi
;;; a = 符号 ft の無限大
fl_setinf:
	lda ft
	ora #$7f
	tax
	lda #$80
fl_set_hi:
	stx fa_m+3
	sta fa_m+2
	lda #0
	sta fa_m+1
	sta fa_m
	rts
;;; a = 符号 ft の 0
fl_setzero:
	lda #0
	sta fa_m
	sta fa_m+1
	sta fa_m+2
	lda ft
	sta fa_m+3
	rts

;;; a = b
fl_ret_b:
	FL_LOAD fb_m, fa_m
	rts

;;; ---------------------------------------------------------------
;;; 取り出す・ずらす・詰める
;;; ---------------------------------------------------------------

;;; 有限で 0 でない値 (X = 0 / FB) を符号・指数・仮数 (24 ビット、隠れたビットは bit 23) に。非正規化数は隠れたビットが
;;; bit 23 に来るまでずらし、指数を減らす (指数は 1 から)
fl_unpack:
	lda fa_m+3,x
	and #$80
	sta fa_s,x
	lda fa_m+2,x
	asl a			; C = 指数の下位のビット
	lda fa_m+3,x
	rol a			; A = 指数
	sta fa_e,x
	lda #0
	sta fa_e+1,x
	sta fa_m+3,x
	lda fa_m+2,x
	and #$7f
	sta fa_m+2,x
	lda fa_e,x
	beq @sub
	lda fa_m+2,x
	ora #$80
	sta fa_m+2,x
	rts
@sub:	lda #1
	sta fa_e,x
@n8:	lda fa_m+2,x		; 8 ビットずつ (m < 0x8000 の間)
	bne @n1
	lda fa_m+1,x
	bmi @n1
	sta fa_m+2,x
	lda fa_m,x
	sta fa_m+1,x
	lda #0
	sta fa_m,x
	sec
	lda fa_e,x
	sbc #8
	sta fa_e,x
	lda fa_e+1,x
	sbc #0
	sta fa_e+1,x
	jmp @n8
@n1:	lda fa_m+2,x		; 1 ビットずつ (bit 23 が立つまで)
	bmi @r
	asl fa_m,x
	rol fa_m+1,x
	rol fa_m+2,x
	lda fa_e,x
	bne :+
	dec fa_e+1,x
:	dec fa_e,x
	jmp @n1
@r:	rts

;;; 仮数 (X = 0 / FB) を 3 ビット左へ
fl_shl3:
	ldy #3
@l:	asl fa_m,x
	rol fa_m+1,x
	rol fa_m+2,x
	rol fa_m+3,x
	dey
	bne @l
	rts

;;; 仮数 (X = 0 / FB) を Y ビット (0〜31) 右へ。消えたビットが 1 つでもあれば最下位を 1 にする (sticky)。ft+1 を使う
fl_shr_sticky:
	lda #0
	sta ft+1
@b8:	cpy #8
	bcc @b1
	lda fa_m,x
	ora ft+1
	sta ft+1
	lda fa_m+1,x
	sta fa_m,x
	lda fa_m+2,x
	sta fa_m+1,x
	lda fa_m+3,x
	sta fa_m+2,x
	lda #0
	sta fa_m+3,x
	tya
	sec
	sbc #8
	tay
	jmp @b8
@b1:	cpy #0
	beq @end
@l:	lsr fa_m+3,x
	ror fa_m+2,x
	ror fa_m+1,x
	ror fa_m,x
	bcc :+
	lda #1
	sta ft+1
:	dey
	bne @l
@end:	lda ft+1
	beq @r
	lda fa_m,x
	ora #1
	sta fa_m,x
@r:	rts

;;; a の 0 でない仮数 (bit 26 未満) を bit 26 が立つまで左へずらしてから詰める (指数を減らす)
fl_norm_pack:
@n8:	lda fa_m+3		; m < 0x40000 の間は 8 ビットずつ
	bne @n1
	lda fa_m+2
	cmp #4
	bcs @n1
	sta fa_m+3
	lda fa_m+1
	sta fa_m+2
	lda fa_m
	sta fa_m+1
	lda #0
	sta fa_m
	sec
	lda fa_e
	sbc #8
	sta fa_e
	lda fa_e+1
	sbc #0
	sta fa_e+1
	jmp @n8
@n1:	lda fa_m+3		; m < 0x4000000 の間は 1 ビットずつ
	and #$04
	bne fl_pack
	asl fa_m
	rol fa_m+1
	rol fa_m+2
	rol fa_m+3
	lda fa_e
	bne :+
	dec fa_e+1
:	dec fa_e
	jmp @n1

;;; a の符号・指数 (バイアス込み)・27 ビットの仮数 (隠れたビットは bit 26、0 でない) を最近接偶数に丸めて f32 のビットの並びを
;;; fa_m に詰める。指数が 1 より小さければ非正規化数にずらす
fl_pack:
	lda fa_e+1
	bmi @den
	ora fa_e
	bne @round
@den:	sec			; n = 1 - e
	lda #1
	sbc fa_e
	sta ft
	lda #0
	sbc fa_e+1
	bne @all		; 256 以上
	lda ft
	cmp #32
	bcs @all
	tay
	ldx #0
	jsr fl_shr_sticky
	jmp @e1
@all:	lda #1			; 全部消える (0 でないので sticky の 1)
	sta fa_m
	lda #0
	sta fa_m+1
	sta fa_m+2
	sta fa_m+3
@e1:	lda #1
	sta fa_e
	lda #0
	sta fa_e+1
@round:	lda fa_m
	and #7
	sta ft			; 丸めの印
	ldy #3
@sh:	lsr fa_m+3
	ror fa_m+2
	ror fa_m+1
	ror fa_m
	dey
	bne @sh
	lda ft
	cmp #5
	bcs @inc
	cmp #4
	bne @noinc
	lda fa_m
	and #1
	beq @noinc
@inc:	inc fa_m
	bne @noinc
	inc fa_m+1
	bne @noinc
	inc fa_m+2
	bne @noinc
	inc fa_m+3
@noinc:	lda fa_m+3		; 繰り上がって 25 ビットになった
	beq @ok
	lsr fa_m+3
	ror fa_m+2
	ror fa_m+1
	ror fa_m
	inc fa_e
	bne @ok
	inc fa_e+1
@ok:	lda fa_e+1
	bne @inf
	lda fa_e
	cmp #255
	bcs @inf
	lda fa_m+2
	bmi @normal
	lda fa_s		; 非正規化数 (指数のビットは 0)
	sta fa_m+3
	rts
@normal:
	lda fa_e
	lsr a			; C = 指数の下位のビット
	ora fa_s
	sta fa_m+3
	lda fa_m+2
	and #$7f
	bcc :+
	ora #$80
:	sta fa_m+2
	rts
@inf:	lda fa_s
	sta ft
	jmp fl_setinf

;;; ---------------------------------------------------------------
;;; + - (a と b のビットの並びから。結果は fa_m)
;;; ---------------------------------------------------------------
fl_add:
	FL_IF_NORMAL @order
	ldx #0
	jsr fl_expmax
	bcc @afin
	jsr fl_isnan
	bcs @nan
	ldx #FB			; a は無限大
	jsr fl_expmax
	bcc @r
	jsr fl_isnan
	bcs @nan
	lda fa_m+3		; inf - inf
	eor fb_m+3
	bmi @nan
@r:	rts
@nan:	jmp fl_setnan
@afin:	ldx #FB
	jsr fl_expmax
	bcc @fin
	jsr fl_isnan
	bcs @nan
	jmp fl_ret_b
@fin:	ldx #0
	jsr fl_iszero
	bne @anz
	ldx #FB
	jsr fl_iszero
	beq :+
	jmp fl_ret_b
:	lda fa_m+3		; 0 + 0 は +0 (両方 -0 なら -0)
	and fb_m+3
	sta fa_m+3
	rts
@anz:	ldx #FB
	jsr fl_iszero
	beq @r
	;; |a| < |b| なら入れ替える
@order:	lda fb_m+3
	and #$7f
	sta ft
	lda fa_m+3
	and #$7f
	cmp ft
	bcc @swap
	bne @keep
	lda fa_m+2
	cmp fb_m+2
	bcc @swap
	bne @keep
	lda fa_m+1
	cmp fb_m+1
	bcc @swap
	bne @keep
	lda fa_m
	cmp fb_m
	bcs @keep
@swap:	ldx #3
:	lda fa_m,x
	ldy fb_m,x
	sta fb_m,x
	sty fa_m,x
	dex
	bpl :-
@keep:	lda fa_m+3
	eor fb_m+3
	and #$80
	pha			; 符号が違えば $80
	ldx #0
	jsr fl_unpack
	jsr fl_shl3
	ldx #FB
	jsr fl_unpack
	jsr fl_shl3
	sec			; b の仮数を指数の差だけ右へ
	lda fa_e
	sbc fb_e
	tay
	lda fa_e+1
	sbc fb_e+1
	bne @far
	cpy #32
	bcs @far
	jsr fl_shr_sticky
	jmp @aligned
@far:	lda #1
	sta fb_m
	lda #0
	sta fb_m+1
	sta fb_m+2
	sta fb_m+3
@aligned:
	pla
	bne @sub
	clc
	lda fa_m
	adc fb_m
	sta fa_m
	lda fa_m+1
	adc fb_m+1
	sta fa_m+1
	lda fa_m+2
	adc fb_m+2
	sta fa_m+2
	lda fa_m+3
	adc fb_m+3
	sta fa_m+3
	and #$08		; 2^27 以上
	beq @pack
	ldy #1
	ldx #0
	jsr fl_shr_sticky
	inc fa_e
	bne @pack
	inc fa_e+1
@pack:	jmp fl_pack
@sub:	sec
	lda fa_m
	sbc fb_m
	sta fa_m
	lda fa_m+1
	sbc fb_m+1
	sta fa_m+1
	lda fa_m+2
	sbc fb_m+2
	sta fa_m+2
	lda fa_m+3
	sbc fb_m+3
	sta fa_m+3
	ora fa_m+2
	ora fa_m+1
	ora fa_m
	bne :+
	rts			; x - x は +0 (全部 0)
:	jmp fl_norm_pack

;;; ---------------------------------------------------------------
;;; ×
;;; ---------------------------------------------------------------
mp = F_float_mul32__scratch	; 48 ビットの積 (6 バイト)
mx = F_float_mul32__scratch+6
my = F_float_mul32__scratch+7

;;; A × Y (8 ビットどうし) → A (下位) / X (上位)。f(a + b) - f(|a - b|)、f(n) = n² / 4 の表 (runtime.asm)
fl_m8:
	sta mx
	sty my
	sec
	sbc my
	bcs :+
	eor #$ff
	adc #1
:	tay
	lda __mul_tbl_l0,y
	sta ft
	lda __mul_tbl_h0,y
	sta ft+1
	lda mx
	clc
	adc my
	tay
	bcs @hi
	lda __mul_tbl_l0,y
	sec
	sbc ft
	sta ft
	lda __mul_tbl_h0,y
	sbc ft+1
	tax
	lda ft
	rts
@hi:	lda __mul_tbl_l1,y
	sec
	sbc ft
	sta ft
	lda __mul_tbl_h1,y
	sbc ft+1
	tax
	lda ft
	rts

;;; mp[Y..] に繰り上がりの 1 を足していく (FL_MULADD が 2 バイトを足して C が立ったとき)
fl_carry:
	cpy #6
	bcs @r
	lda mp,y
	clc
	adc #1
	sta mp,y
	bcc @r
	iny
	jmp fl_carry
@r:	rts

;;; mp[k..] += a の仮数のバイト ia × b の仮数のバイト ib (足すのはその場で。繰り上がりは fl_carry)
.macro FL_MULADD ia, ib, k
	lda fa_m+ia
	ldy fb_m+ib
	jsr fl_m8
	clc
	adc mp+k
	sta mp+k
	txa
	adc mp+k+1
	sta mp+k+1
	bcc :+
	ldy #k+2
	jsr fl_carry
:
.endmacro

fl_mul:
	lda fa_m+3
	eor fb_m+3
	and #$80
	sta ft			; 符号
	FL_IF_NORMAL @go
	ldx #0
	jsr fl_isnan
	bcs @nan
	ldx #FB
	jsr fl_isnan
	bcs @nan
	ldx #0
	jsr fl_expmax
	bcc @afin
	ldx #FB			; inf × 0 は NaN
	jsr fl_iszero
	beq @nan
	jmp fl_setinf
@nan:	jmp fl_setnan
@afin:	ldx #FB
	jsr fl_expmax
	bcc @fin
	ldx #0
	jsr fl_iszero
	beq @nan
	jmp fl_setinf
@fin:	ldx #0
	jsr fl_iszero
	beq @zero
	ldx #FB
	jsr fl_iszero
	bne @go
@zero:	jmp fl_setzero
@go:	lda ft
	pha
	ldx #0
	jsr fl_unpack
	ldx #FB
	jsr fl_unpack
	pla
	sta fa_s
	clc			; e = ea + eb - 127
	lda fa_e
	adc fb_e
	sta fa_e
	lda fa_e+1
	adc fb_e+1
	sta fa_e+1
	sec
	lda fa_e
	sbc #127
	sta fa_e
	lda fa_e+1
	sbc #0
	sta fa_e+1
	lda #0			; mp = a × b (24 ビットどうし)
	ldx #5
:	sta mp,x
	dex
	bpl :-
	FL_MULADD 0, 0, 0
	FL_MULADD 0, 1, 1
	FL_MULADD 1, 0, 1
	FL_MULADD 0, 2, 2
	FL_MULADD 1, 1, 2
	FL_MULADD 2, 0, 2
	FL_MULADD 1, 2, 3
	FL_MULADD 2, 1, 3
	FL_MULADD 2, 2, 4
	;; 積は [2^46, 2^48)。上の 27 ビットを仮数に (下は sticky): mp >> 16 にしてから 4 か 5 ビット右へ
	lda mp+2
	sta fa_m
	lda mp+3
	sta fa_m+1
	lda mp+4
	sta fa_m+2
	lda mp+5
	sta fa_m+3
	lda mp
	ora mp+1
	beq :+
	lda fa_m
	ora #1
	sta fa_m
:	ldy #4
	lda fa_m+3
	bpl :+
	iny			; bit 47 が立っている: 5 ビット、指数 + 1
	inc fa_e
	bne :+
	inc fa_e+1
:	ldx #0
	jsr fl_shr_sticky
	jmp fl_pack

;;; ---------------------------------------------------------------
;;; ÷
;;; ---------------------------------------------------------------
dq = F_float_div32__scratch	; 商 (4 バイト)
dn = F_float_div32__scratch+4	; 繰り返しの数

fl_div:
	lda fa_m+3
	eor fb_m+3
	and #$80
	sta ft
	FL_IF_NORMAL @go
	ldx #0
	jsr fl_isnan
	bcs @nan
	ldx #FB
	jsr fl_isnan
	bcs @nan
	ldx #0
	jsr fl_expmax
	bcc @afin
	ldx #FB			; inf ÷ inf は NaN、inf ÷ x は inf
	jsr fl_expmax
	bcs @nan
	jmp fl_setinf
@nan:	jmp fl_setnan
@afin:	ldx #FB
	jsr fl_expmax
	bcc @fin
	jmp fl_setzero		; x ÷ inf は 0
@fin:	ldx #FB
	jsr fl_iszero
	bne @bnz
	ldx #0			; 0 ÷ 0 は NaN、x ÷ 0 は inf
	jsr fl_iszero
	beq @nan
	jmp fl_setinf
@bnz:	ldx #0
	jsr fl_iszero
	bne @go
	jmp fl_setzero
@go:	lda ft
	pha
	ldx #0
	jsr fl_unpack
	ldx #FB
	jsr fl_unpack
	pla
	sta fa_s
	sec			; e = ea - eb + 127
	lda fa_e
	sbc fb_e
	sta fa_e
	lda fa_e+1
	sbc fb_e+1
	sta fa_e+1
	clc
	lda fa_e
	adc #127
	sta fa_e
	lda fa_e+1
	adc #0
	sta fa_e+1
	;; m1 < m2 なら m1 を 1 ビット左へ、指数 - 1
	lda fa_m+2
	cmp fb_m+2
	bcc @lt
	bne @ge
	lda fa_m+1
	cmp fb_m+1
	bcc @lt
	bne @ge
	lda fa_m
	cmp fb_m
	bcs @ge
@lt:	asl fa_m
	rol fa_m+1
	rol fa_m+2
	rol fa_m+3
	lda fa_e
	bne :+
	dec fa_e+1
:	dec fa_e
@ge:	lda #0
	sta dq
	sta dq+1
	sta dq+2
	sta dq+3
	lda #26			; 商を 26 ビット (引き戻し)
	sta dn
@loop:	asl dq
	rol dq+1
	rol dq+2
	rol dq+3
	lda fa_m+3		; m1 >= m2 なら引いて商に 1
	cmp fb_m+3
	bcc @next
	bne @subt
	lda fa_m+2
	cmp fb_m+2
	bcc @next
	bne @subt
	lda fa_m+1
	cmp fb_m+1
	bcc @next
	bne @subt
	lda fa_m
	cmp fb_m
	bcc @next
@subt:	sec
	lda fa_m
	sbc fb_m
	sta fa_m
	lda fa_m+1
	sbc fb_m+1
	sta fa_m+1
	lda fa_m+2
	sbc fb_m+2
	sta fa_m+2
	lda fa_m+3
	sbc fb_m+3
	sta fa_m+3
	inc dq			; (最下位は 0 にしてある)
@next:	asl fa_m
	rol fa_m+1
	rol fa_m+2
	rol fa_m+3
	dec dn
	beq :+
	jmp @loop
:
	asl dq			; 最下位は余りが 0 でないか (sticky)
	rol dq+1
	rol dq+2
	rol dq+3
	lda fa_m
	ora fa_m+1
	ora fa_m+2
	ora fa_m+3
	beq :+
	inc dq
:	FL_LOAD dq, fa_m
	jmp fl_pack

;;; ---------------------------------------------------------------
;;; √ (正しく丸める)
;;; ---------------------------------------------------------------
sr = F_float_sqrt32__scratch	; 被開平数を上から 2 ビットずつ出す (4 バイト。m を上に寄せる)
srem = F_float_sqrt32__scratch+4	; 余り
sres = F_float_sqrt32__scratch+8	; 根 × 4
stry = F_float_sqrt32__scratch+12	; (sres | 1) の下位のバイト
sn = F_float_sqrt32__scratch+16

fl_sqrt:
	ldx #0
	jsr fl_isnan
	bcs @nan
	jsr fl_iszero
	beq @r			; ±0
	lda fa_m+3
	bmi @nan		; 負
	jsr fl_expmax
	bcc @fin
@r:	rts			; +inf
@nan:	jmp fl_setnan
@fin:	jsr fl_unpack
	sec			; ex = e - 127
	lda fa_e
	sbc #127
	sta fa_e
	lda fa_e+1
	sbc #0
	sta fa_e+1
	lda fa_e		; ex が奇数なら m を 1 ビット左へ、ex - 1
	lsr a
	bcc :+
	asl fa_m
	rol fa_m+1
	rol fa_m+2
	rol fa_m+3
	lda fa_e
	bne @dec
	dec fa_e+1
@dec:	dec fa_e
:	;; 被開平数 m × 2^29 (54 ビット): m (25 ビット以下) を bit 31 まで寄せる (下は 0 が入る)
	ldy #7
:	asl fa_m
	rol fa_m+1
	rol fa_m+2
	rol fa_m+3
	dey
	bne :-
	FL_LOAD fa_m, sr
	lda #0
	ldx #7
:	sta srem,x		; srem と sres
	dex
	bpl :-
	lda #27
	sta sn
@loop:	ldy #2			; rem = (rem << 2) | 上の 2 ビット
:	asl sr
	rol sr+1
	rol sr+2
	rol sr+3
	rol srem
	rol srem+1
	rol srem+2
	rol srem+3
	dey
	bne :-
	;; sres は根 × 4 で持つ (try = (根 << 2) | 1 を毎回作らない): rem >= (sres | 1) なら引いて、sres = (sres << 1) | 4
	lda sres
	ora #1
	sta stry
	lda srem+3
	cmp sres+3
	bcc @no
	bne @yes
	lda srem+2
	cmp sres+2
	bcc @no
	bne @yes
	lda srem+1
	cmp sres+1
	bcc @no
	bne @yes
	lda srem
	cmp stry
	bcc @no
@yes:	sec
	lda srem
	sbc stry
	sta srem
	lda srem+1
	sbc sres+1
	sta srem+1
	lda srem+2
	sbc sres+2
	sta srem+2
	lda srem+3
	sbc sres+3
	sta srem+3
	asl sres
	rol sres+1
	rol sres+2
	rol sres+3
	lda sres
	ora #4
	sta sres
	jmp @next
@no:	asl sres
	rol sres+1
	rol sres+2
	rol sres+3
@next:	dec sn
	beq :+
	jmp @loop
:	lsr sres+3		; 根 = sres >> 2
	ror sres+2
	ror sres+1
	ror sres
	lsr sres+3
	ror sres+2
	ror sres+1
	ror sres
	lda srem
	ora srem+1
	ora srem+2
	ora srem+3
	beq :+
	lda sres
	ora #1
	sta sres
:	FL_LOAD sres, fa_m
	lda #0
	sta fa_s
	lda fa_e+1		; e = (ex >> 1) + 127 (ex は偶数)
	cmp #$80
	ror a
	sta fa_e+1
	lda fa_e
	ror a
	clc
	adc #127
	sta fa_e
	lda fa_e+1
	adc #0
	sta fa_e+1
	jmp fl_pack

;;; ---------------------------------------------------------------
;;; 比較: A = 0 小さい / 1 等しい / 2 大きい / 3 順序が無い (NaN)
;;; ---------------------------------------------------------------

;;; X の値を大小の鍵にする (大きいほど大きい符号なしの値。-0 は +0 と同じ)
fl_key:
	jsr fl_iszero
	bne @nz
	sta fa_m,x		; (A = 0)
	sta fa_m+1,x
	sta fa_m+2,x
	lda #$80
	sta fa_m+3,x
	rts
@nz:	lda fa_m+3,x
	bmi @neg
	ora #$80
	sta fa_m+3,x
	rts
@neg:	ldy #3
@l:	lda fa_m,x
	eor #$ff
	sta fa_m,x
	inx
	dey
	bpl @l
	rts

fl_cmp:
	ldx #0
	jsr fl_isnan
	bcs @un
	ldx #FB
	jsr fl_isnan
	bcs @un
	ldx #0
	jsr fl_key
	ldx #FB
	jsr fl_key
	ldx #3
@l:	lda fa_m,x
	cmp fb_m,x
	bcc @lt
	bne @gt
	dex
	bpl @l
	lda #1
	rts
@lt:	lda #0
	rts
@gt:	lda #2
	rts
@un:	lda #3
	rts

;;; ---------------------------------------------------------------
;;; 整数との変換
;;; ---------------------------------------------------------------

;;; 絶対値 fa_m (0 でない、32 ビット) と符号 fa_s の f32 (最近接偶数に丸める)
fl_from_mag:
	lda #153		; 最上位が bit 26 なら値は仮数そのもの
	sta fa_e
	lda #0
	sta fa_e+1
@r:	lda fa_m+3		; 2^27 以上なら 1 ビットずつ右へ
	and #$f8
	beq @l
	ldy #1
	ldx #0
	jsr fl_shr_sticky
	inc fa_e
	jmp @r
@l:	jmp fl_norm_pack

;;; fa_m を 2 の補数で符号を反転する
fl_neg:
	sec
	ldx #0
	ldy #4
:	lda #0
	sbc fa_m,x
	sta fa_m,x
	inx
	dey
	bne :-
	rts

;;; 4 バイトの 0 か (Z = 1)
fl_m_zero:
	lda fa_m
	ora fa_m+1
	ora fa_m+2
	ora fa_m+3
	rts

;;; x × 2^n を整数にして (ts_rnd が 0 でなければ最近接偶数、0 なら 0 へ切り捨て)、ts_bits ビットの整数 (ts_sgn が 0 でなければ
;;; 符号付き) の範囲に飽和させる (NaN は 0)。f32 → 整数・固定小数の `as` (sema の floatCast) と to_i32 / to_u32。
;;; 仮数から直に丸める (ldexp32 → rint32 → to_i32 と同じ値: 2^n 倍を f32 に丸めて違うのは |値| < 2^-126 で、どちらも 0 になる)。
;;; 入力: fa_m = x のビットの並び。結果は fa_m (4 バイトの 2 の補数。sema が型の大きさに縮める)
ts_rnd = fb_s
ts_n = fb_e			; 2 バイト
ts_bits = fb_m
ts_sgn = fb_m+1
ts_r = fb_m+2			; 最後に消えたビット (半分の位)
ts_k = fb_m+3			; その下が 0 でないか

fl_to_sat:
	ldx #0
	jsr fl_isnan
	bcs @zero
	jsr fl_iszero
	beq @zero
	lda fa_m+3
	and #$80
	sta fa_s
	jsr fl_expmax
	bcc :+		; ±無限大
	jmp @sat
:
	jsr fl_unpack		; 値は m × 2^(e - 150)
	clc			; s = e - 150 + n
	lda fa_e
	adc ts_n
	sta ft
	lda fa_e+1
	adc ts_n+1
	sta ft+1
	sec
	lda ft
	sbc #150
	sta ft
	lda ft+1
	sbc #0
	sta ft+1
	bmi @right
	beq :+		; 2^9 以上ずらすと 32 ビットに入らない
	jmp @sat
:
	lda ft
	cmp #9
	bcc :+
	jmp @sat
:
	tay
	bne :+
	jmp @range
:
:	asl fa_m
	rol fa_m+1
	rol fa_m+2
	rol fa_m+3
	dey
	bne :-
	jmp @range
@zero:	lda #0
	sta fa_m
	sta fa_m+1
	sta fa_m+2
	sta fa_m+3
	rts
@right:	lda #0			; -s ビット右へ (消えたビットは ts_r / ts_k)
	sta ts_r
	sta ts_k
	sec
	sbc ft
	sta ft
	lda #0
	sbc ft+1
	bne @tiny
	lda ft
	cmp #26
	bcs @tiny
@b8:	lda ft
	cmp #8
	bcc @b1
	lda fa_m
	and #$7f
	ora ts_r
	ora ts_k
	sta ts_k
	lda fa_m
	and #$80
	sta ts_r
	lda fa_m+1
	sta fa_m
	lda fa_m+2
	sta fa_m+1
	lda #0
	sta fa_m+2
	lda ft
	sec
	sbc #8
	sta ft
	jmp @b8
@b1:	ldy ft
	beq @rnd
:	lda ts_r
	ora ts_k
	sta ts_k
	lsr fa_m+2
	ror fa_m+1
	ror fa_m
	lda #0
	rol a
	sta ts_r
	dey
	bne :-
	jmp @rnd
@tiny:	lda #0			; 全部消える (0 でないので sticky)
	sta fa_m
	sta fa_m+1
	sta fa_m+2
	sta ts_r
	lda #1
	sta ts_k
@rnd:	lda ts_rnd
	beq @range
	lda ts_r
	beq @range
	lda ts_k
	bne @inc
	lda fa_m
	and #1
	beq @range
@inc:	inc fa_m		; (1 ビット以上右へずらしたので 3 バイトに収まる)
	bne @range
	inc fa_m+1
	bne @range
	inc fa_m+2
@range:	ldy ts_bits		; 絶対値 fa_m を範囲に
	lda ts_sgn
	bne @signed
	lda fa_s		; 符号なし: 負は 0、2^bits 以上は 2^bits - 1
	beq :+
	jmp @zero
:
	jsr fl_over
	bcc @r
	ldy ts_bits
	jmp fl_ones
@signed:
	jsr fl_m_zero
	beq @r
	dey
	lda fa_s
	bne @neg
	jsr fl_over		; 正: 2^(bits-1) 以上は 2^(bits-1) - 1
	bcc @r
	ldy ts_bits
	dey
	jmp fl_ones
@neg:	jsr fl_dec		; 負: |x| - 1 が 2^(bits-1) 以上なら -2^(bits-1)、ほかは -|x| = ~(|x| - 1)
	ldy ts_bits
	dey
	jsr fl_over
	bcc fl_not
	ldy ts_bits
	dey
	jsr fl_ones
	jmp fl_not
@r:	rts
@sat:	ldy ts_bits		; 範囲の外・無限大: 符号の向きの端
	lda ts_sgn
	bne @ssat
	lda fa_s
	beq :+
	jmp @zero
:
	jmp fl_ones
@ssat:	dey
	jsr fl_ones
	lda fa_s
	beq @r
	;; (fl_not へ続く)

;;; fa_m のビットを反転する
fl_not:
	ldx #3
:	lda fa_m,x
	eor #$ff
	sta fa_m,x
	dex
	bpl :-
	rts

;;; fa_m から 1 を引く
fl_dec:
	ldx #0
:	lda fa_m,x
	sec
	sbc #1
	sta fa_m,x
	bcs @r
	inx
	cpx #4
	bne :-
@r:	rts

;;; fa_m (符号なし) が 2^Y 以上なら C = 1 (Y は 0〜32)
fl_over:
	cpy #32
	bcs @no
	tya
	lsr a
	lsr a
	lsr a
	tax			; Y / 8 バイト目
	tya
	and #7
	tay
	lda fa_m,x
@sh:	dey			; その中の Y % 8 ビットより上
	bmi @chk
	lsr a
	jmp @sh
@chk:	cmp #0
	bne @yes
@hi:	inx			; 上のバイト
	cpx #4
	bcs @no
	lda fa_m,x
	beq @hi
@yes:	sec
	rts
@no:	clc
	rts

;;; fa_m = 2^Y - 1 (Y は 0〜32)
fl_ones:
	ldx #0
@l:	cpy #8
	bcc @part
	lda #$ff
	sta fa_m,x
	tya
	sbc #8			; (C = 1)
	tay
	inx
	cpx #4
	bcc @l
	rts
@part:	lda #0
@p:	dey
	bmi @w
	sec
	rol a
	jmp @p
@w:	sta fa_m,x
	lda #0
@z:	inx
	cpx #4
	bcs @r
	sta fa_m,x
	jmp @z
@r:	rts

;;; ---------------------------------------------------------------
;;; 整数への丸め: A = 0 最近接偶数 / 1 負の無限大へ / 2 正の無限大へ / 3 0 へ / 4 四捨五入 (0 から遠い向き)
;;; ---------------------------------------------------------------
rd_mode = fb_s
rd_sh = fb_e
rd_rnd = fb_e+1
rd_stk = fb_m

fl_round:
	sta rd_mode
	ldx #0
	jsr fl_isnan
	bcc :+
	jmp fl_setnan
:	lda fa_m+2
	asl a
	lda fa_m+3
	rol a			; 指数のビット
	cmp #127+23
	bcc :+
	rts			; 無限大と、もう整数の値 (fa_m は x のまま)
:	tay
	jsr fl_iszero
	bne :+
	rts			; ±0
:	lda fa_m+3
	and #$80
	sta fa_s
	sta ft			; (0 になったときの符号)
	;; m (隠れたビット込み、24 ビット) と、ずらす量 sh (|x| < 1 は 24。|x| < 0.5 は m = 1)
	lda #0
	sta fa_m+3
	lda fa_m+2
	ora #$80
	sta fa_m+2
	lda #24
	sta rd_sh
	tya
	sec
	sbc #127
	bcc @small
	sta ft+1		; ex (0〜22)
	lda #23
	sec
	sbc ft+1
	sta rd_sh
	jmp @shift
@small:	cmp #<-1
	beq @shift
	lda #1
	sta fa_m
	lda #0
	sta fa_m+1
	sta fa_m+2
@shift:	lda #0
	sta rd_rnd		; 最後に消えたビット (半分の位)
	sta rd_stk		; その下が 0 でないか
@b8:	lda rd_sh
	cmp #8
	bcc @b1
	lda fa_m
	and #$7f
	ora rd_rnd
	ora rd_stk
	sta rd_stk
	lda fa_m
	and #$80
	sta rd_rnd
	lda fa_m+1
	sta fa_m
	lda fa_m+2
	sta fa_m+1
	lda #0
	sta fa_m+2
	lda rd_sh
	sec
	sbc #8
	sta rd_sh
	jmp @b8
@b1:	lda rd_sh
	beq @dec
	lda rd_rnd
	ora rd_stk
	sta rd_stk
	lsr fa_m+2
	ror fa_m+1
	ror fa_m
	lda #0
	rol a
	sta rd_rnd
	dec rd_sh
	jmp @b1
@dec:	ldx rd_mode		; 1 足すか
	beq @rne
	dex
	beq @floor
	dex
	beq @ceil
	dex
	beq @done
	lda rd_rnd		; 4: 半分以上
	bne @inc
	beq @done
@rne:	lda rd_rnd		; 0: 半分より上、ちょうど半分なら奇数のとき
	beq @done
	lda rd_stk
	bne @inc
	lda fa_m
	and #1
	bne @inc
	beq @done
@floor:	lda fa_s		; 1: 負で小数がある
	beq @done
	bne @frac
@ceil:	lda fa_s		; 2: 正で小数がある
	bne @done
@frac:	lda rd_rnd
	ora rd_stk
	beq @done
@inc:	inc fa_m
	bne @done
	inc fa_m+1
	bne @done
	inc fa_m+2
@done:	jsr fl_m_zero
	bne :+
	jmp fl_setzero		; 符号付きの 0 (ft)
:	jmp fl_from_mag

;;; ---------------------------------------------------------------
;;; x × 2^n (ft:ft+1 = n、符号付き)
;;; ---------------------------------------------------------------
fl_ldexp:
	ldx #0
	jsr fl_isnan
	bcc :+
	jmp fl_setnan
:	jsr fl_iszero
	beq @r
	jsr fl_expmax
	bcs @r
	lda ft
	pha
	lda ft+1
	pha
	jsr fl_unpack
	jsr fl_shl3
	pla
	tay
	pla
	clc
	adc fa_e
	sta fa_e
	tya
	adc fa_e+1
	sta fa_e+1
	jmp fl_pack
@r:	rts

;;; ---------------------------------------------------------------
;;; 公開の関数 (abi "frame": 戻り値が先頭、続いて引数)
;;; ---------------------------------------------------------------

_float_add32:
	FL_LOAD F_float_add32__a, fa_m
	FL_LOAD F_float_add32__b, fb_m
	jsr fl_add
	FL_LOAD fa_m, F_float_add32
	rts

_float_sub32:
	FL_LOAD F_float_sub32__a, fa_m
	FL_LOAD F_float_sub32__b, fb_m
	lda fb_m+3
	eor #$80
	sta fb_m+3
	jsr fl_add
	FL_LOAD fa_m, F_float_sub32
	rts

_float_mul32:
	FL_LOAD F_float_mul32__a, fa_m
	FL_LOAD F_float_mul32__b, fb_m
	jsr fl_mul
	FL_LOAD fa_m, F_float_mul32
	rts

_float_div32:
	FL_LOAD F_float_div32__a, fa_m
	FL_LOAD F_float_div32__b, fb_m
	jsr fl_div
	FL_LOAD fa_m, F_float_div32
	rts

_float_sqrt32:
	FL_LOAD F_float_sqrt32__a, fa_m
	jsr fl_sqrt
	FL_LOAD fa_m, F_float_sqrt32
	rts

_float_eq32:
	FL_LOAD F_float_eq32__a, fa_m
	FL_LOAD F_float_eq32__b, fb_m
	jsr fl_cmp
	ldx #0
	cmp #1
	bne :+
	inx
:	stx F_float_eq32
	rts

_float_lt32:
	FL_LOAD F_float_lt32__a, fa_m
	FL_LOAD F_float_lt32__b, fb_m
	jsr fl_cmp
	ldx #0
	cmp #0
	bne :+
	inx
:	stx F_float_lt32
	rts

_float_le32:
	FL_LOAD F_float_le32__a, fa_m
	FL_LOAD F_float_le32__b, fb_m
	jsr fl_cmp
	ldx #0
	cmp #2
	bcs :+
	inx
:	stx F_float_le32
	rts

_float_from_i32:
	FL_LOAD F_float_from_i32__n, fa_m
	lda #0
	sta fa_s
	jsr fl_m_zero
	beq @ret
	lda fa_m+3
	bpl :+
	sta fa_s		; (bit 7 だけ見る)
	jsr fl_neg
	lda #$80
	sta fa_s
:	jsr fl_from_mag
@ret:	FL_LOAD fa_m, F_float_from_i32
	rts

_float_from_u32:
	FL_LOAD F_float_from_u32__n, fa_m
	lda #0
	sta fa_s
	jsr fl_m_zero
	beq :+
	jsr fl_from_mag
:	FL_LOAD fa_m, F_float_from_u32
	rts

_float_to_i32:
	FL_LOAD F_float_to_i32__x, fa_m
	ldy #1
	jsr fl_to_int32
	FL_LOAD fa_m, F_float_to_i32
	rts

_float_to_u32:
	FL_LOAD F_float_to_u32__x, fa_m
	ldy #0
	jsr fl_to_int32
	FL_LOAD fa_m, F_float_to_u32
	rts

;;; 32 ビットの整数へ (0 へ切り捨て)。Y = 符号付きか
fl_to_int32:
	sty ts_sgn
	lda #0
	sta ts_n
	sta ts_n+1
	sta ts_rnd
	lda #32
	sta ts_bits
	jmp fl_to_sat

_float_to_sat_i:
	FL_LOAD F_float_to_sat_i__x, fa_m
	lda F_float_to_sat_i__n
	sta ts_n
	lda F_float_to_sat_i__n+1
	sta ts_n+1
	lda F_float_to_sat_i__rnd
	sta ts_rnd
	lda F_float_to_sat_i__bits
	sta ts_bits
	lda #1
	sta ts_sgn
	jsr fl_to_sat
	FL_LOAD fa_m, F_float_to_sat_i
	rts

_float_to_sat_u:
	FL_LOAD F_float_to_sat_u__x, fa_m
	lda F_float_to_sat_u__n
	sta ts_n
	lda F_float_to_sat_u__n+1
	sta ts_n+1
	lda F_float_to_sat_u__rnd
	sta ts_rnd
	lda F_float_to_sat_u__bits
	sta ts_bits
	lda #0
	sta ts_sgn
	jsr fl_to_sat
	FL_LOAD fa_m, F_float_to_sat_u
	rts

_float_rint32:
	FL_LOAD F_float_rint32__x, fa_m
	lda #0
	jsr fl_round
	FL_LOAD fa_m, F_float_rint32
	rts

_float_floor32:
	FL_LOAD F_float_floor32__x, fa_m
	lda #1
	jsr fl_round
	FL_LOAD fa_m, F_float_floor32
	rts

_float_ceil32:
	FL_LOAD F_float_ceil32__x, fa_m
	lda #2
	jsr fl_round
	FL_LOAD fa_m, F_float_ceil32
	rts

_float_trunc32:
	FL_LOAD F_float_trunc32__x, fa_m
	lda #3
	jsr fl_round
	FL_LOAD fa_m, F_float_trunc32
	rts

_float_round32:
	FL_LOAD F_float_round32__x, fa_m
	lda #4
	jsr fl_round
	FL_LOAD fa_m, F_float_round32
	rts

_float_ldexp32:
	FL_LOAD F_float_ldexp32__x, fa_m
	lda F_float_ldexp32__n
	sta ft
	lda F_float_ldexp32__n+1
	sta ft+1
	jsr fl_ldexp
	FL_LOAD fa_m, F_float_ldexp32
	rts

;;; ---------------------------------------------------------------
;;; f16 との変換
;;; ---------------------------------------------------------------

;;; f16 (fa_m の 2 バイト) → f32 (正確)
fl_from16:
	lda fa_m+1
	and #$80
	sta fa_s
	sta ft
	lda fa_m+1
	and #$7c
	lsr a
	lsr a
	sta fa_e		; 指数 (0〜31)
	lda fa_m+1
	and #$03
	sta fa_m+1		; 仮数 (10 ビット)
	lda fa_e
	cmp #31
	bne @fin
	lda fa_m
	ora fa_m+1
	beq :+
	jmp fl_setnan
:	jmp fl_setinf
@fin:	lda fa_e
	bne @norm
	lda fa_m
	ora fa_m+1
	bne @sub
	jmp fl_setzero
@sub:	lda #1			; 非正規化数: 隠れたビット (bit 10) が立つまでずらす
	sta fa_e
@sl:	lda fa_m+1
	and #$04
	bne @strip
	asl fa_m
	rol fa_m+1
	dec fa_e
	jmp @sl
@strip:	lda fa_m+1
	and #$03
	sta fa_m+1
@norm:	lda fa_e		; f32 の指数 = e - 15 + 127
	clc
	adc #112
	sta fa_e
	ldy #5			; 仮数 << 13 (5 ビットずらしてから 1 バイト上へ)
:	asl fa_m
	rol fa_m+1
	dey
	bne :-
	lda fa_m+1
	sta fa_m+2
	lda fa_m
	sta fa_m+1
	lda #0
	sta fa_m
	lda fa_e
	lsr a
	ora fa_s
	sta fa_m+3
	bcc :+
	lda fa_m+2
	ora #$80
	sta fa_m+2
:	rts

;;; f32 (fa_m) → f16 (fa_m の 2 バイト。最近接偶数に丸める、NaN は 0x7e00)
fl_to16:
	lda fa_m+3
	and #$80
	sta fa_s
	ldx #0
	jsr fl_isnan
	bcc :+
	lda #0
	sta fa_m
	lda #$7e
	sta fa_m+1
	rts
:	jsr fl_expmax
	bcc :+
	jmp @inf
:	jsr fl_iszero
	bne :+
	lda #0
	sta fa_m
	lda fa_s
	sta fa_m+1
	rts
:	jsr fl_unpack
	sec			; e = e - 127 + 15
	lda fa_e
	sbc #112
	sta fa_e
	lda fa_e+1
	sbc #0
	sta fa_e+1
	ldy #10			; 14 ビット (隠れたビットは bit 13、下の 3 ビットが丸めの印)
	ldx #0
	jsr fl_shr_sticky
	lda fa_e+1
	bmi @den
	ora fa_e
	bne @round
@den:	sec			; e が 1 より小さい: 1 - e ビット右へ (非正規化数)
	lda #1
	sbc fa_e
	sta ft
	lda #0
	sbc fa_e+1
	bne @all
	lda ft
	cmp #32
	bcs @all
	tay
	ldx #0
	jsr fl_shr_sticky
	jmp @e1
@all:	lda #1
	sta fa_m
	lda #0
	sta fa_m+1
	sta fa_m+2
	sta fa_m+3
@e1:	lda #1
	sta fa_e
	lda #0
	sta fa_e+1
@round:	lda fa_m
	and #7
	sta ft
	ldy #3
@sh:	lsr fa_m+1
	ror fa_m
	dey
	bne @sh
	lda ft
	cmp #5
	bcs @inc
	cmp #4
	bne @noinc
	lda fa_m
	and #1
	beq @noinc
@inc:	inc fa_m
	bne @noinc
	inc fa_m+1
@noinc:	lda fa_m+1		; 繰り上がって 12 ビットになった
	and #$08
	beq @ok
	lsr fa_m+1
	ror fa_m
	inc fa_e
	bne @ok
	inc fa_e+1
@ok:	lda fa_e+1
	bne @inf
	lda fa_e
	cmp #31
	bcs @inf
	lda fa_m+1
	and #$04
	bne @normal
	lda fa_m+1		; 非正規化数 (指数のビットは 0)
	ora fa_s
	sta fa_m+1
	rts
@normal:
	lda fa_e
	asl a
	asl a
	sta ft
	lda fa_m+1
	and #$03
	ora ft
	ora fa_s
	sta fa_m+1
	rts
@inf:	lda #0
	sta fa_m
	lda fa_s
	ora #$7c
	sta fa_m+1
	rts

_float_from16:
	lda F_float_from16__h
	sta fa_m
	lda F_float_from16__h+1
	sta fa_m+1
	jsr fl_from16
	FL_LOAD fa_m, F_float_from16
	rts

_float_to16:
	FL_LOAD F_float_to16__x, fa_m
	jsr fl_to16
	lda fa_m
	sta F_float_to16
	lda fa_m+1
	sta F_float_to16+1
	rts
