;;; 旧 fclib の mem.asm のコピー (2026-09-29。doc/v4_stdlib.md §7)
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
	;; abi "frame" (F_mem_copy): _to = F_mem_copy+0,1、_from = +2,3、size = +4,5 (ポインタと size はその場で進める)
	lda F_mem_copy+5

;;; 256byteごとのコピー
	beq @end
@loop:
	ldy #0
:	lda (F_mem_copy+2),y
	sta (F_mem_copy+0),y
	iny
	bne :-
	inc F_mem_copy+3
	inc F_mem_copy+1
	dec F_mem_copy+5
	bne @loop
@end:	
	

;;; 残りのコピー
	lda F_mem_copy+4
	beq @end2
    ldy #0
:	lda (F_mem_copy+2),y
    sta (F_mem_copy+0),y
    iny
    cpy F_mem_copy+4
    bne :-
@end2:

    rts
        
;; function set(p:*u8, c:u8, size:u16):void
;;; USING Y
_mem_set:
	;; abi "frame" (F_mem_set): p = F_mem_set+0,1、c = +2、size = +3,4 (ポインタと size はその場で進める。size 0 なら何もしない)
	lda F_mem_set+2
	ldy F_mem_set+4
	beq @rest
@page:						; 256 バイトごと
	ldy #0
:	sta (F_mem_set+0),y
	iny
	bne :-
	inc F_mem_set+1
	dec F_mem_set+4
	bne @page
@rest:						; 残り (後ろから)
	ldy F_mem_set+3
	beq @end
:	dey
	sta (F_mem_set+0),y
	bne :-
@end:
	rts

;; function zero(p:*u8, size:u16):void
;;; USING Y
_mem_zero:
	;; abi "frame" (F_mem_zero): p = F_mem_zero+0,1、size = +2,3 (ポインタと size はその場で進める。size 0 なら何もしない)
	lda #0
	ldy F_mem_zero+3
	beq @rest
@page:
	ldy #0
:	sta (F_mem_zero+0),y
	iny
	bne :-
	inc F_mem_zero+1
	dec F_mem_zero+3
	bne @page
@rest:
	ldy F_mem_zero+2
	beq @end
:	dey
	sta (F_mem_zero+0),y
	bne :-
@end:
	rts

;; function compare(p1:*const u8, p2:*const u8, size:u16):u8 (等しければ 0、違えば 1)
;;; USING Y
_mem_compare:
	;; abi "frame" (F_mem_compare): 戻り値 = F_mem_compare+0、p1 = +1,2、p2 = +3,4、size = +5,6 (ポインタと size はその場で進める。size 0 なら等しい)
	lda F_mem_compare+6
	beq @rest
@page:
	ldy #0
:	lda (F_mem_compare+1),y
	cmp (F_mem_compare+3),y
	bne @fail
	iny
	bne :-
	inc F_mem_compare+2
	inc F_mem_compare+4
	dec F_mem_compare+6
	bne @page
@rest:
	ldy F_mem_compare+5
	beq @equal
:	dey
	lda (F_mem_compare+1),y
	cmp (F_mem_compare+3),y
	bne @fail
	tya
	bne :-
@equal:
	lda #0
	sta F_mem_compare+0
	rts
@fail:
	lda #1
	sta F_mem_compare+0
	rts
