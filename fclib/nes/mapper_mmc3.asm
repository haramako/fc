;;; MMC3 (fclib/nes/mmc3.fc) の asm: far call のトランポリンと IRQ の入口。
;;; farcall_ay は fclib/nes/farcall_mmc3.asm と同じだが、BANK_SELECT に書く前に _mmc3_select (BANK_SELECT の写し) も書く
;;; (NMI / IRQ の中でバンクを切り替える asm は、終わりに BANK_SELECT へ _mmc3_select を書き戻す決まり。主の側が BANK_SELECT と
;;; BANK_DATA の間で割り込まれても壊れないように)。
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (ld65.cfg の bank = N)。
;;;       A / Y = 呼び先の引数 (そのまま呼び先へ渡す)、X は自由 (呼び先へは X = FC_SP で入る: stack 系の呼び先の引数の底)。
;;; 出力: A = 呼び先の戻り値 (そのまま返す)。X / Y は壊す。FC_FASTCALL_REG は触らない (Agent/wiki/design/farcall.md §3.7)。
;;; スロットは呼び先アドレスの上位バイトで決める ($80-$9F → slot 0、$A0-$BF → slot 1)。$C000 以上は固定なので切り替えない。
;;; そのスロットに既に同じバンクが入っていれば切り替えずに飛ぶ (X だけを使う。slot 0 で +26 サイクル)。違えば A の引数・
;;; 今のバンク・スロットをハードウェアスタックに退避して切り替え、戻ってから復帰する。入れ子も可。
;;; 注意: .segment は書かない (mmc3 モジュールの固定のバンクに置かれる)。

	.import FC_FARCALL
	.importzp FC_SP
	.global farcall_ay
	.export _interrupt_irq

farcall_ay:
	ldx FC_FARCALL+1
	cpx #$A0
	bcs @hi
	ldx _mmc3_prg_banks+0
	cpx FC_FARCALL+2
	bne @switch0            ; slot 0 ($8000-$9FFF) に別のバンク
@go:
	ldx FC_SP
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ (呼び先の rts が呼び出し元へ戻る)
@hi:
	cpx #$C0
	bcs @go                 ; $C000 以上は固定バンク: 切り替えずに飛ぶ
	ldx _mmc3_prg_banks+1
	cpx FC_FARCALL+2
	beq @go
							; slot 1 ($A000-$BFFF) に別のバンク: X = 今のバンク
	pha                     ; A の引数
	txa
	pha                     ; 今のバンク
	lda FC_FARCALL+2
	sta _mmc3_prg_banks+1
	ldx #7                  ; BANK_SELECT = 7 (PRG $A000)
	sei
	stx _mmc3_select
	stx $8000
	sta $8001
	cli
	tsx
	lda $0102,x             ; A の引数 (Y はそのまま)
	ldx FC_SP
	jsr @indirect
	tay                     ; 戻り値
	pla                     ; 元のバンク
	sta _mmc3_prg_banks+1
	ldx #7
	sei
	stx _mmc3_select
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
	sta _mmc3_prg_banks+0
	ldx #6                  ; BANK_SELECT = 6 (PRG $8000)
	sei
	stx _mmc3_select
	stx $8000
	sta $8001
	cli
	tsx
	lda $0102,x             ; A の引数 (Y はそのまま)
	ldx FC_SP
	jsr @indirect
	tay                     ; 戻り値
	pla                     ; 元のバンク
	sta _mmc3_prg_banks+0
	ldx #6
	sei
	stx _mmc3_select
	stx $8000
	sta $8001
	cli
	pla                     ; A の引数を捨てる
	tya
	rts
@indirect:
	jmp (FC_FARCALL)

;;; IRQ (走査線のカウンタ): 受け取って止め ($E000)、呼び出し口があれば呼ぶ (呼び出し口の rts が runtime.asm の
;;; interrupt_irq へ戻る。続けて IRQ が要るなら呼び出し口の中で $C000 / $C001 / $E001 を書く)
_interrupt_irq:
	sta $E000
	lda _mmc3_irq_hook+1
	beq @end
	pha
	lda _mmc3_irq_hook
	pha
	php
	rti
@end:
	rts
