	.export runtime_init
	.include "_nes.s"

.segment "CODE"
	
runtime_init:

	;; initialize hardwares
	;; See: http://wiki.nesdev.com/w/index.php/Init_code
	ldx #$40
	stx $4017
	ldx #0
	stx $2000
	stx $2001
	stx $4010
	
	;; wait for PPU worm-up
	;; See: http://wiki.nesdev.com/w/index.php/PPU_power_up_state#Best_practice
	ldx #4
@loop:
	bit _nes_PPU_STAT
	bpl @loop
	dex
	bne @loop
	rts
