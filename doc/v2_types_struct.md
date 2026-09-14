# 型構文（Go/Zig 順）と struct の設計メモ

作成: 2026-09-14（Opus 5）。**2026-09-14 レビュー済み（Q1〜Q8・S1〜S5 決定、§8）**。
前提は [v2_grammar.md](v2_grammar.md)（文法 v2、実装済み）。

---

## 1. 目的

1. 型の表記を C 風の後置（`int*[4]`）から Go / Zig 風の前置（`[4]*int`）に変える。
   読む順序が「外側から内側」= 英語の語順になり、複合型（配列のポインタ、関数のポインタの配列）で迷わない
2. `struct` を入れる。castle は敵の状態を `en.px[i]`, `en.py[i]`, `en.p1[i]`… のように**配列の束**で持っている
   （`en.fc` に 13 本）。構造体があれば `enemies[i].px` と書ける

---

## 2. 現状

### 2.1 型の表記（コーパス集計、v2 ファイル 62 本）

| 現行 | 件数 | 意味 |
|---|---|---|
| `int[N]`（`sint8[N]`, `int16[N]` 含む） | 83 | 配列 |
| `int*`（`uint8*`, `uint16*`, `sint*` 含む） | 82 | ポインタ |
| `int[]`（`uint16[]`, `sint[]` 含む） | 32 | 長さ省略配列（引数、初期値付き定数） |
| `void(int)`, `int(int*, int)` | 11 | 関数型（値は 2 バイトのアドレス） |
| `void(int)[]`, `int(..)[]` | 4 | **関数型の配列**（castle の `en_vtbl` の仮想関数表） |
| `int*[16]`, `int*[]` | 5 | **ポインタの配列**（debug_menu の項目文字列） |
| `<sint>x`, `<uint16*>p` | 26 | キャスト |

文法は左再帰の後置形（`type_decl: type_decl '[' exp ']' | type_decl '*' | type_decl '(' params ')' | IDENT`）。
`int*[16]` は「int へのポインタの配列」、`int[16]*` は「配列へのポインタ」で、C と同じく内側から読む。

### 2.2 型の内部表現

`types.Type{Kind, Size, Signed, Base, Length, Params}`。Kind は Void / Bool / Int / Module / Macro / Pointer /
Array / Func。`Universe` でインターンされ、名前は `uint8*`, `uint8[4]`, `fastcall void(uint8)` の形（golden の
ir/allocir ダンプに出る）。

### 2.3 `.` の扱い

`a.b` は二項演算子 `.`（最高優先順位）で、sema は左辺が**モジュール束縛**のときだけ受理する（`mod.name`）。

---

## 3. 型構文の提案

### 3.1 前置形

| 意味 | 現行 | 提案 |
|---|---|---|
| ポインタ | `int*` | `*int` |
| 固定長配列 | `int[4]` | `[4]int` |
| 長さ省略配列 | `int[]` | `[]int` |
| ポインタの配列 | `int*[16]` | `[16]*int` |
| 配列へのポインタ | `int[4]*` | `*[4]int` |
| 関数型 | `void(int, int*)` | `fn(int, *int):void`（**要判断 Q2**） |
| 関数型の配列 | `void(int)[]` | `[]fn(int):void` |
| キャスト（数値変換） | `<sint>x` | `x as sint`（§3.5、Q3 決定） |
| キャスト（ビット読み替え） | `<uint16*>p`, `<uint16>f` | `bitcast<*uint16>(p)`, `bitcast<uint16>(f)` |

読み方は Go と同じで左から右に「16 個の、〜へのポインタの、int」。

**関数型（Q2）**: 候補は
- (a) `fn(int, *int):void` — 関数宣言 `function f(a:int):void` と同じ「引数 → `:` → 戻り値」の順で、Zig の `fn(a: T) R` にも近い。`fastcall` 属性は `fn(int):void fastcall`? → 属性は型に**含めない**方向で別途（§3.4）
- (b) `function(int, *int):void` — 長い
- (c) 現行の `void(int, *int)` を残す — 「戻り値が前」は C の関数ポインタ由来で、前置形と混ぜると `[]void(int)` のように読みにくい

