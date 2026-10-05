;;; farcall_ay (UxROM 用の参考実装。Agent/wiki/design/farcall.md §3.4・§3.7、fc.toml の [target] mapper = "UxROM")
;;;
;;; プロジェクトの固定の領域のモジュールから @include("farcall_uxrom.asm") する (fclib/nes/uxrom.fc が include している)。
;;; 前提: プロジェクトが今のバンクを持つ変数 `var uxrom_bank:u8 @(symbol: "_uxrom_bank");` を用意し、起動時に
;;;       @bank(...) で選んだバンクを書いてそれに合わせる。
;;;
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (16KB 単位。ld65.cfg の bank = N)。
;;;       A / Y = 呼び先の引数 (そのまま呼び先へ渡す)、X は自由 (呼び先へは X = FC_SP で入る: stack 系の呼び先の引数の底)。
;;; 出力: A = 呼び先の戻り値 (そのまま返す)。X / Y は壊す。FC_FASTCALL_REG は触らない。
;;; 切り替えのスロットは $8000 の 1 つだけ ($C000 以上は最後のバンクに固定)。既に同じバンクなら切り替えずに飛ぶ (X だけを使う)。
;;; 違えば今のバンクと A をハードウェアスタックに退避して切り替え、戻ってから復帰する。入れ子も可。
;;; UxROM は書き込みが ROM の出力とぶつかる (バスの衝突) ので、書く値と同じ値の入った表 (uxrom_banks) に書く。

	.import FC_FARCALL
	.importzp FC_SP
	.global _uxrom_bank
	.global farcall_ay

;;; 注意: .segment は書かない。include したモジュールのセグメント (固定の領域に置くこと) に置かれる。

farcall_ay:
	ldx FC_FARCALL+1
	cpx #$C0
	bcs @go                 ; $C000 以上は固定: 切り替えずに飛ぶ
	ldx _uxrom_bank
	cpx FC_FARCALL+2
	bne @switch
@go:
	ldx FC_SP
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ (呼び先の rts が呼び出し元へ戻る)
@switch:                    ; X = 今のバンク
	pha                     ; A の引数
	txa
	pha                     ; 今のバンク
	lda FC_FARCALL+2
	sta _uxrom_bank
	tax
	sta uxrom_banks,x       ; バスの衝突を避けて同じ値の番地に書く
	tsx
	lda $0102,x             ; A の引数 (Y はそのまま)
	ldx FC_SP
	jsr @indirect
	tay                     ; 戻り値
	pla                     ; 元のバンク
	sta _uxrom_bank
	tax
	sta uxrom_banks,x
	pla                     ; A の引数を捨てる
	tya
	rts
@indirect:
	jmp (FC_FARCALL)

uxrom_banks:
	.byte 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15
