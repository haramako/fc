;;; farcall (UxROM 用の参考実装。doc/v3_plan.md §3、fc.toml の [target] mapper = "UxROM")
;;;
;;; プロジェクトの固定の領域のモジュールから @include("farcall_uxrom.asm") する。
;;; 前提: プロジェクトが今のバンクを持つ変数 `var uxrom_bank:u8 @(symbol: "_uxrom_bank");` を用意し、起動時に
;;;       @bank(...) で選んだバンクを書いてそれに合わせる。
;;;
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (16KB 単位。ld65.cfg の bank = N)
;;; 規約: X (フレームポインタ) と FC_FASTCALL_REG を壊さない。A / Y は自由。
;;; 切り替えのスロットは $8000 の 1 つだけ ($C000 以上は最後のバンクに固定)。既に同じバンクなら切り替えずに飛ぶ。
;;; 違えば今のバンクをハードウェアスタックに退避して切り替え、戻ってから復帰する。入れ子も可。
;;; UxROM は書き込みが ROM の出力とぶつかる (バスの衝突) ので、書く値と同じ値の入った表 (uxrom_banks) に書く。

	.import FC_FARCALL
	.global _uxrom_bank
	.global farcall

;;; 注意: .segment は書かない。include したモジュールのセグメント (固定の領域に置くこと) に置かれる。

farcall:
	lda FC_FARCALL+1
	cmp #$C0
	bcs @fixed              ; $C000 以上は固定: 切り替えずに飛ぶ
	lda _uxrom_bank
	cmp FC_FARCALL+2
	bne @switch
@fixed:
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ (呼び先の rts が呼び出し元へ戻る)
@switch:
	pha                     ; 今のバンクを退避
	ldy FC_FARCALL+2
	sty _uxrom_bank
	tya
	sta uxrom_banks,y       ; バスの衝突を避けて同じ値の番地に書く
	jsr @indirect
	pla                     ; 元のバンク
	sta _uxrom_bank
	tay
	sta uxrom_banks,y
	rts
@indirect:
	jmp (FC_FARCALL)

uxrom_banks:
	.byte 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15
