
	.export scroll
	
.segment "main"
	
;;; scanline中のスクロールを行う
;;; x: X座標 y: Y座標
;;; See: http://forums.nesdev.com/viewtopic.php?p=64111#p64111
;;; use 56cycle ( jsr含む、最後の'sta _PPU_ADDR'まで )
scroll:
	lda #0
	clc
	rol a
	asl a
	asl a
	sta _nes_PPU_ADDR
	tya
	sta _nes_PPU_SCROLL
	asl a
	asl a
	and #%11100000
	sta _common_scroll_x+0
	txa
	lsr a
	lsr a
	lsr a
	ora _common_scroll_x+0

	;finish setting the scroll during HBlank (11 cycles)
	stx _nes_PPU_SCROLL
	sta _nes_PPU_ADDR
	rts
	
