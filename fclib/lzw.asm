result = FC_FASTCALL_REG+0
x_backup = FC_FASTCALL_REG+8
dest_addr = FC_FASTCALL_REG+10
total_len = FC_FASTCALL_REG+12
idx = FC_FASTCALL_REG+14
len = FC_FASTCALL_REG+15
	
addr = _lzw_addr
bpos = _lzw_bpos 			; curの残りビット数
cur = _lzw_cur				; 現在読んでいるbyteの内容

;;; use FC_FASTCALL_REG[8]
.proc _lzw_read_bit
	stx x_backup
	
	lda #0
	sta result
	sta result+1
	ldx FC_FASTCALL_REG+2
	beq @end
	
@loop:
	;; byteを使いきったら読み込む
	lda bpos
	bne @no_read
	tay 						; == ldy #0
	lda (addr),y
	sta cur
	
	clc
	lda addr+0
	adc #1
	sta addr+0
	lda addr+1
	adc #0
	sta addr+1
	
	lda #8
	sta bpos
@no_read:

	;; curから1bit読み込む
	rol cur
	rol result+0
	rol result+1
	
	dec bpos
	dex
	bne @loop
	
@end:	
	ldx x_backup
	rts
.endproc

;;; use FC_FASTCALL_REG[8]
.proc _lzw_read_vln
	lda #1
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit

	lda FC_FASTCALL_REG+0
	bne @long
	lda #4
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	rts
@long:	
	lda #8
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	rts
.endproc

;;; use FC_FASTCALL_REG[8]
.proc _lzw_read_vln16
	lda #1
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit

	lda FC_FASTCALL_REG+0
	bne @long
	lda #8
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	rts
@long:	
	lda #16
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	rts
.endproc

.macro dout aa
	lda aa+0
	sta _stdio_EMU_DATA+0
	lda aa+1
	sta _stdio_EMU_DATA+1
	lda #3
	sta _stdio_EMU_PRINT
.endmacro

;;; use FC_FASTCALL_REG[8-15]
.proc _lzw_unpack
	lda S+2,x
	sta dest_addr+0
	lda S+3,x
	sta dest_addr+1
	lda S+4,x
	sta addr+0
	lda S+5,x
	sta addr+1
	lda #0
	sta bpos

	jsr _lzw_read_vln16
	lda FC_FASTCALL_REG+0
	sta total_len+0
	sta S+0,x
	lda FC_FASTCALL_REG+1
	sta total_len+1
	sta S+1,x

@loop:
	;; dout total_len

	lda total_len+0				; while(total_len){
	ora total_len+1
	bne @not_end
	jmp @end
@not_end:

	lda #1						;   if(read_bit(1)==0){
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit
	lda FC_FASTCALL_REG+0
	bne @normal

	jsr _lzw_read_vln			;     idx = read_vln();
	lda FC_FASTCALL_REG+0
	sta idx
	
	jsr _lzw_read_vln			;     len = read_vln();
	lda FC_FASTCALL_REG+0
	sta len

	lda dest_addr+0				;     mem.copy(dest_addr, dest_addr-idx, len);
	sta S+6,x
	lda dest_addr+1
	sta S+7,x
	sec
	lda dest_addr+0
	sbc idx
	sta S+8,x
	lda dest_addr+1
	sbc #0
	sta S+9,x
	lda len
	sta S+10,x
	lda #0
	sta S+11,x
	call _mem_copy, #6

	sec							;     total_len -= len;
	lda total_len+0
	sbc len
	sta total_len+0
	lda total_len+1
	sbc #0
	sta total_len+1

	clc							;     dest_addr += len;
	lda dest_addr+0
	adc len
	sta dest_addr+0
	lda dest_addr+1
	adc #0
	sta dest_addr+1
	jmp @loop
@normal:						;   }else{

	lda #8						;     *dest_addr = read_bit(8);
	sta FC_FASTCALL_REG+2
	jsr _lzw_read_bit

	ldy #0
	lda FC_FASTCALL_REG+0
	sta (dest_addr),y
	
	sec							;     total_len -= 1;
	lda total_len+0
	sbc #1
	sta total_len+0
	lda total_len+1
	sbc #0
	sta total_len+1
	
	clc							;     dest_addr += 1;
	lda dest_addr+0
	adc #1
	sta dest_addr+0
	lda dest_addr+1
	adc #0
	sta dest_addr+1
	
	jmp @loop					; } }
@end:

	rts
	
.endproc
