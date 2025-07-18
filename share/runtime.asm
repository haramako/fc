
	.import _interrupt
	.import _interrupt_irq
	.import _main
	.import runtime_init

	.importzp L
	.importzp reg
	.importzp S

	.export start
	.export interrupt
	.export interrupt_irq
	.export jsr_reg
	.export __mul_8
	.export __mul_8s
	.export __mul_8t16
	.export __mul_16
	.export __div_8
	.export __div_8s
	.export __div_16
	.export __mod_8
	.export __mod_16
	
.segment "FC_RUNTIME"

.proc start
	sei
	cld

	jsr runtime_init

	;; 0x0000..0x07ff までのメモリをゼロクリアする
	ldx #0
	lda #0
@loop:
	sta $000,x
	sta $100,x
	sta $200,x					; スタックもまだ使ってないから掃除しちゃえ
	sta $300,x
	sta $400,x
	sta $500,x
	sta $600,x
	sta $700,x
	inx
	bne @loop
	
	ldx #255						; initialize stack and frame
	txs
	ldx #0

	jsr _main
    jmp *
.endproc

.proc interrupt
    pha
    txa
    pha
    tya
    pha
    jsr _interrupt
    pla
    tay
    pla
    tax
    pla
    rti
.endproc

;;; use 19 cycle (include jsr) before subroutine
.proc interrupt_irq
    pha
    txa
    pha
	tya
	pha
    jsr _interrupt_irq
	pla
	tay
    pla
    tax
    pla
    rti
.endproc
	
;;; 間接関数呼び出し
.proc jsr_reg
	jmp (<reg)
.endproc

;;; 掛け算用のテーブル
;;; LOW(x*x/4)  | x < 256
.proc __mul_tbl_l0
	.byte 0,0,1,2,4,6,9,12,16,20,25,30,36,42,49,56
	.byte 64,72,81,90,100,110,121,132,144,156,169,182,196,210,225,240
	.byte 0,16,33,50,68,86,105,124,144,164,185,206,228,250,17,40
	.byte 64,88,113,138,164,190,217,244,16,44,73,102,132,162,193,224
	.byte 0,32,65,98,132,166,201,236,16,52,89,126,164,202,241,24
	.byte 64,104,145,186,228,14,57,100,144,188,233,22,68,114,161,208
	.byte 0,48,97,146,196,246,41,92,144,196,249,46,100,154,209,8
	.byte 64,120,177,234,36,94,153,212,16,76,137,198,4,66,129,192
	.byte 0,64,129,194,4,70,137,204,16,84,153,222,36,106,177,248
	.byte 64,136,209,26,100,174,249,68,144,220,41,118,196,18,97,176
	.byte 0,80,161,242,68,150,233,60,144,228,57,142,228,58,145,232
	.byte 64,152,241,74,164,254,89,180,16,108,201,38,132,226,65,160
	.byte 0,96,193,34,132,230,73,172,16,116,217,62,164,10,113,216
	.byte 64,168,17,122,228,78,185,36,144,252,105,214,68,178,33,144
	.byte 0,112,225,82,196,54,169,28,144,4,121,238,100,218,81,200
	.byte 64,184,49,170,36,158,25,148,16,140,9,134,4,130,1,128
.endproc
	
;;; LOW(x*x/4)  | x >= 256
.proc __mul_tbl_l1
	.byte 0,128,1,130,4,134,9,140,16,148,25,158,36,170,49,184
	.byte 64,200,81,218,100,238,121,4,144,28,169,54,196,82,225,112
	.byte 0,144,33,178,68,214,105,252,144,36,185,78,228,122,17,168
	.byte 64,216,113,10,164,62,217,116,16,172,73,230,132,34,193,96
	.byte 0,160,65,226,132,38,201,108,16,180,89,254,164,74,241,152
	.byte 64,232,145,58,228,142,57,228,144,60,233,150,68,242,161,80
	.byte 0,176,97,18,196,118,41,220,144,68,249,174,100,26,209,136
	.byte 64,248,177,106,36,222,153,84,16,204,137,70,4,194,129,64
	.byte 0,192,129,66,4,198,137,76,16,212,153,94,36,234,177,120
	.byte 64,8,209,154,100,46,249,196,144,92,41,246,196,146,97,48
	.byte 0,208,161,114,68,22,233,188,144,100,57,14,228,186,145,104
	.byte 64,24,241,202,164,126,89,52,16,236,201,166,132,98,65,32
	.byte 0,224,193,162,132,102,73,44,16,244,217,190,164,138,113,88
	.byte 64,40,17,250,228,206,185,164,144,124,105,86,68,50,33,16
	.byte 0,240,225,210,196,182,169,156,144,132,121,110,100,90,81,72
	.byte 64,56,49,42,36,30,25,20,16,12,9,6,4,2,1,0
