;;; farcall_ay (MMC3 用の参考実装。Agent/wiki/design/farcall.md §3.4・§3.7)
;;;
;;; プロジェクトにコピーして、mmc3 モジュールから include("farcall_mmc3.asm") する。
;;; 前提: fc の mmc3 モジュールが pbank_bak:[2]int (各スロットの今のバンク) を持ち、
;;;       BANK_SELECT ($8000) / BANK_DATA ($8001) で切り替える (castle の src/mmc3.fc と同じ)。
;;;
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (ld65.cfg の bank = N)。
;;;       A / Y = 呼び先の引数 (そのまま呼び先へ渡す)、X は自由 (呼び先へは X = FC_SP で入る: stack 系の呼び先の引数の底)。
;;; 出力: A = 呼び先の戻り値 (そのまま返す)。X / Y は壊す。FC_FASTCALL_REG は触らない (Agent/wiki/design/farcall.md §3.7)。
;;; スロットは呼び先アドレスの上位バイトで決める ($80-$9F → slot 0、$A0-$BF → slot 1)。$C000 以上は固定なので切り替えない。
;;; そのスロットに既に同じバンクが入っていれば切り替えずに飛ぶ (X だけを使う。slot 0 で +26 サイクル)。違えば A の引数・
;;; 今のバンク・スロットをハードウェアスタックに退避して切り替え、戻ってから復帰する。入れ子も可。

	.import FC_FARCALL
	.global _mmc3_pbank_bak         ; mmc3 モジュールから include するので .import でなく .global (定義側なら export になる)
	.global _mmc3_select_shadow     ; $8000 に書く値のシャドウ (macro.asm の規則。割り込みが出口で $8000 を戻すのに使う)
	.importzp FC_SP
	.global farcall_ay

farcall_ay:
	ldx FC_FARCALL+1
	cpx #$A0
	bcs @hi
	ldx _mmc3_pbank_bak+0
	cpx FC_FARCALL+2
	bne @switch0            ; slot 0 ($8000-$9FFF) に別のバンク
@go:
	ldx FC_SP
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ (呼び先の rts が呼び出し元へ戻る)
@hi:
	cpx #$C0
	bcs @go                 ; $C000 以上は固定バンク: 切り替えずに飛ぶ
	ldx _mmc3_pbank_bak+1
	cpx FC_FARCALL+2
	beq @go
							; slot 1 ($A000-$BFFF) に別のバンク: X = 今のバンク
	pha                     ; A の引数
	txa
	pha                     ; 今のバンク
	lda FC_FARCALL+2
	sta _mmc3_pbank_bak+1
	ldx #7                  ; BANK_SELECT = 7 (PRG $A000)
	sei
	stx _mmc3_select_shadow ; ハードより先にシャドウ
	stx $8000
	sta $8001
	cli
	tsx
	lda $0102,x             ; A の引数 (Y はそのまま)
	ldx FC_SP
	jsr @indirect
	tay                     ; 戻り値
	pla                     ; 元のバンク
	sta _mmc3_pbank_bak+1
	ldx #7
	sei
	stx _mmc3_select_shadow ; ハードより先にシャドウ
	stx $8000
	sta $8001
	cli
	pla                     ; A の引数を捨てる
	tya
	rts
@switch0:                   ; X = 今のバンク
	pha                     ; A の引数
	txa
	pha                     ; 今のバンク
	lda FC_FARCALL+2
	sta _mmc3_pbank_bak+0
	ldx #6                  ; BANK_SELECT = 6 (PRG $8000)
	sei
	stx _mmc3_select_shadow ; ハードより先にシャドウ
	stx $8000
	sta $8001
	cli
	tsx
	lda $0102,x             ; A の引数 (Y はそのまま)
	ldx FC_SP
	jsr @indirect
	tay                     ; 戻り値
	pla                     ; 元のバンク
	sta _mmc3_pbank_bak+0
	ldx #6
	sei
	stx _mmc3_select_shadow ; ハードより先にシャドウ
	stx $8000
	sta $8001
	cli
	pla                     ; A の引数を捨てる
	tya
	rts
@indirect:
	jmp (FC_FARCALL)
