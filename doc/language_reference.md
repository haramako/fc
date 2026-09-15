# FC 言語リファレンス（文法 v2）

FC は NES（6502）向けの静的型付き言語で、C 風の構文を持つ。コンパイラ `fcc` は FC ソースを
ca65 アセンブリに変換し、ld65 でリンクして NES ROM（`-t nes`）または内蔵エミュレータ用バイナリ
（`-t emu`、既定）を作る。

本書は **文法 v2**（ファイル先頭に `#fc 2`）を正典として記述する。v1（宣言なし）との差は
末尾の「v1 との違い」にまとめた。
「注」として記す。

---

## 1. ソースファイルとモジュール

- 1 ファイル = 1 モジュール。モジュール名はファイル名から `.fc` を除いたもの（`stdio.fc` → `stdio`）
- 先頭行に `#fc 2` を書くと文法 v2。無ければ v1 として読まれる
- ファイルは UTF-8。文字列リテラルの中にそのまま日本語を書ける（`textmap` で文字コードに変換できる）
- コメントは `// ...`（行末まで）と `/* ... */`

```
#fc 2
// main.fc
use * from stdio;

function main():void
{
	print("hello\n");
	exit(0);
}
```

### 1.1 `use` — モジュールの参照

```
use mod;                 // モジュール束縛。mod.name で参照する
use mod as m;            // 別名で束縛
use * from mod;          // mod の public 宣言を非修飾名で取り込む（glob）
use a, b from mod;       // mod の public 宣言 a, b だけを非修飾名で取り込む（選択的インポート）
public use mod;          // 束縛を再輸出する（このモジュールを glob した側にも mod が見える）
public use * from mod;   // glob 取り込みを再輸出する
public use a from mod;   // 選択的インポートを再輸出する
```

相互 `use`（A が B を use し、B が A を use する）は許される。

### 1.2 可視性

宣言（`var` / `const` / `function`）と `use` は**デフォルトで private**（モジュール内だけで見える）。
他モジュールから見せるものには `public` を付ける。

```
public var score:int;
public const MAX = 16;
public function reset():void { ... }
```

- `mod.name`（ドット参照）と `use * from mod` / `use a from mod` は **public な宣言だけ**に届く。
  private を指すと `mod.name is private` エラー
- `use X;` / `use * from X;` は既定では再輸出されない。`public use` で明示する

### 1.3 名前解決

非修飾名は次の順で探す: 自モジュールの宣言 → 選択的インポート → glob 取り込み（`use` の順）→ 組み込み。

- 自宣言と glob 取り込みが同名なら自宣言が勝つ（glob は弱い束縛）
- 自宣言と選択的インポートが同名ならエラー（`already imported` / `already defined`）
- 関数内では、ブロックに入るごとにスコープができ、内側の宣言が外側を隠す

### 1.4 `include` — アセンブラと CHR データ

```
include("util.asm");      // ca65 ソースをこのモジュールのアセンブラ出力に取り込む（.asm / .inc）
include("font.chr");      // CHR データ（拡張子で判定）
```

ファイルは検索パス（ソースのディレクトリ → `fclib/` → `fclib/<target>/`）から探す。

### 1.5 `options` — モジュール属性

ファイルの先頭付近に書く。

```
options(bank: -1);            // 配置バンク（負数は末尾から）
options(org: 0xa000);         // 配置アドレス
options(mapper: "MMC3");      // iNES マッパ（"MMC0" / "MMC3" / 番号）— メインモジュールで
options(bank_count: 4);       // PRG バンク数 — メインモジュールで
options(char_banks: 1);       // CHR バンク数 — メインモジュールで
options(fastcall_reg: 32);    // extern の fastcall 関数が使うゼロページ領域 FC_FASTCALL_REG の大きさ（既定 32、16〜128）— メインモジュールで
options(static_zp: 64);       // 静的フレーム（§4.5）のゼロページ側 FC_SZP の大きさ（既定 64、0〜256）— メインモジュールで
options(static_ram: 512);     // 静的フレームの RAM 側 FC_SRAM の大きさ（既定 512、0〜8192）— メインモジュールで
options(farcall: true);       // far call（§4.4）を有効にする — メインモジュールで
options(near: true);          // このモジュールは常にマップされている扱い（far call の対象にしない）
```

