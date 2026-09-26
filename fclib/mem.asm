.segment "mem"
	
;; function memcpy(_to:int*, _from:int*, size:int):void
;; {
;;   var i = 0;
;;   while( i < size ){
;;     p[i] = c;
;;     i += 1;
;;   }
;; }
;;; USING Y
_mem_copy:
	;; fastcall: _to = FC_FASTCALL_REG+0,1、_from = +2,3、size = +4,5 (ポインタと size はその場で進める)
	lda FC_FASTCALL_REG+5

;;; 256byteごとのコピー
	beq @end
@loop:
	ldy #0
:	lda (FC_FASTCALL_REG+2),y
	sta (FC_FASTCALL_REG+0),y
	iny
	bne :-
	inc FC_FASTCALL_REG+3
	inc FC_FASTCALL_REG+1
	dec FC_FASTCALL_REG+5
	bne @loop
@end:	
	

;;; 残りのコピー
	lda FC_FASTCALL_REG+4
	beq @end2
    ldy #0
:	lda (FC_FASTCALL_REG+2),y
    sta (FC_FASTCALL_REG+0),y
    iny
    cpy FC_FASTCALL_REG+4
    bne :-
@end2:

    rts
        
;; function set(p:*u8, c:u8, size:u16):void
;;; USING Y
_mem_set:
	;; fastcall: p = FC_FASTCALL_REG+0,1、c = +2、size = +3,4 (ポインタと size はその場で進める。size 0 なら何もしない)
	lda FC_FASTCALL_REG+2
	ldy FC_FASTCALL_REG+4
	beq @rest
@page:						; 256 バイトごと
	ldy #0
:	sta (FC_FASTCALL_REG+0),y
	iny
	bne :-
	inc FC_FASTCALL_REG+1
	dec FC_FASTCALL_REG+4
	bne @page
@rest:						; 残り (後ろから)
	ldy FC_FASTCALL_REG+3
	beq @end
:	dey
	sta (FC_FASTCALL_REG+0),y
	bne :-
@end:
	rts

;; function zero(p:*u8, size:u16):void
;;; USING Y
_mem_zero:
	;; fastcall: p = FC_FASTCALL_REG+0,1、size = +2,3 (ポインタと size はその場で進める。size 0 なら何もしない)
	lda #0
	ldy FC_FASTCALL_REG+3
	beq @rest
@page:
	ldy #0
:	sta (FC_FASTCALL_REG+0),y
	iny
	bne :-
	inc FC_FASTCALL_REG+1
	dec FC_FASTCALL_REG+3
	bne @page
@rest:
	ldy FC_FASTCALL_REG+2
	beq @end
:	dey
	sta (FC_FASTCALL_REG+0),y
	bne :-
@end:
	rts

;; function compare(p1:*const u8, p2:*const u8, size:u16):u8 (等しければ 0、違えば 1)
;;; USING Y
_mem_compare:
	;; fastcall: 戻り値 = FC_FASTCALL_REG+0、p1 = +1,2、p2 = +3,4、size = +5,6 (ポインタと size はその場で進める。size 0 なら等しい)
	lda FC_FASTCALL_REG+6
	beq @rest
@page:
	ldy #0
:	lda (FC_FASTCALL_REG+1),y
	cmp (FC_FASTCALL_REG+3),y
	bne @fail
	iny
	bne :-
	inc FC_FASTCALL_REG+2
	inc FC_FASTCALL_REG+4
	dec FC_FASTCALL_REG+6
	bne @page
@rest:
	ldy FC_FASTCALL_REG+5
	beq @equal
:	dey
	lda (FC_FASTCALL_REG+1),y
	cmp (FC_FASTCALL_REG+3),y
	bne @fail
	tya
	bne :-
@equal:
	lda #0
	sta FC_FASTCALL_REG+0
	rts
@fail:
	lda #1
	sta FC_FASTCALL_REG+0
	rts
