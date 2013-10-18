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
		lda S+0,x
		sta reg+0
		lda S+1,x
		sta reg+1
		lda S+2,x
		sta reg+2
		lda S+3,x
		sta reg+3
		lda S+4,x
		sta reg+4
        ldy #0
.loop:
        lda [reg+2],y
        sta [reg],y
        iny
        cpy reg+4
        bne .loop
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
		lda S+0,x
		sta reg+0
		lda S+1,x
		sta reg+1
		lda S+2,x
		sta reg+2
        ldy #0
.loop:
        lda S+2,x
        sta [reg+0],y
        iny
        cpy reg+2
        bne .loop
        rts

;;; USING Y
_mem_zero:
		lda S+0,x
		sta reg+0
		lda S+1,x
		sta reg+1
		lda S+2,x
		sta reg+2
        ldy #0
		lda #0
.loop:
        sta [reg+0],y
        iny
        cpy reg+2
        bne .loop
        rts

;;; USING Y
_mem_compare:
	lda S+1,x
	sta reg+0
	lda S+2,x
	sta reg+1
	lda S+3,x
	sta reg+2
	lda S+4,x
	sta reg+3
	lda S+5,x
	sta reg+4
	
	ldy #0
.loop:
	lda [reg+0],y
	cmp [reg+2],y
	bne .fail
	iny
	cpy reg+4
	bne .loop
	
	lda #0
	sta S+0,x
	rts

.fail:
	lda #1
	sta S+0,x
	rts