`bank: N` は fc が ld65.cfg を生成する構成では配置先のバンク。自前の ld65.cfg を使う構成では配置に使われないが、
far call の判定に「N ≥ 0 なら切替バンク、無しか負なら固定バンク」として使われる（§4.4）。

---

## 2. 型

| 型 | サイズ | 範囲 | 備考 |
|---|---|---|---|
| `int` / `uint` / `int8` / `uint8` | 1 | 0〜255 | `int` は `uint8` の別名 |
| `sint` / `sint8` | 1 | -128〜127 | |
| `int16` / `uint16` | 2 | 0〜65535 | |
| `sint16` | 2 | -32768〜32767 | |
| `bool` | 1 | | `true` / `false`（v2）の型。整数（`uint8`）と互換で、`if` の条件や `&` にそのまま使える。比較演算の結果は今のところ `uint8`（`bool` に変える予定） |
| `void` | 0 | | 戻り値なし |
| `[N]T` | N×size | | 配列。`[]T` は長さ省略（初期値か `address` から決まる） |
| `*T` | 2 | | ポインタ。`null`（v2）を入れられる（ポインタと関数ポインタのみ。SoA ハンドルには null なし） |
| `*void` | 2 | | 何を指すか問わないポインタ（v2）。どのポインタ・関数ポインタ・配列も暗黙に入る。戻すには `bitcast<*T>(p)`。参照はがし・添字・算術は不可 |
| `fn(T1, T2, ...):R` | 2 | | 関数型（値は関数のアドレス）。`fastcall` 属性は型の一部（表示名 `fastcall fn(...):R`） |
| `Name` / `mod.Name` | フィールドの合計 | | struct（§2.1）。他モジュールの public な struct は `mod.Name`、または `use Name from mod;` |
| `*Name`（Name は `soa`） | 1 | | SoA コンテナの要素ハンドル（§2.2）。2 バイトのポインタとは別物 |

型は Go / Zig と同じく**前置**で、左から右に読む: `[16]*int` は「16 個の、int へのポインタ」、`*[4]int` は
「4 個の int の配列へのポインタ」、`[]fn(int):void` は「`fn(int):void` の配列」。

- 整数リテラルの型は値で決まる: 0〜255 → `uint8`、256 以上 → `uint16`、-1〜-128 → `sint8`、-129 以下 → `sint16`
  （`-128` は `sint8` に収まるので `sint8`）
- 型変換は明示的に書く。2 種類ある（§6.1）: `x as T`（数値変換）と `bitcast<T>(x)`（ビットの読み替え）。
  サイズが合わない代入はエラー
- 配列は暗黙にポインタへ変換される（`[]int` を `*int` の引数に渡せる）
- `sizeof(T)` は型のバイト数（定数）。`sizeof(x)` と変数名を書けばその変数の型のサイズ

### 2.1 struct

```
struct Point {
	x:int;
	y:int16;
}
struct Node {
	value:int;
	next:*Node;                 // 自分自身へのポインタは可（値としての自己参照は不可）
}

var g:Point;                    // フィールドは宣言順に詰めて置かれる（x が +0、y が +1。アラインメントなし）
var p:Point = {10, 20};         // 位置指定のリテラル（宣言に型があるので型名を省ける）
var q = Point{x: 1, y: 2};      // 名前付きのリテラル（省いたフィールドは 0）
const ORIGIN = Point{x: 0, y: 0};
const TABLE:[3]Point = [{1, 100}, {2, 200}, Point{x: 3, y: 300}];   // ROM 上のデータ（要素ごとに .byte / .word）
```

- 宣言はモジュールのトップレベルにだけ書ける。`public struct` で他モジュールから見える
- `p.x` でフィールドを読み書きする。`pp:*Point` なら `pp.x` で自動的に参照はがし（`(*pp).x` と同じ）。
  入れ子（`ln.a.x`）、配列要素（`arr[i].y`）も同じ
