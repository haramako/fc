;;; 24 / 32 ビットの掛け算・割り算・余り (Agent/wiki/design/wide-int.md)。
;;; codegen が 3・4 バイトの mul / div / mod を出したビルドだけリンクする (driver。使わないビルドの ROM は変わらない)。
;;; 3 バイトの値は codegen が 4 バイトに広げて渡し、下位 3 バイトを使う。
;;; 入力: a = reg(0..3)、b = reg(4..7)。結果: reg(8..11)。割り算の余りは reg(12..15)。どれも A・Y と reg を壊し、X は保つ
;;; (codegen/asm.go の jsr __mul_ / __div_ / __mod_ の例外)。

	.importzp reg

	.export __mul_32
	.export __div_32
	.export __div_32s
	.export __mod_32
	.export __mod_32s

.segment "FC_RUNTIME"

;;; reg(8..11) = reg(0..3) * reg(4..7) (下位 32 ビット。符号付きでも同じ)
.proc __mul_32
	lda #0
	sta reg+8
	sta reg+9
	sta reg+10
	sta reg+11
	ldy #32
@loop:
	lsr reg+7				; b >>= 1、押し出したビットが C
	ror reg+6
	ror reg+5
	ror reg+4
	bcc @skip
	clc						; 結果 += a
	lda reg+8
	adc reg+0
	sta reg+8
	lda reg+9
	adc reg+1
	sta reg+9
	lda reg+10
	adc reg+2
	sta reg+10
	lda reg+11
	adc reg+3
	sta reg+11
@skip:
	asl reg+0				; a <<= 1
	rol reg+1
	rol reg+2
	rol reg+3
	dey
	bne @loop
	rts
.endproc

;;; 符号なし: reg(8..11) = reg(0..3) / reg(4..7)、余り = reg(12..15)
;;; 商は被除数の場所 reg(0..3) に下から作り、最後に reg(8..11) へ写す (reg(8..11) は引き算の試しに使う)
.proc __div_32
	lda #0
	sta reg+12
	sta reg+13
	sta reg+14
	sta reg+15
	ldy #32
@loop:
	asl reg+0				; 余り:被除数 (64 ビット) を 1 つ左へ
	rol reg+1
	rol reg+2
	rol reg+3
	rol reg+12
	rol reg+13
	rol reg+14
	rol reg+15
	lda #0					; 余りから押し出したビット (除数が 2^31 以上のとき 33 ビットになる)
	rol a
	pha
	sec						; 試し = 余り - 除数
	lda reg+12
	sbc reg+4
	sta reg+8
	lda reg+13
	sbc reg+5
	sta reg+9
	lda reg+14
	sbc reg+6
	sta reg+10
	lda reg+15
	sbc reg+7
	sta reg+11
	pla						; pla は C を変えない
	bne @take				; 33 ビット目が立っていれば必ず引ける
	bcc @skip
@take:
	lda reg+8
	sta reg+12
	lda reg+9
	sta reg+13
	lda reg+10
	sta reg+14
	lda reg+11
	sta reg+15
	inc reg+0				; 商のビット (asl の後なので最下位は 0)
@skip:
	dey
	bne @loop
	lda reg+0
	sta reg+8
	lda reg+1
	sta reg+9
	lda reg+2
	sta reg+10
	lda reg+3
	sta reg+11
	rts
.endproc

;;; 符号なし: reg(8..11) = reg(0..3) % reg(4..7)
.proc __mod_32
	jsr __div_32
	jmp copy_rem
.endproc

;;; 符号付き: reg(8..11) = reg(0..3) / reg(4..7) (__div_16s と同じ床除算: 商は負の無限大方向に丸め、余りの符号は除数に合わせる)
;;; 余り = reg(12..15)。符号の印はスタックに置く
.proc __div_32s
	lda reg+7				; 除数の符号
	and #$80
	pha
	eor reg+3				; 商の符号 = 被除数の符号 xor 除数の符号
	and #$80
	pha
	lda reg+3				; 被除数が負なら反転
	bpl @a_pos
	sec
	lda #0
	sbc reg+0
	sta reg+0
	lda #0
	sbc reg+1
	sta reg+1
	lda #0
	sbc reg+2
	sta reg+2
	lda #0
	sbc reg+3
	sta reg+3
@a_pos:
	lda reg+7				; 除数が負なら反転
	bpl @b_pos
	sec
	lda #0
	sbc reg+4
	sta reg+4
	lda #0
	sbc reg+5
	sta reg+5
	lda #0
	sbc reg+6
	sta reg+6
	lda #0
	sbc reg+7
	sta reg+7
@b_pos:
	jsr __div_32
	pla						; 商が負なら商 = -商、余りがあれば商 -1、余り = |除数| - 余り
	beq @else1
	sec
	lda #0
	sbc reg+8
	sta reg+8
	lda #0
	sbc reg+9
	sta reg+9
	lda #0
	sbc reg+10
	sta reg+10
	lda #0
	sbc reg+11
	sta reg+11
	lda reg+12
	ora reg+13
	ora reg+14
	ora reg+15
	beq @else1
	sec
	lda reg+4
	sbc reg+12
	sta reg+12
	lda reg+5
	sbc reg+13
	sta reg+13
	lda reg+6
	sbc reg+14
	sta reg+14
	lda reg+7
	sbc reg+15
	sta reg+15
	lda reg+8				; 商 -= 1
	bne @d0
	lda reg+9
	bne @d1
	lda reg+10
	bne @d2
	dec reg+11
@d2:
	dec reg+10
@d1:
	dec reg+9
@d0:
	dec reg+8
@else1:
	pla						; 除数が負なら余り = -余り
	beq @else2
	sec
	lda #0
	sbc reg+12
	sta reg+12
	lda #0
	sbc reg+13
	sta reg+13
	lda #0
	sbc reg+14
	sta reg+14
	lda #0
	sbc reg+15
	sta reg+15
@else2:
	rts
.endproc

;;; 符号付き: reg(8..11) = reg(0..3) % reg(4..7)
.proc __mod_32s
	jsr __div_32s
	jmp copy_rem
.endproc

;;; 余り reg(12..15) を結果 reg(8..11) へ
.proc copy_rem
	lda reg+12
	sta reg+8
	lda reg+13
	sta reg+9
	lda reg+14
	sta reg+10
	lda reg+15
	sta reg+11
	rts
.endproc
