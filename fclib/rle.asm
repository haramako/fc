;;; http://codebase64.org/doku.php?id=base:rle_pack_unpack

.segment "CODE"
	
src		= reg+0		; borrow cc65's temp pointers
dest	= reg+2
lastbyte = reg+4		; last byte read
destlen = reg+5		; number of bytes written (2bytes)
x_backup = reg+7


; read a byte and increment source pointer
rle_read:
	lda (src),y
	inc src
	bne @else
	inc src + 1
@else:
	rts


; write a byte and increment destination pointer
rle_store:
	sta (dest),y
	inc dest
	bne @else1
	inc dest + 1
@else1:
	inc destlen
	bne @else2
	inc destlen + 1
@else2:
	rts

; cc65 interface to rle_unpack
; unsigned int __fastcall__ rle_unpack(unsigned char *dest, unsigned char *src);
_rle_unpack:
	lda S+2,x
	sta dest
	lda S+3,x
	sta dest+1
	lda S+4,x
	sta src
	lda S+5,x
	sta src+1
	stx x_backup
	jsr rle_unpack		; execute
	ldx x_backup
	lda destlen
	sta S+0,x
	lda destlen+1
	sta S+1,x
	rts

; unpack a run length encoded stream
rle_unpack:
	ldy #0
	sty destlen		; reset byte counter
	sty destlen + 1
	jsr rle_read		; read the first byte
	sta lastbyte		; save as last byte
	jsr rle_store		; store
@unpack:
	jsr rle_read		; read next byte
	cmp lastbyte		; same as last one?
	beq @rle		; yes, unpack
	sta lastbyte		; save as last byte
	jsr rle_store		; store
	jmp @unpack		; next
@rle:
	jsr rle_read		; read byte count
	tax
	beq @end		; 0 = end of stream
	lda lastbyte
@read:
	jsr rle_store		; store X bytes
	dex
	bne @read
	beq @unpack		; next
@end:
	rts