推奨 (a)。`fn` を予約語にする（識別子としての使用はコーパスに 0）。

**キャスト**: §3.5。

### 3.5 キャスト: `x as T` と `bitcast<T>(x)`（Q3 決定）

現行の `<T>x` は値を変換せず**型ラベルを貼り替えるだけ**（`CastedValue`。足りない上位バイトは 0）。
`<int16>x` で `x:sint8 = -1` は 255 になる。一方、暗黙の変換（代入・引数）は符号拡張つきの数値変換で、
fc には既に 2 種類の変換がある。明示キャストも 2 つに分ける。**よく使う安全な方は短く、まれで危険な方は目立たせる**
（Rust の `as` / `transmute` と同じ非対称）:

| 表記 | 意味 | 規則 | castle での頻度 |
|---|---|---|---|
| `x as T` | 数値変換（値を保つ） | 整数 → 整数: 拡張は元が符号付きなら符号拡張、縮小は下位バイト、同サイズの符号違いはビットそのまま。配列 → 要素型のポインタ。それ以外はエラー | 22 / 26 |
| `bitcast<T>(x)` | ビット読み替え | サイズが同じもの同士: ポインタ ↔ ポインタ、ポインタ ↔ `uint16`/`int16`、関数ポインタ ↔ `uint16`、同サイズの整数同士。配列はポインタ（2 バイト）とみなす。サイズが違えばエラー | 4 / 26 |

```
x -= (giff as sint8 + speed) / 16;        // as は単項演算子の次の優先順位 (Rust と同じ)。a + b as int は a + (b as int)
var addr = bitcast<*int>(BASE + offset);  // まれなので長くてよい。grep でも見つけやすい
```

- `as` は予約済み（`use mod as m`）。`bitcast` `fn` を予約語に追加（コーパスで識別子としての使用は 0）
- 文法: `exp kAS type`（優先順位は単項の次）、`kBITCAST '<' type '>' '(' exp ')'`（`bitcast` が予約語なので比較の `<` と衝突しない）
- `<T>x` は v2 で廃止。`fcc migrate` は sema で記録した型情報で `as` / `bitcast` に振り分ける
  （整数同士・配列 → 同じ要素型のポインタは `as`、それ以外は `bitcast`）。生成コードは不変
- `x as int16` が符号拡張になるのは現行からの意図的な変更（コーパスに該当なし）

### 3.2 文法

```
type: IDENT                          // 名前付き型 (int, uint8, ..., struct 名)
    | '*' type                       // ポインタ
    | '[' exp ']' type               // 固定長配列
    | '[' ']' type                   // 長さ省略配列
    | kFN '(' param_types ')' ':' type   // 関数型
    | kFASTCALL type                     // fastcall 属性つき関数型 (fastcall fn(int):void)
```

右再帰の前置形。LALR での衝突は無い:
- 型が現れる位置は `:` の後（宣言・引数・戻り値）、`<` の中（キャスト）、`fn(...)` の中と `:` の後、`->` の後（ラムダ）だけで、
  式の `[` `*` とは文脈が分かれる（`var a:[4]int` の `[` は型、`var a = [1, 2]` の `[` はリテラル）
- ラムダ `->fn(a:int):int { ... }`: 戻り値の型の直後に `{` が来る。型の中で `{` は使わないので衝突しない
  （struct リテラルを `Point{...}` にしても、型文脈の `IDENT` の後の `{` はブロックにしか読めない）
- `<*uint16>p`: `<` の直後の `*` は型の先頭。`a < *p`（比較とデリファレンス）は式文脈なので別

### 3.3 バージョン（Q1 決定: v2 に含める）

型構文の変更は破壊的。選択肢:
- (a) **v2 に含める**（v2 の定義を今のうちに変えて、リポジトリ内を再移行）。実プロジェクトはまだ v1 なので、
      利用者の移行は 1 回で済む。v2 を公開した直後の今が最後の機会
- (b) v3 にする。v2 と v3 のゲートが増え、リポジトリ内の v2 ファイルを v3 に再移行する