.endproc
	
;;; HIGH(x*x/4)  | x < 256
.proc __mul_tbl_h0
	.byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	.byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	.byte 1,1,1,1,1,1,1,1,1,1,1,1,1,1,2,2
	.byte 2,2,2,2,2,2,2,2,3,3,3,3,3,3,3,3
	.byte 4,4,4,4,4,4,4,4,5,5,5,5,5,5,5,6
	.byte 6,6,6,6,6,7,7,7,7,7,7,8,8,8,8,8
	.byte 9,9,9,9,9,9,10,10,10,10,10,11,11,11,11,12
	.byte 12,12,12,12,13,13,13,13,14,14,14,14,15,15,15,15
	.byte 16,16,16,16,17,17,17,17,18,18,18,18,19,19,19,19
	.byte 20,20,20,21,21,21,21,22,22,22,23,23,23,24,24,24
	.byte 25,25,25,25,26,26,26,27,27,27,28,28,28,29,29,29
	.byte 30,30,30,31,31,31,32,32,33,33,33,34,34,34,35,35
	.byte 36,36,36,37,37,37,38,38,39,39,39,40,40,41,41,41
	.byte 42,42,43,43,43,44,44,45,45,45,46,46,47,47,48,48
	.byte 49,49,49,50,50,51,51,52,52,53,53,53,54,54,55,55
	.byte 56,56,57,57,58,58,59,59,60,60,61,61,62,62,63,63
.endproc
	
;;; HIGH(x*x/4)  | x >= 256
.proc __mul_tbl_h1
	.byte 64,64,65,65,66,66,67,67,68,68,69,69,70,70,71,71
	.byte 72,72,73,73,74,74,75,76,76,77,77,78,78,79,79,80
	.byte 81,81,82,82,83,83,84,84,85,86,86,87,87,88,89,89
	.byte 90,90,91,92,92,93,93,94,95,95,96,96,97,98,98,99
	.byte 100,100,101,101,102,103,103,104,105,105,106,106,107,108,108,109
	.byte 110,110,111,112,112,113,114,114,115,116,116,117,118,118,119,120
	.byte 121,121,122,123,123,124,125,125,126,127,127,128,129,130,130,131
	.byte 132,132,133,134,135,135,136,137,138,138,139,140,141,141,142,143
	.byte 144,144,145,146,147,147,148,149,150,150,151,152,153,153,154,155
	.byte 156,157,157,158,159,160,160,161,162,163,164,164,165,166,167,168
	.byte 169,169,170,171,172,173,173,174,175,176,177,178,178,179,180,181
	.byte 182,183,183,184,185,186,187,188,189,189,190,191,192,193,194,195
	.byte 196,196,197,198,199,200,201,202,203,203,204,205,206,207,208,209
	.byte 210,211,212,212,213,214,215,216,217,218,219,220,221,222,223,224
	.byte 225,225,226,227,228,229,230,231,232,233,234,235,236,237,238,239
	.byte 240,241,242,243,244,245,246,247,248,249,250,251,252,253,254,255
.endproc
		
;;; uint8xuint8=>uint8の掛け算
;;; reg4 = reg0 * reg2
;;; 
;;; 以下の等式を利用する
;;; a*b = f(a+b) - f(a-b)  | f(x): x*x/4
;;; 
;;; USING: reg[0,2,4,6,7]
.proc __mul_8
        lda reg+0             ; reg6 = (reg0-reg2)^2/4
        sec
        sbc reg+2
        tay
        bcs @pl1
        lda __mul_tbl_l1,y
        jmp @pl1_end
@pl1:
        lda __mul_tbl_l0,y
@pl1_end:
        sta reg+6
        lda reg+0             ; a = (reg0+reg2)^2/4
        clc
        adc reg+2
        tay
        bcc @pl2
        lda __mul_tbl_l1,y
        jmp @pl2_end
@pl2:
        lda __mul_tbl_l0,y
@pl2_end:
        sta reg+7
        sec                     ; reg4 = a - reg6
        sbc reg+6
        sta reg+4
        rts
.endproc

__mul_8s = __mul_8