- 値コピー: 代入 `a = b`、引数（値渡し）、戻り値はフィールド全体をコピーする。比較 `==` はない
- リテラルの型名は、宣言の型・代入先・`return`・配列リテラルの要素・フィールドの型から決まる文脈で省ける（`{1, 2}`）。
  全項目が定数ならデータブロック（ROM）になり、そうでなければ実行時にフレーム上に組み立てる
- struct の配列の添字は要素サイズを掛ける（サイズが 1・2 以外なら 16 ビットのシフト加算）
- コード: 変数のフィールドは `sym+off` / `S+addr+off,x` で直接触る。ポインタ経由は `ldy #off; lda (reg),y`

### 2.2 soa — SoA コンテナ

6502 では「フィールドごとに配列を分けて添字で触る」（structure of arrays）方が `ldy i; lda Field,y` で速い。
`soa` は struct の定義を共有したまま、そのレイアウトで置いたコンテナを宣言する:

```
soa Enemies:[8]Enemy;                        // Enemies_px, Enemies_py, ... の配列（N ≤ 256）。options(segment: "...") 可
soa const TABLE:[3]Enemy = [{...}, ...];     // 初期値を転置して ROM に置く（読み出しのみ）

var e:*Enemies = &Enemies[2];                // 要素ハンドル（1 バイトのインデックス）
e.px = 10;                                   // ldy e; lda #10; sta Enemies_px,y
e.hp -= 1;                                   // 2 バイト以上のフィールドはバイトごとの配列（Enemies_hp_0 / _1）
Enemies[3].py = 5;                           // 直接添字
e++;  e += 2;  e == f;  e < f;               // ハンドルの算術・比較はインデックスのもの。`e as int`、`2 as *Enemies`
var v:Enemy = Enemies[2];  Enemies[3] = v;   // 要素全体のコピー（gather / scatter）。`*e` でも同じ
function move(e:*Enemies):void { ... }       // ハンドルは 1 バイトで渡せる
```

- `Enemies[i]` は要素（左辺値）、`&Enemies[i]` はハンドル、`*e` はハンドルの指す要素。入れ子の struct フィールド（`e.pos`）も要素として扱える
- 配列フィールドを持つ struct は `soa` にできない。2 バイト以上のフィールドのアドレス（`&e.hp`）は取れない
- 他モジュールからは `mod.Enemies[i]`、`*mod.Enemies`、`use Enemies from mod;`（`public soa` のとき）

---

## 3. リテラル

```
123      0x7f      0b1010   1_000  // 10 進 / 16 進 / 2 進（桁の間の '_' は区切り）
"text"   'text'                    // 文字列（どちらも同じ。末尾に終端の 0 が付く）
"\n"     "\x41"                   // エスケープは \n と \xNN（16 進 2 桁）だけ。他の \ はそのまま
[1, 2, 3]   [1, 2, 3,]             // 配列リテラル（定数）。末尾のカンマは可（呼び出しの引数も）
incbin("data.bin")                 // ファイルの中身を配列定数として埋め込む
true  false                        // bool（v2）
null                               // ポインタ / 関数ポインタの 0（v2）。型は代入先・比較相手・引数から決まる（`null as *T` も可）
->int(a:int) { return a + 1; }     // ラムダ（無名関数）
```

文字列は終端 0 付きの `[]uint8` 定数（ポインタ `*uint8` としても使える）。

---

## 4. 宣言

### 4.1 変数・定数

```
var x:int;                     // 型指定
var y = 10;                    // 型推論（初期値の型）
var a:[16]int;                 // 配列
var p:*int = &x;               // 初期化は関数内のみ（グローバル変数は初期化できない）
const N = 4;                   // 定数（コンパイル時に確定）
const T:[]int = [1, 2, 3];     // 配列定数（ROM に置かれる）
const S:*int = "text";         // ポインタ型で宣言しても配列定数（データ自体に名前が付く）
var a:int, b:int;              // まとめて宣言
```

グローバル変数の属性:

```
var vram:int options(address: 0x2007);        // 固定アドレス（メモリマップド I/O）
var buf:[256]int options(segment: "BSS_EX");  // 配置セグメント
const REG:int options(address: "_reg_sym");   // アセンブラのシンボルに束縛（値なし const）
```