(a) に決定。`fcc migrate` は v2 ファイルにも「後置 → 前置」「`<T>x` → `as`/`bitcast`」の書き換えを適用する
（for のときと同じ仕組み）。生成コードは変わらない（型の意味は同じ）。

### 3.4 ついでに決めたいこと

- `fastcall` は現行では**型の一部**（`fastcall void(uint8)`）で、呼び出し規約は呼び出される式の型で決まる
  （関数ポインタ経由の呼び出しも）。型から外すと fastcall 関数のアドレスを通常の関数型の変数に入れて呼べてしまうので、
  **型に残す**（Q4 決定）。表記は前置属性 `fastcall fn(int):void`。宣言側は今までどおり `options(fastcall: true)`
- `int` という名前が `uint8` の別名なのは紛らわしい（memo.txt にも `int` の整理がある）。今回は触らない

---

## 4. struct の提案

### 4.1 宣言

```
struct Point {
	x:int;
	y:int;
}

public struct Enemy {
	px:int;
	py:int;
	vx:sint;
	vy:sint;
	pos:Point;          // ネスト
	name:*int;          // ポインタ
	flags:[4]int;       // 配列
}
```

- モジュールのトップレベルだけ（関数内は不可）。`public` で他モジュールから見える（型は `mod.Point` でも参照できる: 要判断 Q5）
- フィールドは宣言順に詰めて配置（アラインメントなし）。サイズ = フィールドの合計
- 再帰（`struct A { next:*A; }`）は許す（ポインタ経由のみ。直接の自己包含はエラー）
- 型としての名前は識別子と別空間ではなく、**同じスコープ**に置く（`struct Point` と `var Point` は衝突）

### 4.2 使い方

```
var p:Point;
p.x = 1;                      // フィールド代入
var q:*Point = &p;
q.x = 2;                      // ポインタ経由も同じ `.`（Go と同じ自動デリファレンス）
var es:[16]Enemy;
es[i].px += es[i].vx;         // 配列の要素のフィールド
es[i].pos.y = 0;              // ネスト
var e:*Enemy = &es[i];        // 要素のアドレス
sizeof(Enemy)                 // 定数 (要判断 Q6: 組み込み `sizeof` を入れる)
```

- `p.x` の `.` は既存の二項演算子。sema で左辺の型を見て、モジュール参照かフィールド参照かを決める
- **値としてのコピー**（`p = q;`、構造体の引数・戻り値）は**可**（Q7 決定）。サイズ分のバイトコピー
  （小さければ展開、大きければループ）。引数はスタックフレームにコピーされるので大きな構造体ほど遅いことは
  リファレンスに書く。fastcall 関数の引数はレジスタ領域 16 バイトに収まる必要がある
- 構造体のポインタは関数の引数・戻り値・配列要素に使える

### 4.3 リテラル

```
var p = Point{x: 1, y: 2};                 // 名前付き（順不同、省略したフィールドは 0）
const ORIGIN = Point{0, 0};                // 位置指定
const PTS:[2]Point = [Point{1, 2}, Point{3, 4}];
const PTS2:[2]Point = [{1, 2}, {3, 4}];    // 要素型が分かっていれば型名省略（要判断 Q8）
```

- 定数（トップレベルの `const`）はデータブロックとして ROM に置く（配列リテラルと同じ経路）
- 関数内の `var p = Point{...}` はフィールドごとの代入に脱糖する
- 文法: `IDENT '{' field_list '}'`。`exp '(' ... ')' opt_block`（マクロの後置ブロック）との衝突は、
  struct リテラルが `IDENT` 直後の `{`、後置ブロックが `)` 直後の `{` なので起きない。
  ただし `if (x) {` のように**式の直後に `{` が来る文**では、`if (p) {` の `p` を struct リテラルの
  始まりと誤読しうる → 現行の文法では条件は必ず `( )` で囲むので `)` の後になり衝突しない（Go が
  `if x == T{} {` を禁止しているのと同じ問題は、fc では構文上起きない）

### 4.4 コード生成

- フィールド参照 `p.x` は「`p` のアドレス + オフセット」。`p` がグローバル/フレーム上の変数なら
  `sym+off` / `S+off,x` で直接アクセス、ポインタ経由なら `ldy #off; lda (ptr),y`
