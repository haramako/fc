;;; IRQの設定
	.include "./macro.asm"
	.import scroll

.segment "irqcmd_bss"
	irq_work_head: .res 1

.segment "irqcmd_irq"
	.define irq_work _irqcmd_buf

;;; コマンド
;;;
;;; 1: (次の割り込み)
	
.proc _irqcmd_irq_setup
	lda _irqcmd_buf_start ; irq_work_head = buf_start
	sta irq_work_head

	tax

	lda irq_work+0,x ; ppu.irq_next = buf[irq_work_head] as fn()
	sta _ppu_irq_next+0 
	lda irq_work+1,x
	sta _ppu_irq_next+1 

	inc irq_work_head ; irq_work_head += 2
	inc irq_work_head

	jmp (_ppu_irq_next) ; ppu.irq_next()
.endproc

;;; IRQコマンドの共通セットアップ
;;; 変更
;;; x: コマンドの現在の位置(irq_work_headが読み込まれる)
.proc irq_command_begin
	ldx irq_work_head

	lda irq_work+0,x
	beq @else
	sta _mmc3_IRQ_LATCH
	sta _mmc3_IRQ_RELOAD
	sta _mmc3_IRQ_DISABLE
	sta _mmc3_IRQ_ENABLE
	jmp @end
@else:
	sta _mmc3_IRQ_DISABLE		; 4c
	ywait #2
	nop
@end:
	rts
.endproc

;;; IRQコマンドの共通終了処理
;;; x: コマンドの現在の位置
.proc irq_command_end
	;; _ppu_irq_next = *(_ppu_irq_next+3)
	lda irq_work+0,x
	sta _ppu_irq_next+0 
	lda irq_work+1,x
	sta _ppu_irq_next+1 

	;; irq_work_head += 2
	inx
	inx
	stx irq_work_head
	rts
.endproc

;;; IRQコマンド(スクロール）
;;; 0: IRQの走査線数-1
;;; ここに入ってくるまでに,24cycle使っている。１回めの h-blank まで 113 - 24 で 89cycle.
.proc irq_command_wait_line
	jsr irq_command_begin

	inx
	jsr irq_command_end

	rts
.endproc

.proc _irqcmd_cmd_begin_screen
	jsr irq_command_begin

	lda irq_work+1,x
	sta _nes_PPU_SCROLL
	lda irq_work+2,x
	sta _nes_PPU_SCROLL
	lda irq_work+3,x
	sta _nes_PPU_CTRL1

	lda #0
	sta _mmc3_BANK_SELECT
	lda irq_work+4,x
	sta _mmc3_BANK_DATA
	
	lda #1
	sta _mmc3_BANK_SELECT
	lda irq_work+5,x
	sta _mmc3_BANK_DATA

	txa
	clc
	adc #6
	tax

	jsr irq_command_end
	rts
.endproc

.segment "irqcmd_irq2"

.proc irq_command_cbank_bg
	jsr irq_command_begin

	ywait #9

	lda #0
	sta _mmc3_BANK_SELECT
	lda irq_work+1,x
	sta _mmc3_BANK_DATA
	
	lda #1
	sta _mmc3_BANK_SELECT
	lda irq_work+2,x
	sta _mmc3_BANK_DATA

	inx
	inx
	inx

	jsr irq_command_end

	rts
.endproc

;;; IRQコマンド(スクロール）
;;; 0-1: 
;;; 2: IRQの走査線数-1
;;; ここに入ってくるまでに,24cycle使っている。１回めの h-blank まで 113 - 24 で 89cycle.
.proc irq_command_scroll
	jsr irq_command_begin

	lda irq_work+1,x
	sta _nes_PPU_ADDR
	lda irq_work+2,x
	sta _nes_PPU_SCROLL
	lda irq_work+3,x
	sta _nes_PPU_SCROLL
	lda irq_work+4,x
	sta _nes_PPU_ADDR

	lda irq_work+5,x
	sta _nes_PPU_CTRL1

	txa
	clc
	adc #6
	tax

	jsr irq_command_end

	rts
.endproc

;;; IRQコマンド(画面全体の切り替え）
;;; 0-1: 
;;; 2: IRQの走査線数-1
;;; ここに入ってくるまでに,24cycle使っている。１回めの h-blank まで 113 - 24 で 89cycle.
.proc irq_command_screen
	jsr irq_command_begin

	ywait #2
	ldy irq_work+4,x

	lda irq_work+1,x
	sta _nes_PPU_ADDR
	lda irq_work+2,x
	sta _nes_PPU_SCROLL

	;lda #%00010000				; nestopia(実機と同等)と他のエミュ(VirtuaNES, FCEUX)の挙動を合わせるためスプライトを消さない
	;ta _nes_PPU_CTRL2			; 2+4c = 6c

	lda irq_work+3,x
	sta _nes_PPU_SCROLL
	sty _nes_PPU_ADDR

	lda irq_work+5,x
	sta _nes_PPU_CTRL1

	lda #0
	sta _mmc3_BANK_SELECT
	lda irq_work+7,x
	sta _mmc3_BANK_DATA
	
	lda #1
	sta _mmc3_BANK_SELECT
	lda irq_work+8,x
	sta _mmc3_BANK_DATA

	ywait #3

	lda irq_work+6,x
	sta _nes_PPU_CTRL2			; 4c

	txa
	clc
	adc #9
	tax

	jsr irq_command_end

	rts
.endproc

.segment "irqcmd"

;;; scanline中のスクロールレジスタの計算を行う
;;; FC_FASTCALL_REG
;;; 0: X座標 
;;; 1: Y座標
;;; 2: ネームテーブル(00~03)
;;; 
;;; yyy NN ZZYYY XXXXX
;;; ||| || ||||| +++++-- coarse X scroll
;;; ||| || +++++-------- coarse Y scroll
;;; ||| ++-------------- nametable select
;;; +++----------------- fine Y scroll
;;; 
;;; 書き込み順
;;; PPU_ADDR(2006)   ----NN--
;;; PPU_SCROLL(2005) ZZ---yyy    (ZZYYYyyyのYYYが上書きされる)
;;; PPU_SCROLL(2005) -----xxx    (XXXXXxxxのXXXXXが上書きされる)
;;; PPU_ADDR(2006)   YYYXXXXX
;;; 
;;; 結果
;;; t = yyyNNZZ YYYXXXXX
;;; x = xxx
;;; 
;;; See: https://www.nesdev.org/wiki/PPU_scrolling
; (make_scroll_reg is written in fc now: irqcmd.fc)