### 4.2 関数

```
function add(a:int, b:int):int
{
	return a + b;
}
function tiny():void options(fastcall: true) { ... }   // 高速呼び出し規約（下記）
function interrupt():void options(symbol: "_interrupt"); // 本体なし: アセンブラ側の定義を参照
function f():void options(segment: "game") { ... }     // 配置セグメント
```

- 引数・戻り値の型は必須。`void` 関数は `return;`、それ以外は `return expr;`。
  非 void 関数は本体が必ず `return` で終わらなければならない（最後の文が `return`、両枝が `return` で終わる
  `if`/`else`、`break` の無い `loop`・`while (1)`・`for (;;)`、`default` 付きで全 case が `return` で終わる `switch`）
- `fastcall`: **本体を持つ関数では意味を持たない**（非再帰の関数は全部静的フレーム §4.5 になる。互換のため受理する。
  以前の「中から他の関数を呼べない」制限も無い）。本体の無い extern 関数（asm 定義）に付けると、引数・戻り値を
  ゼロページの `FC_FASTCALL_REG`（既定 32 バイト、`options(fastcall_reg: N)`）で渡す規約になる。
  base.asm を自前で持つプロジェクトは `FC_FASTCALL_REG: .res N` と `FC_FASTCALL_REG_SIZE = N`（`.export … : absolute`）を
  合わせる（不足はリンク時の `.assert` で検出される）
- `options(abi: "stack")`: 静的フレームにせず、スタック（`S+n,x`）の規約のままにする（§4.5）
- `options(interrupt: true)`: 割り込みハンドラから呼ばれる関数（§4.5）
- `options(zeropage: false)`: 静的フレームを RAM 側に置く
- `options(symbol: "...")`: 生成するシンボル名を固定する（割り込みベクタなど）
- `options(near: true)`: far call（§4.4）の対象にしない（呼ぶ側はマップ済みと仮定して `jsr` する）

### 4.4 far call（バンクをまたぐ呼び出し）

### 4.5 呼び出し規約と静的フレーム

fc で本体を持つ関数のうち**再帰しないもの**は、引数・戻り値・ローカルを固定アドレスのフレーム（`F_<sym>`）に持つ。
同時に活性になりえない関数（呼び出しグラフで一方から他方へ届かない関数どうし）のフレームは重ねて置かれるので、
使う領域は「呼び出しの連鎖 1 本分の合計」程度で済む。領域はゼロページ側 `FC_SZP`（`options(static_zp: N)`。
呼び出しの深い関数から順に入るだけ入る）と RAM 側 `FC_SRAM`（`options(static_ram: N)`）。
base.asm を自前で持つプロジェクトは `FC_SZP: .res N` / `FC_SRAM: .res M` と `FC_SZP_SIZE` / `FC_SRAM_SIZE` の
`.export … : absolute` を合わせる（不足はリンク時の `.assert` で検出される。配置は `.fc-build/_frames.inc`）。

| 種類 | 対象 | 引数の渡し方 |
|---|---|---|
| static | 本体を持つ非再帰の関数（既定） | 呼び出し側が `F_g+k` に直接書き `jsr` |
| entry | static のうち、アドレスを取られた関数（関数ポインタ・`const` の表・インラインアセンブラからの参照）と `options(interrupt: true)` | スタック経由（下の stack と同じ）。プロローグで自分のフレームに写す |
| stack | 再帰する関数、`options(abi: "stack")`、本体の無い extern 関数 | X が指すスタック `S+k,x`。呼び出し側が stack 関数なら X を進める `call` マクロ |
| fastcall | extern で `fastcall` 指定 | `FC_FASTCALL_REG` |

再帰の判定は呼び出しグラフの閉路で、関数ポインタ経由の呼び出しは「同じ関数型でアドレスを取られた関数の全部」
への呼び出しとみなす。`bitcast` で関数ポインタの型を変えて呼ぶ再帰は検出できない（`options(abi: "stack")` を付ける）。
`options(interrupt: true)` の関数から届く関数は全部 static でなければならず（X が何を指すか分からないため）、
そのフレームは他のどの関数とも重ねない。

