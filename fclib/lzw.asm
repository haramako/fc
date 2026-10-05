;;; lzw.asm: 独自の LZW 形式の展開 (lzw.fc)。unpack_raw は abi "frame" (ほかの関数を呼ばない)。
;;; 形式 (ビットは各バイトの上位から読む): 展開した長さ vln(8, 16)、続けてその長さになるまで「1 + 8 ビットの文字」か
;;; 「0 + 距離 vln(4, 8) + 長さ vln(4, 8)」(書いた所の距離だけ前から長さだけ写す。前から 1 バイトずつなので重なってよい)。
;;; vln(s, l) は 1 ビット読んで 0 なら s ビット、1 なら l ビットの数。
;;; 戻り値は書いた長さ。書き先が足りない・データが壊れている (距離が 0 か書き先の頭より前、一致が長さを超える、入力が途中で
;;; 終わる) なら $FFFF。入力の終わりは lzw_bits の中で調べ、壊れていたらスタックを入口の高さに戻して失敗を返す。

dst = F_lzw_unpack_raw__dst			; 書く位置 (進める)
rest = F_lzw_unpack_raw__dst_len		; 始めは書き先の長さ、展開した長さを読んだ後は残りの長さ
src = F_lzw_unpack_raw__src			; 読む位置 (進める)
src_end = F_lzw_unpack_raw__src_len		; 入力の終わり (始めに src + src_len にする)
bpos = F_lzw_unpack_raw__scratch+0		; cur の残りのビット数
cur = F_lzw_unpack_raw__scratch+1		; 読んでいるバイト
rbits = F_lzw_unpack_raw__scratch+2		; lzw_bits の結果 (2 バイト)
idx = F_lzw_unpack_raw__scratch+4		; 一致の距離
len = F_lzw_unpack_raw__scratch+5		; 一致の長さ
from = F_lzw_unpack_raw__scratch+6		; 写す元 (2 バイト)
dst0 = F_lzw_unpack_raw__scratch+8		; 書き先の頭 (2 バイト)
stack0 = F_lzw_unpack_raw__scratch+10		; 入口のスタックの高さ (失敗したときに戻す)

;;; X ビット (0〜16) 読んで rbits に (上位から詰める)。A / X / Y を壊す。入力が終わっていれば lzw_fail へ
lzw_bits:
	lda #0
	sta rbits
	sta rbits+1
	cpx #0
	beq @end
@loop:
	lda bpos				; cur を使い切ったら読む
	bne @no_read
	lda src
	cmp src_end
	bne @read
	lda src+1
	cmp src_end+1
	bne @read
	jmp lzw_fail
@read:
	ldy #0
	lda (src),y
	sta cur
	inc src
	bne :+
	inc src+1
:	lda #8
	sta bpos
@no_read:
	rol cur					; cur から 1 ビット
	rol rbits
	rol rbits+1
	dec bpos
	dex
	bne @loop
@end:
	rts

;;; 長さの可変な数: 1 ビット読んで 0 なら X ビット、1 なら Y ビット読む
lzw_vln:
	tya
	pha					; long
	txa
	pha					; short
	ldx #1
	jsr lzw_bits
	pla
	tax					; short
	pla					; long
	ldy rbits
	beq :+
	tax					; long
:	jmp lzw_bits

lzw_fail:
	ldx stack0
	txs
	lda #$ff
	sta F_lzw_unpack_raw+0
	sta F_lzw_unpack_raw+1
	rts

;; function unpack_raw(dst:*u8, dst_len:u16, src:*const u8, src_len:u16):u16
_lzw_unpack_raw:
	tsx
	stx stack0
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
	lda #0
	sta bpos

	ldx #8					; 展開した長さ (書き先より長ければ失敗)
	ldy #16
	jsr lzw_vln
	lda rest
	cmp rbits
	lda rest+1
	sbc rbits+1
	bcc @fail
	lda rbits
	sta rest
	lda rbits+1
	sta rest+1

@loop:
	lda rest
	ora rest+1
	bne @item
	sec					; 書いた長さを返す
	lda dst
	sbc dst0
	sta F_lzw_unpack_raw+0
	lda dst+1
	sbc dst0+1
	sta F_lzw_unpack_raw+1
	rts
@fail:
	jmp lzw_fail

@item:
	ldx #1
	jsr lzw_bits
	lda rbits
	bne @lit

	ldx #4					; 距離 (0 なら壊れている)
	ldy #8
	jsr lzw_vln
	lda rbits
	beq @fail
	sta idx
	ldx #4					; 長さ (残りを超えたら壊れている)
	ldy #8
	jsr lzw_vln
	lda rbits
	sta len
	lda rest+1
	bne :+
	lda rest
	cmp len
	bcc @fail
:
	sec					; from = dst - idx (書き先の頭より前 (番地の折り返しも) なら壊れている)
	lda dst
	sbc idx
	sta from
	lda dst+1
	sbc #0
	sta from+1
	bcc @fail
	lda from
	cmp dst0
	lda from+1
	sbc dst0+1
	bcc @fail

	ldy #0
	lda len
	beq @copied
@copy:
	lda (from),y
	sta (dst),y
	iny
	cpy len
	bne @copy
@copied:
	sec					; rest -= len
	lda rest
	sbc len
	sta rest
	lda rest+1
	sbc #0
	sta rest+1
	clc					; dst += len
	lda dst
	adc len
	sta dst
	bcc @loop
	inc dst+1
	jmp @loop

@lit:
	ldx #8					; 文字
	jsr lzw_bits
	ldy #0
	lda rbits
	sta (dst),y
	lda rest				; rest -= 1
	bne :+
	dec rest+1
:	dec rest
	inc dst
	bne @loop2
	inc dst+1
@loop2:
	jmp @loop
