	.exportzp FC_LOCAL
	.exportzp FC_REG
	.exportzp FC_STACK
	.exportzp FC_FASTCALL_REG
	.export FC_FARCALL
	.exportzp L 					; TODO: そのうち消すこと
	.exportzp reg
	.exportzp S
	.import interrupt
	.import start
	.import interrupt_irq
	
.segment "HEADER"

;    +--------+------+------------------------------------------+
;    | Offset | Size | Content(s)                               |
;    +--------+------+------------------------------------------+
;    |   0    |  3   | 'NES'                                    |
;    |   3    |  1   | $1A                                      |
;    |   4    |  1   | 16K PRG-ROM page count                   |
;    |   5    |  1   | 8K CHR-ROM page count                    |
;    |   6    |  1   | ROM Control Byte #1                      |
;    |        |      |   %####vTsM                              |
;    |        |      |    |  ||||+- 0=Horizontal mirroring      |
;    |        |      |    |  ||||   1=Vertical mirroring        |
;    |        |      |    |  |||+-- 1=SRAM enabled              |
;    |        |      |    |  ||+--- 1=512-byte trainer present  |
;    |        |      |    |  |+---- 1=Four-screen mirroring     |
;    |        |      |    |  |                                  |
;    |        |      |    +--+----- Mapper # (lower 4-bits)     |
;    |   7    |  1   | ROM Control Byte #2                      |
;    |        |      |   %####0000                              |
;    |        |      |    |  |                                  |
;    |        |      |    +--+----- Mapper # (upper 4-bits)     |
;    |  8-15  |  8   | $00                                      |
;    | 16-..  |      | Actual 16K PRG-ROM pages (in linear      |
;    |  ...   |      | order). If a trainer exists, it precedes |
;    |  ...   |      | the first PRG-ROM page.                  |
;    | ..-EOF |      | CHR-ROM pages (in ascending order).      |
;    +--------+------+------------------------------------------+

    .byte   $4e,$45,$53,$1a ; "NES"^Z
    .byte   16      ; ines prg  - Specifies the number of 16k prg banks.
    .byte   16		; ines chr  - Specifies the number of 8k chr banks.
	;; MMC3(4), battery backuped RAM
    .byte   %01000011 ; ines mir  - Specifies VRAM mirroring of the banks.
    .byte   0   	; ines map  - Specifies the NES mapper used.
    .byte   0,0,0,0,0,0,0,0 ; 8 zeroes

;; fc の fastcall 関数が使う領域の大きさ。fc は各モジュールで .assert して不足をリンク時に検出する。
;; ZP に空きが無いので 16 のまま (main.fc の options(fastcall_reg: 16) と一致させる)
FC_FASTCALL_REG_SIZE = $10
	.export FC_FASTCALL_REG_SIZE : absolute

.segment "FC_ZEROPAGE": zeropage
	
FC_LOCAL: .res $10
FC_REG: .res $10
FC_FASTCALL_REG: .res FC_FASTCALL_REG_SIZE

.segment "BSS"
FC_FARCALL: .res 3						; far call の呼び先アドレスとバンク (fc が使う。ZP でなくてよい)

.segment "FC_STACK": zeropage
	
FC_STACK: .res $80

	L = FC_LOCAL
	reg = FC_REG
	S = FC_STACK
	
.segment "VECTORS"
	.word interrupt
	.word start
	.word interrupt_irq


	
.segment "fs_data"
	.incbin "../res/fs_data.bin"