### 4.6 far call

MMC3 のように PRG が切替バンクに分かれているとき、メインモジュールに `options(farcall: true)` を書くと、
**切替バンク（`options(bank: N)`、N ≥ 0）にある別モジュールの関数**への呼び出しを、コンパイラが自動で
`farcall` トランポリン経由にする。呼ぶ側の書き方は普通の呼び出しと同じで、引数・戻り値の渡し方も変わらない。

```
options(bank: 8);                              // bg_mmc.fc: 切替バンク
public function fetch_area(area:int, dir:int):void { ... }

bg_mmc.fetch_area(a, d);                        // 固定バンクから: farcall 経由 (バンクを切り替えて呼び、戻す)
```

- 呼び先が固定バンク（`bank` 無しか負）、同じモジュール内、または `options(near: true)` の関数 → 今までどおり `jsr`
- バンク番号はコンパイラでなく ld65 が決める: `ld65.cfg` の MEMORY に `bank = N`（マッパーに書く値）を付ける
  （fc が cfg を生成する構成では自動）。生成コードは `.bank(シンボル)` で参照する
- `farcall` はターゲット側が asm で用意する（`fclib/nes/farcall_mmc3.asm` が MMC3 の参考実装。emu と MMC0 は fc が
  「そのまま飛ぶ」だけのものを用意する）。規約: `FC_FARCALL`（3 バイト: 呼び先アドレス、バンク）を読み、X と
  FC_FASTCALL_REG を壊さない。参考実装は同じバンクが既に入っていれば切り替えずに飛ぶ（+37 サイクル）、
  違えば退避・切替・復帰する（+100 サイクル程度）
- 関数ポインタ経由の呼び出しは対象外（バンクは呼ぶ側の責任）。他バンクのデータ参照も対象外
- `fcc build -d` で far call になった箇所の一覧が出る

設計の経緯は [v2_farcall.md](v2_farcall.md)。

### 4.3 ラムダ

```
const add2 = ->int(a:int) { return a + 2; };
var cb:void() = ->void() { ... };
cb();
```

---

## 5. 文

```
expr;                                  // 式文
var ... / const ...                    // 関数内の宣言（ブロックスコープ）
{ ... }                                // ブロック
;                                      // 空文

if (cond) stmt [elsif (cond) stmt]... [else stmt]
while (cond) stmt
loop { ... }                           // 無限ループ（break で抜ける）
for (init; cond; step) { ... }         // C 型。init は var 宣言か式、step は式か ++/--。各部は省略可
x++;  x--;  ++x;  --x;                 // x = x + 1 / x = x - 1（文としてのみ。式の値は持たない）
switch (expr) {
case 1, 2:                             // 複数の値を列挙できる
	...                                // fallthrough しない（case の終わりで switch を抜ける）
case 3:
	...
default:
	...
}
break;        continue;                // 最も内側のループ（break は switch も）を抜ける / 次の繰り返しへ（for では step に進む）
break L;      continue L;              // ラベル付き
return [expr];
```

### 5.1 `for`

```
for (var i = 0; i < 10; i++) { ... }        // i のスコープは for の中だけ。型は初期値から（0 → int）
for (var i:sint = -5; i < 5; i++) { ... }   // 型指定
for (i = 0; i < n; i += 2) { ... }          // 既存の変数
for (;;) { ... }                            // 無限ループ
```

意味は `{ init; while (cond) { body; step; } }` と同じで、`continue` は step に進む。
`i++` は `i = i + 1` の略記で、`for` の step 以外では文としてだけ書ける（`a = i++` は不可）。

### 5.2 文ラベル

`loop` / `while` / `for` / `switch` にはラベルを付けられる。`break L;` はラベルの文を抜け、
`continue L;` はラベルのループの次の繰り返しに進む（switch に `continue` はできない）。

```
outer: loop {
	switch (x) {
	case 1:
		break;          // switch を抜ける
	case 2:
		break outer;    // ループを抜ける
	}
}
```

### 5.3 条件

条件式には整数も書ける（0 が偽）。`&&` / `||` は短絡評価する。

