	.exportzp FC_LOCAL
	.exportzp FC_REG
	.exportzp FC_STACK
	.exportzp FC_FASTCALL_REG
	.exportzp L 					; TODO: そのうち消すこと
	.exportzp reg
	.exportzp S
	.export FC_FARCALL
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
    .byte   1   	; ines prg ram - PRG RAM size in 8KB units ($6000-$7FFF)
    .byte   0,0,0,0,0,0,0 ; 7 zeroes

;; fc の cc65 規約 (options(abi: "cc65")) の extern 関数が引数を A/X に置く前に使う領域と、sound.asm の NSD グルーの退避 (+0〜1、+14〜15)。fc は各モジュールで .assert する
FC_FASTCALL_REG_SIZE = $10
	.export FC_FASTCALL_REG_SIZE : absolute

;; fc の静的フレーム (fc の doc/v2_frame_alloc.md §6)。mmc3.fc の options(static_zp: 64, static_ram: 256) と一致させる。
;; スタックは再帰関数と asm 定義の関数だけが使うので半分 ($40) にして、残りを静的フレームに充てる
FC_SZP_SIZE = $40
FC_SRAM_SIZE = $100						; RAM 側は WRAM (BSS_EX) に置く。実際の必要量は数十バイト
	.export FC_SZP_SIZE : absolute
	.export FC_SRAM_SIZE : absolute
	.exportzp FC_SZP
	.exportzp FC_SP
	.export FC_SRAM

.segment "FC_ZEROPAGE": zeropage

FC_LOCAL: .res $10
FC_REG: .res $10
FC_FASTCALL_REG: .res FC_FASTCALL_REG_SIZE

.segment "BSS_EX"
FC_FARCALL: .res 3						; far call の呼び先アドレスとバンク (fc が使う。ZP でなくてよい)
FC_SRAM: .res FC_SRAM_SIZE				; 静的フレーム (RAM 側。ZP に入りきらない関数のフレーム)

.segment "FC_STACK": zeropage
	
FC_STACK: .res $3F
FC_SP: .res 1							; スタックの空き先頭 (S からのオフセット。X の代わり)
FC_SZP: .res FC_SZP_SIZE				; 静的フレーム (ゼロページ側)

	L = FC_LOCAL
	reg = FC_REG
	S = FC_STACK
	
.segment "VECTORS"
	.word interrupt
	.word start
	.word interrupt_irq


	
.segment "fs_data"
	.incbin "../res/fs_data.bin"

