;;; lz4.asm: LZ4 のブロック形式の展開 (lz4.fc)。unpack_raw は abi "frame" (ほかの関数を呼ばない)。
;;; 入力を読み終えたら終わる (最後の列は文字だけ)。戻り値は書いた長さ。書き先が足りない・データが壊れている (距離が 0 か
;;; 書き先の頭より前、途中で終わる) なら $FFFF。一致は 1 バイトずつ前から写すので、重なり (距離 < 長さ) もよい。

dst = F_lz4_unpack_raw__dst			; 書く位置 (進める)
dst_end = F_lz4_unpack_raw__dst_len		; 書き先の終わり (始めに dst + dst_len にする)
src = F_lz4_unpack_raw__src			; 読む位置 (進める)
src_end = F_lz4_unpack_raw__src_len		; 入力の終わり (始めに src + src_len にする)
token = F_lz4_unpack_raw__scratch+0
len = F_lz4_unpack_raw__scratch+1		; 写す長さ (2 バイト。copy の中で符号を反転して数える)
from = F_lz4_unpack_raw__scratch+3		; 写す元 (2 バイト)
dst0 = F_lz4_unpack_raw__scratch+5		; 書き先の頭 (2 バイト)

_lz4_unpack_raw:
	lda dst
	sta dst0
	clc
	adc dst_end
	sta dst_end
	lda dst+1
	sta dst0+1
	adc dst_end+1
	sta dst_end+1
	lda src
	clc
	adc src_end
	sta src_end
	lda src+1
	adc src_end+1
	sta src_end+1
@seq:
	jsr lz4_at_end
	beq @done
	ldy #0
	lda (src),y
	sta token
	jsr lz4_inc_src
	lsr a				; 文字の数
	lsr a
	lsr a
	lsr a
	jsr lz4_read_len
	bcs @fail
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
	jsr lz4_at_end			; 最後の列 (文字だけ)
	beq @done
	;; 距離: from = dst - 距離 (dst0 より前・距離 0 なら壊れている)
	ldy #1
	lda (src),y
	tax
	dey
	sec
	lda dst
	sbc (src),y
	sta from
	txa
	sta len				; (距離の上位を一時に)
	lda dst+1
	sbc len
	sta from+1
	lda (src),y
	ora len
	beq @fail			; 距離 0
	jsr lz4_inc_src
	jsr lz4_inc_src
	lda from
	cmp dst0
	lda from+1
	sbc dst0+1
	bcc @fail
	lda token			; 一致の長さ - 4
	and #$0f
	jsr lz4_read_len
	bcs @fail
	lda len
	clc
	adc #4
	sta len
	bcc @copy
	inc len+1
@copy:
	jsr lz4_copy
	bcs @fail
	jmp @seq
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

;;; Z = 1 なら入力を読み終えた
lz4_at_end:
	lda src
	cmp src_end
	bne @r
	lda src+1
	cmp src_end+1
@r:
	rts

lz4_inc_src:
	inc src
	bne @r
	inc src+1
@r:
	rts

;;; A (0〜15) の長さを len に。15 なら続くバイトを足す (255 なら更に続く)。途中で入力が終われば C = 1
lz4_read_len:
	sta len
	ldy #0
	sty len+1
	cmp #15
	bne @ok
@more:
	jsr lz4_at_end
	beq @fail
	ldy #0
	lda (src),y
	tax
	jsr lz4_inc_src
	txa
	clc
	adc len
	sta len
	bcc @nc
	inc len+1
@nc:
	cpx #255
	beq @more
@ok:
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
	sec				; 残り = dst_end - dst が len 以上か
	lda dst_end
	sbc dst
	tax
	lda dst_end+1
	sbc dst+1
	cmp len+1
	bcc @fail
	bne @fits
	cpx len
	bcc @fail
@fits:
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