- 配列要素 `es[i].px`: 要素サイズ × i の計算が要る。6502 には乗算が無いので
  - サイズが 2 の冪なら `asl` で
  - それ以外は 8bit × 8bit の乗算ルーチン（`share/runtime.asm` に追加。math.fc の乗算表を使う手もある）。
    現行の `loadYIdx` は要素サイズ 2 以外を正しく扱えていない（サイズ N に対して `asl` を N−1 回 = ×2^(N−1)）ので、
    これを機に「2 の冪はシフト、それ以外は乗算」に直す
  - インデックスの 2 バイト化（要素サイズ × 添字が 255 を超える配列）は今回も対象外
- IR: `Op{Code: OpIndex}` は要素サイズを型から取る（変更なし）。フィールドアクセスは
  `OpRef`（アドレス）+ 定数オフセットの加算で表せるが、ゼロページ間接の `ldy #off` を出したいので
  新オペレータ `OpField{Dst, Src: base, Offset}` を足す方が素直
- `types.Type` に `Kind: Struct`、`Fields []Field{Name, Type, Offset}` を追加。名前 `struct Point` でインターン
  （ir ダンプに出るので golden は struct を使うテストだけ変わる）
- `ModuleInterface` に型の輸出を追加（F-mod のシリアライズ対象にもなる）

### 4.5 SoA コンテナ `soa`（S1〜S5 決定）

6502 では「フィールドごとに配列を分け、要素をインデックスで指す」（structure of arrays）方が
`ldy idx; lda Field,y` で触れて圧倒的に速い。castle の `en.px[i]` / `en.py[i]` … の束（`en.fc` に 13 本）は
これを手で書いているもの。構造体の定義を共有したまま、レイアウトだけ SoA にしたコンテナを宣言できるようにする:

```
struct Point {
	x:int;
	y:int;
}

soa Points:[4]Point;                 // SoA コンテナ。型でもあり唯一の実体でもある (型と実体が 1 対 1)。
                                     // メモリは x0,x1,x2,x3, y0,y1,y2,y3 (フィールドごとに Points_x, Points_y のシンボル)
                                     // options(segment: "BSS_EX") 可

var p:*Points;                       // 要素ハンドル。実体は uint8 のインデックス (sizeof = 1)
p = 0 as *Points;
p.x = 1;                             // ldy p ; lda #1 ; sta Points_x,y
p.y = 2;
p++;                                 // インデックス +1。p += n、p == q、p < q もインデックスの演算
Points[2].y = 5;                     // 直接添字 (Points 自体が配列のように振る舞う)

function move(e:*Points):void { e.x += 1; }   // ハンドルは 1 バイトで渡せる

var pt:Point = *p;                   // gather: 要素全体を通常の Point にコピー
*p = pt;                             // scatter

soa const TABLE:[8]Point = [{1, 2}, {3, 4}, ...];   // ROM に転置して置く (lookup table)
```

- 要素数 ≤ 256（Y レジスタで添字するため）。範囲外は検査しない
- 2 バイト以上のフィールド（`int16`、通常ポインタ）はバイトごとの配列に分ける（`Points_v_0`, `Points_v_1`。
  `lda Points_v_0,y` / `lda Points_v_1,y`）
- ネストした struct フィールドは平坦化（`pos.x` → `Points_pos_x`）。**配列フィールドは禁止**（S5）
- `*Points` はポインタの記法だが 2 バイトのポインタとは別の型。通常のポインタとの相互変換は `bitcast` でも不可
- `soa const` は初期値を転置してデータブロックにする。const の要素は読み出しのみ
- 型の表示名は `soa [4]Point`、ハンドルは `*Points`。`ModuleInterface` の型輸出の対象（`mod.Points`、`use Points from mod`）
- 類似機能を持つ言語: Odin の `#soa[4]Point`（`#soa` ポインタも「配列の先頭 + インデックス」で、この設計に最も近い）、
  ISPC の `soa<8> struct`、Jai の初期の `SOA` キーワード（後に削除）、Zig の `std.MultiArrayList`（ライブラリ）。
  8/16 ビット向けコンパイラ（cc65, KickC, Millfork, Prog8）には無く、手書きアセンブラの定番を言語に持ち込む形になる

