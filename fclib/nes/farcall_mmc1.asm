;;; farcall_ay (MMC1 用の参考実装。Agent/wiki/design/farcall.md §3.4・§3.7、fc.toml の [target] mapper = "MMC1")
;;;
;;; プロジェクトの固定の領域のモジュールから @include("farcall_mmc1.asm") する (fclib/nes/mmc1.fc が include している)。
;;; 前提: MMC1 の PRG はモード 3 ($8000 の 16KB を切り替え、$C000 は最後のバンクに固定。電源投入時の既定。起動時に
;;;       `jsr mmc1_reset` か $8000 へ $80 を書いて確かめる)。プロジェクトが今のバンクを持つ変数
;;;       `var mmc1_bank:u8 @(symbol: "_mmc1_bank");` を用意し、起動時に mmc1_set で選んだバンクに合わせる。
;;;
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (16KB 単位。ld65.cfg の bank = N)。
;;;       A / Y = 呼び先の引数 (そのまま呼び先へ渡す)、X は自由 (呼び先へは X = FC_SP で入る: stack 系の呼び先の引数の底)。
;;; 出力: A = 呼び先の戻り値 (そのまま返す)。X / Y は壊す。FC_FASTCALL_REG は触らない (Agent/wiki/design/farcall.md §3.7)。
;;; 既に同じバンクなら切り替えずに飛ぶ (X だけを使う)。違えば A / Y の引数と今のバンクをハードウェアスタックに退避して切り替え、
;;; 戻ってから復帰する。入れ子も可。
;;; PRG のレジスタ ($E000) へは 1 ビットずつ 5 回書く。途中で IRQ が入ると壊れるので sei / cli で囲む
;;; (NMI の処理ではマッパーを触らないこと)。

	.import FC_FARCALL
	.importzp FC_SP
	.global _mmc1_bank
	.global farcall_ay
	.global mmc1_set
	.global mmc1_reset

;;; 注意: .segment は書かない。include したモジュールのセグメント (固定の領域に置くこと) に置かれる。

farcall_ay:
	ldx FC_FARCALL+1
	cpx #$C0
	bcs @go                 ; $C000 以上は固定: 切り替えずに飛ぶ
	ldx _mmc1_bank
	cpx FC_FARCALL+2
	bne @switch
@go:
	ldx FC_SP
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ
@switch:                    ; X = 今のバンク
	pha                     ; A の引数
	tya
	pha                     ; Y の引数
	txa
	pha                     ; 今のバンク
	lda FC_FARCALL+2
	jsr mmc1_set            ; (A / Y を壊す)
	tsx
	lda $0102,x             ; Y の引数
	tay
	lda $0103,x             ; A の引数
	ldx FC_SP
	jsr @indirect
	tax                     ; 戻り値 (mmc1_set は X を壊さない)
	pla                     ; 元のバンク
	jsr mmc1_set
	pla                     ; Y と A の引数を捨てる
	pla
	txa
	rts
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