;;; uint8xuint8=>uint16の掛け算
;;; reg(4,6) = reg0 * reg2
;;; USING: reg[0,2,4,6,7]
;;; TODO: 中途半端な実装(ほぼ未実装)
.proc __mul_8t16
        lda reg+0             ; reg6 = (reg0-reg2)^2/4
        sec
        sbc reg+2
        tay
        bcs @pl1
        lda __mul_tbl_l1,y
        jmp @pl1_end
@pl1:
        lda __mul_tbl_l0,y
@pl1_end:
        sta reg+6
        lda reg+0             ; a = (reg0+reg2)^2/4
        clc
        adc reg+2
        tay
        bcc @pl2
        lda __mul_tbl_l1,y
        jmp @pl2_end
@pl2:
        lda __mul_tbl_l0,y
@pl2_end:
        sta reg+7
        sec                     ; reg4 = a - reg6
        sbc reg+6
        sta reg+4
        rts
.endproc
        
;;; int16xint16=>int16 の掛け算
;;;  reg(4,5) = reg(0,1) * reg(2,3)
;;; TODO: 未実装
.proc __mul_16
        rts
.endproc
        
;;; uint8/uint8=>int8 の割り算
;;;  reg4 = reg0 / reg2 ( 余り=reg5)
;;; USING: reg[0,2,4,5]
.proc __div_8
        ldy #8
        lda #0
        sta reg+5
@loop:
        rol reg+0
        rol reg+5
        
        lda reg+5
        sec
        sbc reg+2
        bcc @end
        sta reg+5
@end:   
        rol reg+4
        dey
        bne @loop
        rts
.endproc

;;; sint8/sint8=>sint8 の割り算
;;;  reg4 = reg0 / reg2 ( 余り=reg5)
;;; USING: reg[0,2,4,5,6,7]
.proc __div_8s
		lda #0
		sta reg+6				; reg6 = 0
		sta reg+7				; reg7 = 0
		
		lda reg+0				; if sign(reg0) then goto .reg0_pos
		bpl @reg0_pos
		lda #0					; reg0 = -reg0
		sec
		sbc reg+0
		sta reg+0
		lda #1					; reg6 = 1
		sta reg+6
@reg0_pos:
		
		lda reg+2				; if sign(reg2) then goto .reg2_pos
		bpl @reg2_pos
		lda #0					; reg2 = -reg2
		sec
		sbc reg+2
		sta reg+2
		lda #1 					; reg7 = 1; reg6 = !reg6
		sta reg+7
		eor reg+6
		sta reg+6				
@reg2_pos:

		jsr __div_8				; reg4 = reg0 / reg2
		
		lda reg+6				; if reg6 then goto .else2
		beq @else1
		lda #0					; reg4 = -reg4
		sec
		sbc reg+4				
		sta reg+4
		lda reg+5				; if reg5 == 0 then goto .else1
		beq @else1
		lda reg+2				; reg5 = reg2 - reg5
		sec
		sbc reg+5
		sta reg+5
		dec reg+4				; reg4 = reg4-1
@else1:
		lda reg+7				; if reg7 then goto .else2
		beq @else2
		lda #0					; reg5 = -reg5
		sec
		sbc reg+5
		sta reg+5
@else2:
		rts
.endproc
        
;;; int16/int16=>int16 の割り算 
;;;  reg(4,5) = reg(0,1) / reg(2,3) ( 余り=reg(6,7))
;;; USING: y, reg[0..8]
;;; TODO: たぶん動いてない
.proc __div_16
	txa
	pha
	ldy #16
	lda #0
	sta reg+6
	sta reg+7
@loop:
	rol reg+0				; rol reg(7,6,1,0)
	rol reg+1
	rol reg+6
	rol reg+7

	sec						; sub reg(7,6), reg(3,2)
	lda reg+6
	sbc reg+2
	tax
	lda reg+7
	sbc reg+3
	bcc @end
	sta reg+7
	stx reg+6
@end:   
	rol reg+4				; rol reg(5,4)
	rol reg+5
	dey
	bne @loop
	
	pla
	tax
	rts
.endproc
	
;;; int8%int8=>int8 の割り算の余り
;;;  reg4 = reg0 % reg2
;;; USING: reg[0,2,4,5,6,7]
.proc __mod_8
        jsr __div_8
        lda reg+5
        sta reg+4
        rts
.endproc
        
.proc __mod_16
        rts						; 未実装
.endproc
