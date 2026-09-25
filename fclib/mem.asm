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
        
;; function memset(p:int*, c:int, size:int):void
;; {
;;   var i = 0;
;;   while( i < size ){
;;     p[i] = c;
;;     i += 1;
;;   }
;; }
;;; USING Y
_mem_set:
    ldy FC_FASTCALL_REG+3
	beq :++
	lda FC_FASTCALL_REG+2
:	dey
	sta (FC_FASTCALL_REG+0),y
	bne :-
:	rts

;;; USING Y
_mem_zero:
	;; fastcall: p = FC_FASTCALL_REG+0,1、size = +2
	ldy #0
	lda #0
:	sta (FC_FASTCALL_REG+0),y
	iny
	cpy FC_FASTCALL_REG+2
    bne :-
	rts

;;; USING Y
_mem_compare:
	;; fastcall: 戻り値 = FC_FASTCALL_REG+0、p1 = +1,2、p2 = +3,4、size = +5
	ldy #0
:	lda (FC_FASTCALL_REG+1),y
	cmp (FC_FASTCALL_REG+3),y
	bne @fail
	iny
	cpy FC_FASTCALL_REG+5
	bne :-
	
	lda #0
	sta FC_FASTCALL_REG+0
	rts

@fail:
	lda #1
	sta FC_FASTCALL_REG+0
	rts
