# メモリアクセス命令の集約（load_mem / store_mem）

2026-09-28。`refactor/memops`。IR のメモリアクセスを 2 命令に集約し、番地を `Base + Index * Scale + Disp` の 1 つの形で持つ。

## 動機

それまでのメモリアクセスは 7 つのオペコード（`index` / `pget` / `pset` / `index_pget` / `index_pset` / `field_pget` /
`field_pset`）に `Scaled` フラグと、配列オペランドに掛けた cast（`<k>a`：fieldindex / indexoff が定数のずれを表すために
偽の配列型を作って包んでいた）が重なっていて、

- 定数のずれの表し方が 3 通り（`field_pget` の Src[1]、配列への cast の Offset、`add` 命令）、
- 書く幅の出どころがオペコードごとに違う（`pset` はポインタの先の型、`index_pset` は要素型、`field_pset` は `Op.Type`）、
- 同じ「配列 + 添字」の読み書きを fuse / fieldindex / indexoff / scale / sink がそれぞれの形で照合し、regalloc（friendly /
  needsY）、codegen（gen 6 つ）、interp（case 6 つ）が同じ分岐を持っていた。

fuzz で出たバグの多く（`sa[i].f0 = 4` が隣のフィールドまで書く、`a16[i] = 4` が下位しか書かない、cast の幅の読み違え）は
この表現に由来する（development_notes.md の fuzz の記録）。

## 表現

```
load_mem  d = base, index, scale=s, disp=k            d (幅 = d の型) = mem[base + index*s + k]
store_mem base, index, v, scale=s, disp=k, w=n        mem[base + index*s + k .. +n) = v の下位 n バイト
```

- `Base`（Src[0]）: グローバルの配列（`{g …}`）か、2 バイトのポインタ値。フレーム上のローカル配列は `index`（番地の計算）を
  通してポインタになる（変わらない）。
- `Index`（Src[1]）: 1 バイトの値。無ければ `ir.NoIndex`（番兵のリテラル。`Op.Mem()` は nil にして返す）。定数の添字はそのまま
  Index に置く（Disp に畳まない。畳むと `lda a+3` の絶対番地になって生成コードが変わるので、それは別の最適化として入れる）。
- `Scale`: 添字 1 につき進むバイト数（要素の大きさ）。以前の `Scaled`（添字がバイト単位）は `Scale = 1` と同じ。Index が無ければ 0。
- `Disp`: 定数のずれ（バイト）。以前の `field_pget` の Src[1] と配列への cast の Offset。
- `Width`（store だけ）: 書く幅。以前の `pset` = ポインタの先の大きさ、`index_pset` = 要素（fieldindex ではフィールド）の大きさ、
  `field_pset` = `Op.Type.Size`。値が幅より小さいときは上位を 0 で書く（codegen の byte の規則。以前と同じ）。
- 値（store の Src[2]）。

`index`（`d = &a[i]`。ポインタ値を作る番地の計算）はそのまま。以前の 7 命令との対応:

| 以前 | 今 |
|---|---|
| `pget d = *p` | `load_mem d = p, -, scale=0, disp=0` |
| `pset *p = v` | `store_mem p, -, v, w=sizeof(*p)` |
| `field_pget d = *(p + k)` | `load_mem d = p, -, disp=k` |
| `field_pset *(p + k) = v` (Type T) | `store_mem p, -, v, disp=k, w=sizeof(T)` |
| `index_pget d = a, i` | `load_mem d = a, i, scale=sizeof(要素)` |
| `index_pget d = <k>a, j` (scaled) | `load_mem d = a, j, scale=1, disp=k` |
| `index_pset a, i, v` | `store_mem a, i, v, scale=…, w=sizeof(要素)` |

## codegen のアドレッシングの選び方（`genLoadMem` / `genStoreMem`）

| Base | Index | 出す形 |
|---|---|---|
| ポインタ | 無し | `ldy #disp+i; lda (p),y`（pointerRead / pointerWrite。ゼロページでなければ reg に写す） |
| ポインタ | 有り (disp = 0) | `ldy i` (scale 1) / `ldy #k*scale` (定数) / `lda i; asl; tay` (scale 2)、`lda (p),y`、`iny` |
| グローバル配列 | 有り | `lda a+disp+i,y`（添字が X に常駐していれば `,x`） |
| グローバル配列 | 無し | `lda a+disp+i`（今は生成しない。定数の添字を Disp に畳む最適化を入れたときに使う） |

それ以外の組み合わせ（ポインタ + 添字 + disp、フレームの配列を Base に）は internal error。opt は作らない。

## 状態（2026-09-28）

1〜3 は済み（`refactor/memops`）。golden の IR ダンプの形式だけが変わり、asm golden・examples の ROM はバイト一致。
fuzz（random 500、畳み込み 200、メタモルフィック 50、変異 30）を通した。

4 のうち「定数の添字を Disp に畳む」（`opt.foldConstIndex`。fusePointer の最後。`FC_DISABLE=constidx`）も入れた:
`ldy #k; lda a,y` → `lda a+k`（2 サイクル・2 バイト短く、Y を使わない）。bench は bgdecode −0.8%（サイズ −3.9%）、
oam −1.2%（−1.8%）、castle のフレームは −0.1〜0.2%。ポインタ + 添字 + disp の形はまだ許していない。

実装で引っかかった点:
- `DefUse` の uses から番兵 `NoIndex` を抜くと Src の位置がずれ、SSA が store の値の位置に定数を伝播しなくなった（`a[0] = 1` の
  `lda #1` が消えず、生成コードが変わった）。uses は Src をそのまま返す。
- sink / devirt / ywalk が「t を番地にした参照」を探すとき、以前は `Src[0] == t`（cast 無し）の厳密な一致だった。fuse と同じ
  「先頭への cast も同じ」判定 (`isSameOperand`) に広げると sink がより多く動かして命令の並びが変わり、oam が +7 バイトになった
  (`plainDeref` は厳密な一致、`fusableDeref` は cast も同じと見る、で分けた）。生成コードを変えない移行では、照合の緩さを
  変えないこと。

## 段階

1. ir: 2 命令と `Op.Mem()`、`ir.NoIndex`、Verify の規則。golden IR の形式が変わる（`-update`）。
2. sema / opt / regalloc / codegen / interp / frames を新しい形に。**生成する asm は変えない**（golden asm は綴りが変わる所
   （`_a+3+0,y` → `_a+3,y`）だけ。ROM はバイト一致のまま）。
3. fuzz（TestRandomPrograms / V3 / ConstFold / Metamorphic / Mutate）を重めに回す。
4. その後の最適化（別のコミット）: 定数の添字を Disp に畳んで絶対番地で読む、ポインタ + 添字 + disp を許す。
