;;; farcall (MMC1 用の参考実装。doc/v3_plan.md §3、fc.toml の [target] mapper = "MMC1")
;;;
;;; プロジェクトの固定の領域のモジュールから @include("farcall_mmc1.asm") する。
;;; 前提: MMC1 の PRG はモード 3 ($8000 の 16KB を切り替え、$C000 は最後のバンクに固定。電源投入時の既定。起動時に
;;;       `jsr mmc1_reset` か $8000 へ $80 を書いて確かめる)。プロジェクトが今のバンクを持つ変数
;;;       `var mmc1_bank:u8 @(symbol: "_mmc1_bank");` を用意し、起動時に mmc1_set で選んだバンクに合わせる。
;;;
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (16KB 単位。ld65.cfg の bank = N)
;;; 規約: X (フレームポインタ) と FC_FASTCALL_REG を壊さない。A / Y は自由。
;;; PRG のレジスタ ($E000) へは 1 ビットずつ 5 回書く。途中で IRQ が入ると壊れるので sei / cli で囲む
;;; (NMI の処理ではマッパーを触らないこと)。

	.import FC_FARCALL
	.global _mmc1_bank
	.global farcall
	.global mmc1_set
	.global mmc1_reset

;;; 注意: .segment は書かない。include したモジュールのセグメント (固定の領域に置くこと) に置かれる。

farcall:
	lda FC_FARCALL+1
	cmp #$C0
	bcs @fixed              ; $C000 以上は固定: 切り替えずに飛ぶ
	lda _mmc1_bank
	cmp FC_FARCALL+2
	bne @switch
@fixed:
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ
@switch:
	pha                     ; 今のバンクを退避
	lda FC_FARCALL+2
	jsr mmc1_set
	jsr @indirect
	pla                     ; 元のバンク
	jmp mmc1_set            ; (mmc1_set の rts で呼び出し元へ)
@indirect:
	jmp (FC_FARCALL)

;;; mmc1_set: A のバンクを $8000 に入れ、_mmc1_bank を合わせる (A / Y を壊す。X は壊さない)
mmc1_set:
	sta _mmc1_bank
	sei
	ldy #5
@bit:
	sta $E000
	lsr a
	dey
	bne @bit
	cli
	rts

;;; mmc1_reset: シリアルの受け口を空にして PRG をモード 3 にする
mmc1_reset:
	lda #$80
	sta $8000
	rts
