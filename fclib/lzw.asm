;;; lzw の展開 (lzw.fc)。公開の関数はどれも abi "frame" (引数と戻り値は F_lzw_<名前> のフレーム。ほかの関数を呼ばない)。
;;; ビットを読む中心の処理 lzw_bits は fc からは見えない下請け (公開の関数の中からだけ呼ぶ): X に読むビット数 (0〜16)、
;;; 結果は _lzw_rbits (2 バイト)。A / X / Y を壊す。

addr = _lzw_addr
bpos = _lzw_bpos 			; curの残りビット数
cur = _lzw_cur				; 現在読んでいるbyteの内容
rbits = _lzw_rbits			; lzw_bits の結果

;;; unpack の作業領域 (フレームの scratch)
dest_addr = F_lzw_unpack__dest		; 引数の dest をその場で進める
total_len = F_lzw_unpack__scratch+0
idx = F_lzw_unpack__scratch+2
len = F_lzw_unpack__scratch+3
copy_src = F_lzw_unpack__scratch+4

;;; X ビット読んで rbits に (上位から詰める)
.proc lzw_bits
	lda #0
	sta rbits
	sta rbits+1
	cpx #0
	beq @end
@loop:
	;; byteを使いきったら読み込む
	lda bpos
	bne @no_read
	tay 						; == ldy #0
	lda (addr),y
	sta cur

	inc addr+0
	bne :+
	inc addr+1
:
	lda #8
	sta bpos
@no_read:

	;; curから1bit読み込む
	rol cur
	rol rbits+0
	rol rbits+1

	dec bpos
	dex
	bne @loop
@end:
	rts
.endproc

;;; 長さの可変な数: 1 ビット読んで 0 なら short ビット、1 なら long ビット読む (X = short、Y = long)
.proc lzw_vln
	tya
	pha							; long
	txa
	pha							; short
	ldx #1
	jsr lzw_bits
	pla
	tax							; short
	pla							; long
	ldy rbits
	beq :+
	tax							; long
:	jmp lzw_bits
.endproc

;; function read_bit(n:u8):u16
.proc _lzw_read_bit
	ldx F_lzw_read_bit__n
	jsr lzw_bits
	lda rbits
	sta F_lzw_read_bit+0
	lda rbits+1
	sta F_lzw_read_bit+1
	rts
.endproc

;; function read_vln():u8
.proc _lzw_read_vln
	ldx #4
	ldy #8
	jsr lzw_vln
	lda rbits
	sta F_lzw_read_vln+0
	rts
.endproc

;; function read_vln16():u16
.proc _lzw_read_vln16
	ldx #8
	ldy #16
	jsr lzw_vln
	lda rbits
	sta F_lzw_read_vln16+0
	lda rbits+1
	sta F_lzw_read_vln16+1
	rts
.endproc

;; function unpack(dest:*u8, src:*const u8):u16 (展開した長さを返す)
.proc _lzw_unpack
	lda F_lzw_unpack__src+0
	sta addr+0
	lda F_lzw_unpack__src+1
	sta addr+1
	lda #0
	sta bpos

	ldx #8						; total_len = read_vln16();
	ldy #16
	jsr lzw_vln
	lda rbits
	sta total_len+0
	sta F_lzw_unpack+0
	lda rbits+1
	sta total_len+1
	sta F_lzw_unpack+1

@loop:
	lda total_len+0				; while(total_len){
	ora total_len+1
	bne @not_end
	rts
@not_end:

	ldx #1						;   if(read_bit(1)==0){
	jsr lzw_bits
	lda rbits
	bne @normal

	ldx #4						;     idx = read_vln();
	ldy #8
	jsr lzw_vln
	lda rbits
	sta idx

	ldx #4						;     len = read_vln();
	ldy #8
	jsr lzw_vln
	lda rbits
	sta len

	sec							;     copy(dest_addr, dest_addr-idx, len) (前から 1 バイトずつ: 重なってもよい)
	lda dest_addr+0
	sbc idx
	sta copy_src+0
	lda dest_addr+1
	sbc #0
	sta copy_src+1
	ldy #0
	lda len
	beq @copied
@copy:
	lda (copy_src),y
	sta (dest_addr),y
	iny
	cpy len
	bne @copy
@copied:

	sec							;     total_len -= len;
	lda total_len+0
	sbc len
	sta total_len+0
	lda total_len+1
	sbc #0
	sta total_len+1

	clc							;     dest_addr += len;
	lda dest_addr+0
	adc len
	sta dest_addr+0
	lda dest_addr+1
	adc #0
	sta dest_addr+1
	jmp @loop
@normal:						;   }else{

	ldx #8						;     *dest_addr = read_bit(8);
	jsr lzw_bits

	ldy #0
	lda rbits
	sta (dest_addr),y

	sec							;     total_len -= 1;
	lda total_len+0
	sbc #1
	sta total_len+0
	lda total_len+1
	sbc #0
	sta total_len+1

	inc dest_addr+0				;     dest_addr += 1;
	bne :+
	inc dest_addr+1
:
	jmp @loop					; } }
.endproc
