;;; MMC3 (fclib/nes/mmc3.fc) の asm: far call のトランポリンと IRQ の入口。
;;; farcall は fclib/nes/farcall_mmc3.asm と同じだが、BANK_SELECT に書く前に _mmc3_select (BANK_SELECT の写し) も書く
;;; (NMI / IRQ の中でバンクを切り替える asm は、終わりに BANK_SELECT へ _mmc3_select を書き戻す決まり。主の側が BANK_SELECT と
;;; BANK_DATA の間で割り込まれても壊れないように)。
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号。X と FC_FASTCALL_REG を壊さない。A / Y は自由。
;;; スロットは呼び先の上位バイトで決める ($80-$9F → 0、$A0-$BF → 1)。$C000 以上は固定なので切り替えない。
;;; 注意: .segment は書かない (mmc3 モジュールの固定のバンクに置かれる)。

	.import FC_FARCALL
	.global farcall
	.export _interrupt_irq

farcall:
	lda FC_FARCALL+1
	cmp #$C0
	bcs @fixed
	cmp #$A0
	bcs @slot1
	ldy #0
	.byte $2C			; bit abs (次の ldy #1 を飛ばす)
@slot1:
	ldy #1
	lda _mmc3_prg_banks,y
	cmp FC_FARCALL+2
	bne @switch
@fixed:
	jmp (FC_FARCALL)
@switch:
	pha				; 今のバンク
	tya
	pha				; スロット
	lda FC_FARCALL+2
	sta _mmc3_prg_banks,y
	jsr @select
	lda FC_FARCALL+2
	sta $8001
	cli
	jsr @indirect
	pla				; スロット
	tay
	pla				; 元のバンク
	sta _mmc3_prg_banks,y
	pha
	jsr @select
	pla
	sta $8001
	cli
	rts
@select:				; BANK_SELECT = スロット + 6 (sei してから。写しも)
	sei
	tya
	clc
	adc #6
	sta _mmc3_select
	sta $8000
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
