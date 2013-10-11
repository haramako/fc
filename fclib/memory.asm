;; function memcpy(from:int*, to:int*, size:int):void
;; {
;;   var i = 0;
;;   while( i < size ){
;;     p[i] = c;
;;     i += 1;
;;   }
;; }
;;; USING Y
_memcpy:
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
        lda [reg],y
        sta [reg+2],y
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
_memset:
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
_memzero:
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