---

## 6. 式と演算子

優先順位（高い順）:

| 演算子 | 意味 |
|---|---|
| `.` `()` `[]` | フィールド参照 / モジュール参照、呼び出し、添字 |
| `!` `~` `-` `+` `*` `&` | 前置: 論理否定、ビット反転、符号、デリファレンス、アドレス |
| `*` `/` `%` | 乗除。除算・剰余は床除算（商は負の無限大方向に丸め、余りの符号は除数に合わせる。Python と同じ）。8 ビット・16 ビット、符号の有無すべて同じ規則（2 のべき乗は算術シフト / マスクに畳まれる） |
| `+` `-` | 加減 |
| `<<` `>>` | シフト（2 バイト値のシフト量は定数のみ） |
| `<` `>` `<=` `>=` | 比較 |
| `==` `!=` | 等値 |
| `&` | ビット AND |
| `^` | ビット XOR |
| `\|` | ビット OR |
| `&&` | 論理 AND（短絡） |
| `\|\|` | 論理 OR（短絡） |
| `=` `+=` `-=` `*=` `/=` `%=` `&=` `\|=` `^=` `<<=` `>>=` | 代入・複合代入（右結合）。`a op= b` は `a = a op b` |

- `a[i]`: 配列/ポインタの添字。添字は 1 バイト（2 バイトの添字は不可）
- `*p` / `&x`: ポインタ演算
- `mod.name`: モジュールの public 宣言。`x.name`: struct のフィールド（§2.1）
- `sizeof(T)`: 型のサイズ（定数）
- `f(args)`: 関数呼び出し。引数の数と型を検査する
- 定数式（リテラル・`const`・それらの演算）はコンパイル時に畳み込まれる

---

### 6.1 キャスト

| 表記 | 意味 | できること |
|---|---|---|
| `x as T` | 数値変換（値を保つ） | 整数 → 整数（拡張は元が符号付きなら符号拡張、縮小は下位バイト、同サイズの符号違いはビットそのまま）、配列 → 要素型のポインタ |
| `bitcast<T>(x)` | ビットの読み替え | サイズが同じもの同士: ポインタ ↔ ポインタ、ポインタ ↔ `uint16`/`int16`、関数ポインタ ↔ `uint16`、同サイズの整数同士。配列はポインタ（2 バイト）とみなす。整数リテラルはサイズを問わない（`bitcast<*int>(0x2000)`） |

`as` の優先順位は単項演算子の次（`-x as int` は `(-x) as int`、`a + b as int` は `a + (b as int)`）。

```
x -= (giff as sint8 + speed) / 16;
var addr = bitcast<*int>(BASE + offset);
var v:int16 = s as int16;                // s:sint8 = -1 なら -1 (符号拡張)
```

## 7. 組み込み

`include` なしで常に使える。

| 名前 | 働き |
|---|---|
| `min(a, b)` / `max(a, b)` | 小さい方 / 大きい方。型は引数の互換型（片方が符号付きなら符号付き比較、`int16` と `int` なら `int16`）。定数なら畳み込み。関数呼び出しではなく、その場に比較と代入のコードを出す（fastcall 関数の中でも使える） |
| `clamp(x, lo, hi)` | `lo` 以上 `hi` 以下に収める（`x < lo` なら `lo`、`hi < x` なら `hi`）。型・コードは `min` / `max` と同じ |
| `asm("lda #1", "sta $2000")` | インラインアセンブラ（各引数が 1 行） |
| `printf(a, b, ...)` | 引数の型で `stdio.print`（`*uint8`）/ `stdio.print_int16`（整数）を呼び分ける。`stdio` モジュールが必要 |
| `unittest_run_tests()` | `stdio.init()` の後、スコープ内の `test_*` 関数を宣言順に呼び、`stdio.exit(0)` する（`stdio` が必要） |
| `cos(x)` | `math.sin(x + 64)` に展開（`math` モジュールが必要）。`math.cos(x)` とも書ける |
| `incbin("file")` | ファイルを配列定数として埋め込む |
| `textmap("table.txt")` | 文字表を読み、文字列→文字コード配列の変換器（定数）を作る |

