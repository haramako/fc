	.setcpu "6502"
	.include "macro.inc"
__MODULE_NES__ = 1
.segment "nes"
_nes_PPU_CTRL1 = 8192
_nes_PPU_CTRL2 = 8193
_nes_PPU_STAT = 8194
_nes_PPU_SPR_ADDR = 8195
_nes_PPU_SPR_DATA = 8196
_nes_PPU_SCROLL = 8197
_nes_PPU_ADDR = 8198
_nes_PPU_DATA = 8199
_nes_SPRITE_DMA = 16404
_nes_PAD_CTRL = 16406
_nes_PAD_DATA = 16407
_nes_APU_DMC = 16400
_nes_APU_INTERRUPT = 16407