### 4.6 やらないこと（今回）

- メソッド、埋め込み、共用体、ビットフィールド、アラインメント指定
- 構造体の比較 `==`
- 匿名構造体

---

## 5. 移行

- 型構文: `fcc migrate` の書き換え規則を 1 つ足す（AST の型ノードは意味で持っているので、印字順を変えるだけ。
  `ArrayType{Elem}` / `PointerType{Elem}` / `FuncType{Result, Params}` はそのまま、プリンタが前置で出す）。
  v2 ファイルにも適用（for のときと同じ）。生成コードは不変で、マイグレータ自身が検証する
- struct は追加機能なので移行は不要。castle の `en.fc` の配列群を struct に書き換えるかは castle 側の判断

---

## 6. 実装計画

| # | 作業 | 目安 |
|---|---|---|
| 1 | 型構文: 文法（前置形、`fn`、`fastcall fn`）、`as` / `bitcast`、プリンタ、ゲート（v2 で後置形・`<T>x` はエラー）、`fcc migrate` の規則（キャストは型で振り分け）、リポジトリ内の再移行、リファレンス更新。型の表示名を前置形にする（ir/allocir golden の型名が変わる） | 1.5 日 |
| 2 | （Q4 決定により不要） | — |
| 3 | struct 前半: 型（`types.Struct`）、宣言、フィールド参照（変数・ポインタ・ネスト）、`sizeof` | 1.5 日 |
| 4 | struct 後半: 配列要素（要素サイズの乗算）、リテラル、定数データブロック、値コピー（代入・引数・戻り値）、モジュール間の型の輸出 | 2 日 |
| 5 | `soa`: コンテナ宣言（var / const）、ハンドル型、フィールド参照のコード生成（`Field,y`）、直接添字、インデックス演算、gather/scatter、転置データブロック | 2 日 |
| 6 | テスト（sema / codegen / emu 実行）、castle の `en.fc` の配列群を `soa` で書いてみる試験（examples ではなく test） | 0.5 日 |

合計 7.5 日。1 / 3〜4 / 5 は順に独立コミットできる。

---

## 7. 例（提案どおりに書いたとき）

```
#fc 2
use * from stdio;

public struct Enemy {
	px:int;
	py:int;
	vx:sint;
	vy:sint;
	process:fn(*Enemy):void;
}

var enemies:[16]Enemy;
var names:[16]*int;
const TABLE:[]fn(*Enemy):void = [none_process, slime_process];

function move(e:*Enemy):void
{
	e.px += <int>e.vx;
	e.py += <int>e.vy;
}

function main():void
{
	for (var i = 0; i < 16; i++) {
		enemies[i].process(&enemies[i]);
	}
	exit(0);
}
```

---

## 8. 要判断

- [x] **Q1** バージョン → **v2 に含めて再移行**
- [x] **Q2** 関数型の表記 → **`fn(int, *int):void`**
- [x] **Q3** キャスト → **`x as T`（数値変換）と `bitcast<T>(x)`（ビット読み替え）。`<T>x` は廃止**（§3.5）
- [x] **Q4** `fastcall` → **型に残す**（表記 `fastcall fn(int):void`）
- [x] **Q5** struct 型は他モジュールから `mod.Point` / `use Point from mod` で参照 → **できる**
- [x] **Q6** `sizeof(T)` / `sizeof(expr)` → **入れる**
- [x] **Q7** struct の値コピー（代入・引数・戻り値） → **可**
- [x] **Q8** struct リテラル → 名前付き・位置指定の両方、要素型が分かる文脈では型名省略可
- [x] **S1** SoA のキーワード → **`soa`**
- [x] **S2** 宣言の形 → **`soa Points:[4]Point;`**
- [x] **S3** ハンドルの表記 → **`*Points`**（実体は uint8）
- [x] **S4** `soa const`（ROM に転置） → **同時に入れる**
- [x] **S5** SoA の要素の配列フィールド → **禁止**
