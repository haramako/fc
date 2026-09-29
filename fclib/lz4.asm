;;; lz4.asm: LZ4 のブロック形式の展開 (lz4.fc)。unpack_raw は abi "frame" (ほかの関数を呼ばない)。
;;; 入力を読み終えたら終わる (最後の列は文字だけ)。戻り値は書いた長さ。書き先が足りない・データが壊れている (距離が 0 か
;;; 書き先の頭より前、途中で終わる) なら $FFFF。一致は 1 バイトずつ前から写すので、重なり (距離 < 長さ) もよい。
;;; 速さ: 列ごとの手間を減らす (入力の終わりの判定・読む位置の進め方はその場に書き、長さの続きのバイトを読むのは長さが 15 の
;;; ときだけ呼ぶ。書き先の残りは dst_len を減らして数える)。

dst = F_lz4_unpack_raw__dst			; 書く位置 (進める)
rem = F_lz4_unpack_raw__dst_len			; 書き先の残り (写すたびに減らす)
src = F_lz4_unpack_raw__src			; 読む位置 (進める)
src_end = F_lz4_unpack_raw__src_len		; 入力の終わり (始めに src + src_len にする)
token = F_lz4_unpack_raw__scratch+0
len = F_lz4_unpack_raw__scratch+1		; 写す長さ (2 バイト。copy の中で符号を反転して数える)
from = F_lz4_unpack_raw__scratch+3		; 写す元 (2 バイト)
dst0 = F_lz4_unpack_raw__scratch+5		; 書き先の頭 (2 バイト)

;;; Z = 1 なら入力を読み終えた (A を壊す)
.macro LZ4_AT_END
	lda src
	cmp src_end
	bne :+
	lda src+1
	cmp src_end+1
:
.endmacro

.macro LZ4_INC_SRC
	inc src
	bne :+
	inc src+1
:
.endmacro

_lz4_unpack_raw:
	lda dst
	sta dst0
	lda dst+1
	sta dst0+1
	clc
	lda src
	adc src_end
	sta src_end
	lda src+1
	adc src_end+1
	sta src_end+1
@seq:
	LZ4_AT_END
	beq @done
	ldy #0
	lda (src),y
	sta token
	LZ4_INC_SRC
	lsr a				; 文字の数
	lsr a
	lsr a
	lsr a
	sta len
	sty len+1
	cmp #15
	bne @lit
	jsr lz4_read_ext
	bcs @fail
@lit:
	lda len+1
	bne @litlong
	lda len
	beq @nolit			; 文字が無い (一致だけの列)
	sec				; rem -= len (足りなければ失敗)
	lda rem
	sbc len
	tax
	lda rem+1
	sbc #0
	bcc @fail
	sta rem+1
	stx rem
	ldy #0				; 255 文字まで: 入力から直に写す (from を経ない)
@litcopy:
	lda (src),y
	sta (dst),y
	iny
	cpy len
	bne @litcopy
	tya
	clc
	adc src
	sta src
	bcc :+
	inc src+1
:
	tya
	clc
	adc dst
	sta dst
	bcc @nolit
	inc dst+1
	jmp @nolit
@done:
	sec
	lda dst
	sbc dst0
	sta F_lz4_unpack_raw+0
	lda dst+1
	sbc dst0+1
	sta F_lz4_unpack_raw+1
	rts
@fail:
	lda #$ff
	sta F_lz4_unpack_raw+0
	sta F_lz4_unpack_raw+1
	rts
@litlong:				; 256 文字以上 (まれ)
	lda src
	sta from
	lda src+1
	sta from+1
	jsr lz4_copy
	bcs @fail
	lda from
	sta src
	lda from+1
	sta src+1
@nolit:
	LZ4_AT_END			; 最後の列 (文字だけ)
	beq @done
	;; 距離: from = dst - 距離 (距離 0・書き先の頭より前 (番地の折り返しも) なら壊れている)
	ldy #0
	sec
	lda dst
	sbc (src),y
	sta from
	iny
	lda dst+1
	sbc (src),y
	sta from+1
	bcc @fail			; 番地が折り返した
	lda (src),y
	dey
	ora (src),y
	beq @fail			; 距離 0
	lda from
	cmp dst0
	lda from+1
	sbc dst0+1
	bcc @fail
	clc				; src += 2
	lda src
	adc #2
	sta src
	bcc :+
	inc src+1
:
	lda token			; 一致の長さ - 4
	and #$0f
	sta len
	sty len+1			; Y = 0
	cmp #15
	bne @match
	jsr lz4_read_ext
	bcs @fail
@match:
	lda len
	clc
	adc #4
	sta len
	bcc :+
	inc len+1
:
	lda len+1
	bne @matchlong
	sec				; 255 バイトまで (たいていこちら): rem -= len
	lda rem
	sbc len
	tax
	lda rem+1
	sbc #0
	bcc @failj
	sta rem+1
	stx rem
	ldy #0
@mcopy:
	lda (from),y
	sta (dst),y
	iny
	cpy len
	bne @mcopy
	tya
	clc
	adc dst
	sta dst
	bcc :+
	inc dst+1
:
	jmp @seq
@matchlong:
	jsr lz4_copy
	bcs @failj
	jmp @seq
@failj:
	jmp @fail

;;; 長さ (len = 15) に続くバイトを足す (255 なら更に続く)。途中で入力が終われば C = 1
lz4_read_ext:
	LZ4_AT_END
	beq @fail
	ldy #0
	lda (src),y
	tax
	LZ4_INC_SRC
	txa
	clc
	adc len
	sta len
	bcc :+
	inc len+1
:
	cpx #255
	beq lz4_read_ext
	clc
	rts
@fail:
	sec
	rts

;;; len バイトを (from) から (dst) へ写し、from / dst を進める。書き先が足りなければ C = 1 (何も書かない)
lz4_copy:
	lda len
	ora len+1
	beq @ok
	sec				; rem -= len (足りなければ失敗)
	lda rem
	sbc len
	tax
	lda rem+1
	sbc len+1
	bcc @fail
	sta rem+1
	stx rem
	ldy #0
	lda len+1
	bne @long
@short:					; 255 バイトまで (一致はたいていこちら): Y で数える
	lda (from),y
	sta (dst),y
	iny
	cpy len
	bne @short
	beq @advance
@long:
	sec				; len = -len (0 になるまで 1 ずつ足す)
	lda #0
	sbc len
	sta len
	lda #0
	sbc len+1
	sta len+1
@loop:
	lda (from),y
	sta (dst),y
	iny
	bne @n
	inc from+1
	inc dst+1
@n:
	inc len
	bne @loop
	inc len+1
	bne @loop
@advance:
	tya				; 端数の分を進める
	clc
	adc from
	sta from
	bcc @f
	inc from+1
@f:
	tya
	clc
	adc dst
	sta dst
	bcc @ok
	inc dst+1
@ok:
	clc
	rts
@fail:
	sec
	rts
