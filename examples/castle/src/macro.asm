;;; 先頭で宣言したいアセンブラのマクロなど

.global _mmc3_select_shadow
	
.macro load mem, v
	lda v
	sta mem
.endmacro

.macro loadw mem,v
	lda #.LOBYTE(v)
	sta mem+0
	lda #.HIBYTE(v)
	sta mem+1
.endmacro
	

;;; バンク切り替えと割り込みの規則 (mmc3.fc の select_shadow):
;;;   MMC3 の $8000 (BANK_SELECT) は「次の $8001 の書き先」を持つ共有の状態で、$8000 と $8001 の書き込みの間に
;;;   割り込み (NMI は sei で止まらない) が入って $8000 を書き換えると、$8001 が別のスロットに入る。
;;;   - $8000 を書く側は、その値を先に _mmc3_select_shadow に書く (mmc3_cbank / mmc3_pbank、set_cbank / set_pbank、farcall)
;;;   - NMI (_interrupt) は入口で _mmc3_select_shadow を退避し、出口で戻してから $8000 にも書く (ppu.asm)。
;;;     なので NMI の中では普通の mmc3_cbank / mmc3_pbank (シャドウを書く) を使ってよい
;;;   - IRQ ハンドラは mmc3_cbank_irq / mmc3_pbank_irq (シャドウを書かない) で切り替え、rts の前に mmc3_irq_end で
;;;     $8000 をシャドウに戻す (入口で退避しないのは、h-blank に合わせたサイクル数を変えないため)。
;;;     前提: IRQ ハンドラの途中で NMI が入らないこと (NMI は I フラグで止まらないが、IRQ は画面途中のラインでだけ
;;;     発生し VBlank (241 ライン目) にかからない)。IRQ のラインを画面の下端近くにするならこの前提が崩れるので、
;;;     IRQ も NMI と同じく入口で退避する形にする

;;; mmc3_cbank select
;;;   select: 0-5
;;;   x     : 2KB bank number
;;; use 14cycle (シャドウの書き込み込み)
.macro mmc3_cbank bank
	lda #bank
	sta _mmc3_select_shadow
	sta _mmc3_BANK_SELECT
	stx _mmc3_BANK_DATA
.endmacro

;;; mmc3_pbank select
;;;   select: 0-1
;;;   x     : 8KB bank number
;;; use 14cycle (シャドウの書き込み込み)
.macro mmc3_pbank bank
	lda #(bank+6)
	sta _mmc3_select_shadow
	sta _mmc3_BANK_SELECT
	stx _mmc3_BANK_DATA
.endmacro

;;; IRQ ハンドラ用 (シャドウを書かない。出口で mmc3_irq_end)
;;; use 10cycle
.macro mmc3_cbank_irq bank
	lda #bank
	sta _mmc3_BANK_SELECT
	stx _mmc3_BANK_DATA
.endmacro

.macro mmc3_pbank_irq bank
	lda #(bank+6)
	sta _mmc3_BANK_SELECT
	stx _mmc3_BANK_DATA
.endmacro

;;; IRQ ハンドラの出口: $8000 を割り込む前の値 (シャドウ) に戻す
;;; use 8cycle
.macro mmc3_irq_end
	lda _mmc3_select_shadow
	sta _mmc3_BANK_SELECT
.endmacro
	
;;; wait N
;;; use N*5+1 cycle
.macro xwait n
	ldx n
:	dex
	bne :-
.endmacro

;;; wait N
;;; use N*5+1 cycle
.macro ywait n
	ldy n
:	dey
	bne :-
.endmacro

;;; irq_set counter
;;; set IRQ for MMC3.
;;; use 18cycle( if \1 is immediate, case zeropage 19cycle, case absolute 20cycle)
.macro irq_set n
	lda n
	sta _mmc3_IRQ_LATCH
	sta _mmc3_IRQ_RELOAD
	sta _mmc3_IRQ_DISABLE
	sta _mmc3_IRQ_ENABLE
.endmacro