### 7.1 `textmap`

```
public const _T = textmap("../tmp/font/text.chr.txt");
...
print(_T("こんにちは"));   // 文字表のコード列 + 終端 0 の []int 定数になる
```

表ファイルは文字を並べたテキストで、先頭から 0, 1, 2, ... のコードが付く。表にない文字は
出現順に末尾へ追加される（同じ表を複数モジュールで使うときは、コードはコンパイル順で決まる）。
ASCII の英数字・記号は全角に、濁点・半濁点付きのかなは基底文字 + 濁点に分解してから変換する。

---

## 8. 標準ライブラリ（fclib）

| モジュール | 主な内容 |
|---|---|
| `stdio`（ターゲット別） | `print(str)`, `print_int16(n)`, `puts(str)`, `exit(code)`, NES では `wait_vsync()`, `ppu_put(...)` |
| `mem` | `set(dst, value, size)`, `copy(dst, src, size)`, `strcpy(dst, src)` など |
| `math` | `sin(x)`, `atan(y, x)`, `rand()`, `sign(i)`, 乗算テーブル |
| `nes`（NES） | PPU / APU / コントローラのレジスタ定義 |
| `pad`（NES） | コントローラ入力 |
| `unittest` | `assert_true(cond, msg)`, `assert_equal(a, b, msg)` |
| `rle` / `lzw` / `inflate` | 圧縮データの展開 |

---

## 9. v1 との違い

v1 のソースは `fcc migrate` で機械的に v2 へ変換できる（[v2_grammar.md](v2_grammar.md) §5）。

| | v1 | v2 |
|---|---|---|
| バージョン宣言 | なし | 先頭行 `#fc 2` |
| 宣言のデフォルト可視性 | public | **private** |
| `public:` / `private:` ラベル | 以降のデフォルトを切り替える | 廃止（宣言ごとに `public`） |
| `mod.name` | private にも届く | public のみ |
| `use X;` / `use * from X;` の再輸出 | 常に再輸出される | `public use` のときだけ |
| 選択的インポート `use a, b from mod;` | なし | あり |
| `loop` | `loop() stmt` | `loop { ... }` |
| `for` | `for (i, 0, n) { ... }`（`continue` がインクリメントを飛ばす癖あり） | C 型 `for (i = 0; i < n; i++) { ... }`（`continue` は step へ） |
| `++` / `--` | なし | あり（文としてのみ） |
| 複合代入 | `+=` `-=` のみ | `*=` `/=` `%=` `&=` `\|=` `^=` `<<=` `>>=` も |
| ビット反転 `~` | なし | あり |
| `true` / `false` / `null` | なし（識別子） | 予約語（v1 のソースでは識別子のまま） |
| `*void` | なし | あり |
| `private` | 予約語（`private:` ラベル用） | 予約語でない（宣言はデフォルトで private） |
| `switch` の空の `case` | `case 0:` の直後に `case 1:` は不可 | 可（空の case は何もしない。fall through はしない）。case の値の重複はエラー |
| 型の表記 | 後置 `int*`, `int[4]`, `void(int)` | 前置 `*int`, `[4]int`, `fn(int):void` |
| キャスト | `<T>x`（ビットの読み替え） | `x as T`（数値変換）と `bitcast<T>(x)`（ビットの読み替え） |
| 末尾のカンマ `[1, 2,]` / `f(a,)` | 不可 | 可（`fcc fmt` は複数行のときだけ残す） |
| `struct` / `soa` / `sizeof` / struct リテラル / `mod.T` | なし | あり（§2.1, §2.2） |
| `break` | 最も内側の**ループ**を抜ける（switch は対象外） | 最も内側のループ**または switch**を抜ける |
| ラベル付き `break` / `continue` | なし | あり |
| `include("x.rb")` / `include macro("x.rb")` | マクロファイルを読み込む | 廃止。`printf` 等は組み込み、文字表は `textmap` |
| `include kind("...")` | キンド指定可 | 廃止（拡張子で決まる） |

v1 と v2 のモジュールは 1 つのプログラムに混在できる。規則は宣言側モジュールのバージョンで決まる。
