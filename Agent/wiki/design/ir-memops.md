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
この表現に由来する（Agent/discussions/2026-09-19-fuzz-findings-log.md の fuzz の記録）。

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
| ポインタ | 有り (disp > 0) | `lda i; (asl); clc; adc #disp; tay`（`loadYIdxDisp`。A を壊し、添字が Y に常駐していれば Y も） |
| グローバル配列 | 有り | `lda a+disp+i,y`（添字が X に常駐していれば `,x`） |
| グローバル配列 | 無し | `lda a+disp+i`（定数の添字を Disp に畳んだ形） |

`(p),y` には変位が無いので、ポインタ経由の添字とずれは Y に足し込む。**添字 * scale + disp + 幅 ≤ 256 は作る側が保証する**
（Verify が見るのは静的な disp + 幅 ≤ 256 と、変数の添字の scale が 1・2 であることだけ）。作るのは fuseArrayField だけで、
配列の長さが型で分かる添字（struct の配列フィールド）に限る。フレームの配列を Base にする形は無い（`index` でポインタにする）。

## 状態（2026-09-28）

1〜3 は済み（`refactor/memops`）。golden の IR ダンプの形式だけが変わり、asm golden・examples の ROM はバイト一致。
fuzz（random 500、畳み込み 200、メタモルフィック 50、変異 30）を通した。

4 のうち「定数の添字を Disp に畳む」（`opt.foldConstIndex`。fusePointer の最後。`FC_DISABLE=constidx`）も入れた:
`ldy #k; lda a,y` → `lda a+k`（2 サイクル・2 バイト短く、Y を使わない）。bench は bgdecode −0.8%（サイズ −3.9%）、
oam −1.2%（−1.8%）、castle のフレームは −0.1〜0.2%。

4 の残り「ポインタ + 添字 + disp」も入れた（`opt.fuseArrayField`。fusePointer の最後。`FC_DISABLE=fieldptr`）。
struct の配列フィールドをポインタ経由で引く `p.items[i].q` は、`add t1 = p, #k`（t1 は `*[L]U`）・`index t2 = <*U>t1, i`・
`load_mem d = t2, disp=m` の 16 ビットの番地の計算だったのが `load_mem d = p, i, scale=sizeof(U), disp=k+m`
（`lda i; asl; clc; adc #k+m; tay; lda (p),y`）になる。添字が長さ L 未満なのは言語の規則（範囲外の添字と配列の外への
ポインタ演算は未定義。docs/reference/language.md の「添字と slice」「ポインタ」）で、k + 配列全体の大きさ ≤ 256 のときだけ作る。要素 3 バイト以上は
fieldindex と同じく `mul j = i, #s`。add と index が参照から離れていても（store の右辺の計算が挟まる）、入力が参照までに
書き換わらないことを sinkAddress の canSink で見て畳む。あわせて fieldindex も定数の添字を絶対番地に畳む
（`objs[6].y = 240` が 8 命令から `lda #240; sta objs+43`。入れ子の `ds[1].items[2].id = 5` は `sta ds+13`）。

castle・miku・golden のプログラムにはこの形が無く（計測した: ポインタ経由の配列フィールドの添字は 0 件）、ROM と asm は
変わらない。bench は oam のサイズだけ −1.0%（上の定数の添字）。fuzz の生成器は `pt.arr[(e & 3)]`（struct T の配列
フィールドをポインタ経由で）を作るので、60 本で 360 件ほど通る（要素 3 バイト以上は生成器が作らないので TestFieldPtr と
opt の TestFuseArrayField で見る）。

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
4. その後の最適化（別のコミット）: 定数の添字を Disp に畳んで絶対番地で読む、ポインタ + 添字 + disp を許す（どちらも済み）。
