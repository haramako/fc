; inflate - uncompress data stored in the DEFLATE format
; by Piotr Fusik <fox@scene.pl>
; Last modified: 2007-06-17

; Compile with xasm (http://xasm.atari.org/), for example:
; xasm inflate.asx /l /d:inflate=$b700 /d:inflate_data=$b900 /d:inflate_zp=$f0
; inflate is 509 bytes of code and initialized data
; inflate_data is 764 bytes of uninitialized data
; inflate_zp is 10 bytes on page zero

	;; .bank 0
	;; .org 0x800
inflate_zp equ $f0

; Pointer to compressed data
inputPointer                    equ	inflate_zp    ; 2 bytes

; Pointer to uncompressed data
outputPointer                   equ	inflate_zp+2  ; 2 bytes

; Local variables

getBit_buffer                   equ	inflate_zp+4  ; 1 byte

getBits_base                    equ	inflate_zp+5  ; 1 byte
isb_pageCounter  equ	inflate_zp+5  ; 1 byte

ic_sourcePointer      equ	inflate_zp+6  ; 2 bytes
idb_lengthIndex equ	inflate_zp+6  ; 1 byte
idb_lastLength	equ	inflate_zp+7  ; 1 byte
idb_tempCodes   equ	inflate_zp+7  ; 1 byte

ic_lengthMinus2       equ	inflate_zp+8  ; 1 byte
idb_allCodes    equ	inflate_zp+8  ; 1 byte

ic_primaryCodes       equ	inflate_zp+9  ; 1 byte


; Argument values for getBits
GET_1_BIT                       equ	$81
GET_2_BITS                      equ	$82
GET_3_BITS                      equ	$84
GET_4_BITS                      equ	$88
GET_5_BITS                      equ	$90
GET_6_BITS                      equ	$a0
GET_7_BITS                      equ	$c0

; Maximum length of a Huffman code
MAX_CODE_LENGTH                 equ	15

; Huffman trees
TREE_SIZE                       equ	MAX_CODE_LENGTH+1
PRIMARY_TREE                    equ	0
DISTANCE_TREE                   equ	TREE_SIZE

; Alphabet
LENGTH_SYMBOLS                  equ	1+29+2
DISTANCE_SYMBOLS                equ	30
CONTROL_SYMBOLS                 equ	LENGTH_SYMBOLS+DISTANCE_SYMBOLS
TOTAL_SYMBOLS                   equ	256+CONTROL_SYMBOLS

_inflate_unpack:
	lda S+2,x
	sta outputPointer+0
	lda S+3,x
	sta outputPointer+1
	lda S+4,x
	sta inputPointer+0
	lda S+5,x
	sta inputPointer+1
	txa
	pha
	jsr inflate
	pla
	tax
	lda outputPointer+0
	sec
	sbc S+2,x
	sta S+0,x
	lda outputPointer+1
	sbc S+3,x
	sta S+1,x
	rts

; Uncompress DEFLATE stream starting from the address stored in inputPointer
; to the memory starting from the address stored in outputPointer
	;; org	inflate
inflate:	
	ldy #0
	sty	getBit_buffer
inflate_blockLoop
; Get a bit of EOF and two bits of block type
;	ldy	#0
	sty	getBits_base
	lda	#GET_3_BITS
	jsr	getBits
	lsr	a
	php
	tax
	bne	inflateCompressedBlock

; Copy uncompressed block
;	ldy	#0
	sty	getBit_buffer
	jsr	getWord
	jsr	getWord
	sta	isb_pageCounter
;	jmp	isb_firstByte
	bcs	isb_firstByte
isb_copyByte
	jsr	getByte
inflateStoreByte
	jsr	storeByte
	bcc	ic_loop
isb_firstByte
	inx
	bne	isb_copyByte
	inc	isb_pageCounter
	bne	isb_copyByte

inflate_nextBlock
	plp
	bcc	inflate_blockLoop
	rts

inflateCompressedBlock

; Decompress a block with fixed Huffman trees:
; :144 dta 8
; :112 dta 9
; :24  dta 7
; :6   dta 8
; :2   dta 8 ; codes with no meaning
; :30  dta 5+DISTANCE_TREE
;	ldy	#0
ifb_setCodeLengths
	lda	#4
	cpy	#144
	rol	a
	sta	literalSymbolCodeLength,y
	cpy	#CONTROL_SYMBOLS
	bcs	ifb_noControlSymbol
	lda	#5+DISTANCE_TREE
	cpy	#LENGTH_SYMBOLS
	bcs	ifb_setControlCodeLength
	cpy	#24
	adc	#2-DISTANCE_TREE
ifb_setControlCodeLength
	sta	controlSymbolCodeLength,y
ifb_noControlSymbol
	iny
	bne	ifb_setCodeLengths
	lda #LENGTH_SYMBOLS
	sta ic_primaryCodes

	dex
	beq	ic

; Decompress a block reading Huffman trees first

; Build the tree for temporary codes
	jsr	buildTempHuffmanTree

; Use temporary codes to get lengths of literal/length and distance codes
	ldx	#0
;	sec
idb_decodeLength
	php
	stx	idb_lengthIndex
; Fetch a temporary code
	jsr	fetchPrimaryCode
; Temporary code 0..15: put this length
	tax
	bpl	idb_verbatimLength
; Temporary code 16: repeat last length 3 + getBits(2) times
; Temporary code 17: put zero length 3 + getBits(3) times
; Temporary code 18: put zero length 11 + getBits(7) times
	jsr	getBits
;	sec
	adc	#1
	cpx	#GET_7_BITS
	bcc .b1
	adc	#7
.b1:
	tay
	lda	#0
	cpx	#GET_3_BITS
	bcs .b2
	lda	idb_lastLength
.b2:
idb_verbatimLength
	iny
	ldx	idb_lengthIndex
	plp
idb_storeLength
	bcc	idb_controlSymbolCodeLength
	sta	literalSymbolCodeLength,x
	inx
	cpx	#1
idb_storeNext
	dey
	bne	idb_storeLength
	sta	idb_lastLength
;	jmp	idb_decodeLength
	beq	idb_decodeLength
idb_controlSymbolCodeLength
	cpx	ic_primaryCodes
	bcc .b3
	ora	#DISTANCE_TREE
.b3:
	sta	controlSymbolCodeLength,x
	inx
	cpx	idb_allCodes
	bcc	idb_storeNext
	dey
;	ldy	#0
;	jmp	ic

; Decompress a block
ic
	jsr	bht
ic_loop
	jsr	fetchPrimaryCode
	bcs .b7
	jmp	inflateStoreByte
.b7:
	tax
	beq	inflate_nextBlock
; Copy sequence from look-behind buffer
;	ldy	#0
	sty	getBits_base
	cmp	#9
	bcc	ic_setSequenceLength
	tya
;	lda	#0
	cpx	#1+28
	bcs	ic_setSequenceLength
	dex
	txa
	lsr	a
	ror	getBits_base
	inc	getBits_base
	lsr	a
	rol	getBits_base
	jsr	getAMinus1BitsMax8
;	sec
	adc	#0
ic_setSequenceLength
	sta	ic_lengthMinus2
	ldx	#DISTANCE_TREE
	jsr	fetchCode
;	sec
	sbc	ic_primaryCodes
	tax
	cmp	#4
	bcc	ic_setOffsetLowByte
	inc	getBits_base
	lsr	a
	jsr	getAMinus1BitsMax8
ic_setOffsetLowByte
	eor	#$ff
	sta	ic_sourcePointer
	lda	getBits_base
	cpx	#10
	bcc	ic_setOffsetHighByte
	lda	getNPlus1Bits_mask-10,x
	jsr	getBits
	clc
ic_setOffsetHighByte
	eor	#$ff
;	clc
	adc	outputPointer+1
	sta	ic_sourcePointer+1
	jsr	copyByte
	jsr	copyByte
ic_copyByte
	jsr	copyByte
	dec	ic_lengthMinus2
	bne	ic_copyByte
;	jmp	ic_loop
	beq	ic_loop

buildTempHuffmanTree
;	ldy	#0
	tya
idb_clearCodeLengths
	sta	literalSymbolCodeLength,y
	sta	literalSymbolCodeLength+TOTAL_SYMBOLS-256,y
	iny
	bne	idb_clearCodeLengths
; numberOfPrimaryCodes = 257 + getBits(5)
; numberOfDistanceCodes = 1 + getBits(5)
; numberOfTemporaryCodes = 4 + getBits(4)
	ldx	#3
idb_getHeader
	lda	ifdb_headerBits-1,x
	jsr	getBits
;	sec
	adc	ifdb_headerBase-1,x
	sta	idb_tempCodes-1,x
	sta	ifdb_headerBase+1
	dex
	bne	idb_getHeader

; Get lengths of temporary codes in the order stored in tempCodeLengthOrder
;	ldx	#0
idb_getTempCodeLengths
	lda	#GET_3_BITS
	jsr	getBits
	ldy	tempCodeLengthOrder,x
	sta	literalSymbolCodeLength,y
	ldy	#0
	inx
	cpx	idb_tempCodes
	bcc	idb_getTempCodeLengths

; Build Huffman trees basing on code lengths (in bits)
; stored in the *SymbolCodeLength arrays
bht
; Clear nbc_totalCount, nbc_literalCount, nbc_controlCount
	tya
;	lda	#0
.b4:
	sta	nbc_clearFrom,y
	iny
	bne .b4
; Count number of codes of each length
;	ldy	#0
bht_countCodeLengths
	ldx	literalSymbolCodeLength,y
	inc	nbc_literalCount,x
	inc	nbc_totalCount,x
	cpy	#CONTROL_SYMBOLS
	bcs	bht_noControlSymbol
	ldx	controlSymbolCodeLength,y
	inc	nbc_controlCount,x
	inc	nbc_totalCount,x
bht_noControlSymbol
	iny
	bne	bht_countCodeLengths
; Calculate offsets of symbols sorted by code length
;	lda	#0
	ldx	#-3*TREE_SIZE
bht_calculateOffsets
	sta	nbc_literalOffset+3*TREE_SIZE-$100,x
	clc
	adc	nbc_literalCount+3*TREE_SIZE-$100,x
	inx
	bne	bht_calculateOffsets
; Put symbols in their place in the sorted array
;	ldy	#0
bht_assignCode
	tya
	ldx	literalSymbolCodeLength,y
	ldy	nbc_literalOffset,x
	inc	nbc_literalOffset,x
	sta	codeToLiteralSymbol,y
	tay
	cpy	#CONTROL_SYMBOLS
	bcc .b8
	jmp	bht_noControlSymbol2
.b8:
	ldx	controlSymbolCodeLength,y
	ldy	nbc_controlOffset,x
	inc	nbc_controlOffset,x
	sta	codeToControlSymbol,y
	tay
bht_noControlSymbol2
	iny
	bne	bht_assignCode
	rts

; Read Huffman code using the primary tree
fetchPrimaryCode
	ldx	#PRIMARY_TREE
; Read a code from input basing on the tree specified in X,
; return low byte of this code in A,
; return C flag reset for literal code, set for length code
fetchCode
;	ldy	#0
	tya
fetchCode_nextBit
	jsr	getBit
	rol	a
	inx
	sec
	sbc	nbc_totalCount,x
	bcs	fetchCode_nextBit
;	clc
	adc	nbc_controlCount,x
	bcs	fetchCode_control
;	clc
	adc	nbc_literalOffset,x
	tax
	lda	codeToLiteralSymbol,x
	clc
	rts
fetchCode_control
	clc
	adc	nbc_controlOffset-1,x
	tax
	lda	codeToControlSymbol,x
	sec
	rts

; Read A minus 1 bits, but no more than 8
getAMinus1BitsMax8
	rol	getBits_base
	tax
	cmp	#9
	bcs	getByte
	lda	getNPlus1Bits_mask-2,x
getBits
	jsr	getBits_loop
getBits_normalizeLoop
	lsr	getBits_base
	ror	a
	bcc	getBits_normalizeLoop
	rts

; Read 16 bits
getWord
	jsr	getByte
	tax
; Read 8 bits
getByte
	lda	#$80
getBits_loop
	jsr	getBit
	ror	a
	bcc	getBits_loop
	rts

; Read one bit, return in the C flag
getBit
	lsr	getBit_buffer
	bne	getBit_return
	pha
;	ldy	#0
	lda	[inputPointer],y
	inc inputPointer
	bne .b6
	inc inputPointer+1
.b6:
	sec
	ror	a
	sta	getBit_buffer
	pla
getBit_return
	rts

; Copy a previously written byte
copyByte
	ldy	outputPointer
	lda	[ic_sourcePointer],y
	ldy	#0
; Write a byte
storeByte
	sta	[outputPointer],y
	inc	outputPointer
	bne	storeByte_return
	inc	outputPointer+1
	inc	ic_sourcePointer+1
storeByte_return
	rts

getNPlus1Bits_mask
	.db	GET_1_BIT,GET_2_BITS,GET_3_BITS,GET_4_BITS,GET_5_BITS,GET_6_BITS,GET_7_BITS

tempCodeLengthOrder
	.db	GET_2_BITS,GET_3_BITS,GET_7_BITS,0,8,7,9,6,10,5,11,4,12,3,13,2,14,1,15

ifdb_headerBits	.db	GET_4_BITS,GET_5_BITS,GET_5_BITS
ifdb_headerBase	.db	3,0,0  ; second byte is modified at runtime!

	;; org inflate_data
	;; .org 0x0600

; Data for building trees

bss_base equ $0000	

literalSymbolCodeLength equ bss_base+$500
	;; .org	*+256
controlSymbolCodeLength equ bss_base+$600
	;; .org	*+CONTROL_SYMBOLS

; Huffman trees

nbc_clearFrom equ bss_base+$640
nbc_totalCount equ bss_base+$640
	;; org	*+2*TREE_SIZE
nbc_literalCount equ bss_base+$660
	;; org	*+TREE_SIZE
nbc_controlCount equ bss_base+$670
	;; org	*+2*TREE_SIZE
nbc_literalOffset equ bss_base+$690
	;; org	*+TREE_SIZE
nbc_controlOffset equ bss_base+$6a0
	;; org	*+2*TREE_SIZE

codeToLiteralSymbol equ bss_base+$700
	;; org	*+256
codeToControlSymbol equ bss_base+$480
	;; org	*+CONTROL_SYMBOLS

	;; end
