;;; pal.asm: pal.fc の shade (abi "frame"。fade のたびに 32 色ぶん呼ぶ。fc で書くと 123 バイト)。
;;; 規則は pal.fc の shade の説明のとおり: 暗くするときは 1 段ごとに $10 を引き、無くなれば黒 ($0D も黒に)。明るくするときは
;;; $10 を足し、$3x を超えれば白。黒 ($xE / $xF) は明るくすると $00 → $10 → $20 → $30 の灰色に (暗くすると黒のまま)。
;;; 注意: .segment は書かない (pal モジュールのセグメントに置かれる)。

sh_c = F_pal_shade__c
sh_d = F_pal_shade__d
sh_r = F_pal_shade+0			; 戻り値 (途中では作業域にも使う)

_pal_shade:
	lda sh_c
	ldy sh_d
	beq @ret			; d = 0: そのまま
	and #$0f
	cmp #$0e
	bcs @gray			; $xE / $xF
	lda sh_c			; A = 上位 (明るさの段)
	lsr a
	lsr a
	lsr a
	lsr a
	cpy #$80
	bcs @dark
	adc sh_d			; 明るく: 上位 + d が 4 以上なら白 (C = 0)
	cmp #4
	bcs @white
	tya				; c + (d << 4) (d は 3 以下なので C = 0 のまま)
	asl a
	asl a
	asl a
	asl a
	adc sh_c
	bcc @ret			; (いつも)
@dark:
	clc				; 暗く: 上位 + d。桁上がりが無ければ上位 < -d で黒
	adc sh_d
	bcc @black
	asl a				; (上位 + d) << 4 | 下位
	asl a
	asl a
	asl a
	sta sh_r
	lda sh_c
	and #$0f
	ora sh_r
	cmp #$0d			; $0D は「黒より黒い」色なので黒に
	bne @ret
@black:
	lda #_pal_BLACK
	bne @ret			; (いつも)
@white:
	lda #_pal_WHITE
	bne @ret			; (いつも)
@gray:
	tya
	bmi @black			; 黒を暗く: 黒のまま
	dey				; 明るく: (d - 1) << 4
	tya
	asl a
	asl a
	asl a
	asl a
@ret:
	sta sh_r
	rts
