# 言語仕様

fc 4 の言語の仕様。例は、このサイトのビルドのたびにコンパイルして動かし、載せた出力と同じことを確かめている。

## ソースファイル

- 1 つのファイルが 1 つの**モジュール**で、モジュールの名前はファイル名から `.fc` を除いたもの（`score.fc` は `score`）
- ファイルは UTF-8。1 行目に `#fc 4` を書く（書かなければ古い版の言語として読む）
- コメントは `//` から行末までと、`/* … */`
- プログラムは `main` 関数から始まる

```fc run
#fc 4
use console;

// 1 行のコメント
/* 範囲の
   コメント */
function main():void
{
	console.init();
	@printf("hello\n");
}
```

```text
hello
```

## リテラル

| 書き方 | 意味 |
|---|---|
| `123` `0x7f` `0b1010` `1_000` | 整数（10 進・16 進・2 進。桁の間の `_` は区切り） |
| `'A'` `'\n'` `'\x41'` | 文字のコードの整数（1 文字だけ。エスケープは文字列と同じ） |
| `"text"` `"a\n"` `"\x41"` | 文字列（エスケープは `\n` `\t` `\0` `\\` `\"` `\'` `\xNN`。ほかの `\` はエラー） |
| `true` `false` | bool |
| `null` | ポインタ・関数ポインタの 0 |
| `[1, 2, 3]` | 配列（最後の要素の後ろの `,` は書いてもよい） |
| `Point{x: 1, y: 2}` `{1, 2}` | struct（[struct](#struct)） |

## 型

| 型 | 大きさ | 範囲・意味 |
|---|---|---|
| `u8` / `i8` | 1 | 0〜255 / -128〜127 |
| `u16` / `i16` | 2 | 0〜65535 / -32768〜32767 |
| `bool` | 1 | `true` / `false`。比較と `&&` `\|\|` `!` の結果の型 |
| `void` | 0 | 戻り値なし |
| `*T` / `*const T` | 2 | ポインタ（`const` は指す先を書き換えない） |
| `*void` | 2 | 何を指すか問わないポインタ（どのポインタも暗黙に入る。使うときは `@bitcast` で戻す） |
| `[N]T` | N × T | 配列。`[?]T` は長さを初期値から決める |
| `[]T` / `[]const T` | 3 | slice（先頭のポインタと長さ `u8`。255 個まで） |
| `[:u16]T` / `[:u16]const T` | 4 | 長さが `u16` の slice（256 個以上。`[]T` から暗黙に変換できる） |
| `fn(T1, T2):R` | 2 | 関数ポインタ |
| `farfn(T1, T2):R` | 3 | バンクの番号つきの関数ポインタ（[far call](#バンクと-far-call)） |
| `struct` / `enum` / `soa` の名前 | | [struct](#struct)・[enum](#enum)・[soa](#soa) |

型は前に付けて、左から右へ読む。`[16]*u8` は「`u8` へのポインタの 16 個の配列」、`*[4]u8` は「`u8` の 4 個の配列へのポインタ」。

## 整数の規則

6502 は 8 ビットの CPU なので、fc は 8 ビットの計算を基本にしつつ、値を黙って変えないようにしている。

### 計算の型

- 2 つの値の演算は**大きいほうの型**、同じ大きさなら**符号付き**で計算する（`u8 + i8` は `i8`、`u8 + u16` は `u16`）
- **型のない定数**（リテラルと、型を書かない `const`）は相手の型に合わせる（`x + 1` は `x` の型）。定数のほうが大きければ広いほうで
  計算する（`x * 300` は `u16`）
- **1 つの式は、代入先の型と、式の中でいちばん広い型の、広いほうで計算する**。代入・初期化・引数・`return`・配列や struct の要素が
  1 つの式の区切り。途中の `as` も区切り（`(a + b) as u8` の中は `a + b` の型で計算する）

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	var a:u8 = 200;
	var b:u8 = 100;
	var w:u16 = a + b; // u16 で計算する
	var t = a + b; // 代入先の型が無いので u8 で計算する (折り返す)
	@printf("{} {}\n", w, t);
	var hi:u8 = 0x12;
	var lo:u8 = 0x34;
	var addr:u16 = hi << 8 | lo;
	@printf("{:x}\n", addr);
}
```

```text
300 44
1234
```

式を分けると結果が変わることがある（上の `t` を後から `u16` の変数に入れても 44 のまま）。

### 左の項の型で計算する演算

`a +% b`・`a -% b`・`a *% b` は、**左の項の型**で計算してその幅で折り返す（結果も左の項の型）。符号なしの座標に符号付きの移動量を
足して、そのまま比べる・割る・シフトするときに使う。

`+` `-` `*` は同じ大きさなら符号付きになるので、`y + dy`（`y:u8`、`dy:i8`）は `i8`。結果のビットはどちらの符号でも同じなので、
同じ大きさの変数に入れるだけ（`y += dy`）や `&` `|` `^` はそのまま書ける。結果を**符号つきの意味で読む所**（大小の比較、`/` `%`、`>>`、
`as` で 16 ビットに広げる所、型を書かない変数、`@printf` / `@format` の引数）に符号の混ざった結果を使うとエラーになるので、`+%` などか
`as` で型を選ぶ（`y +% dy` は `u8`、`dy +% y` は `i8`）。16 ビットの代入先や相手があれば、式ごと 16 ビットで正しく計算されるので
そのまま書ける（`var t:u16 = (y + dy) / 16` は 12）。

```fc error
#fc 4
function main():void
{
	var y:u8 = 200;
	var dy:i8 = 1;
	var tile:u8 = (y + dy) / 16;
}
```

```text
mixed.fc:6:16: error: `(y + dy)` mixes u8 and i8, so it is i8 (the signed type wins at the same size); `/` reads a value of 128 or more as negative. Choose the type: `a +% b` / `a -% b` / `a *% b` compute in the type of the left operand, or write `(y + dy) as T`
```

- 右の項は左の項と同じ大きさか、より狭い整数（狭ければ右の項の符号で広げる）。型のない定数は左の項の大きさに入ればよい（`x +% -1`）
- 左の項に型が要る（型のない定数は書けない）
- 結果はちょうど左の項の型で、外側の広い代入先でも広げない（`as` と同じ区切り）

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	var y:u8 = 200; // 座標
	var dy:i8 = 1; // 移動量
	@printf("{} {}\n", (y +% dy) / 16, (dy +% y) / 16); // u8 と i8 で割る
	@printf("{}\n", y +% dy > 100);
	var d:u16 = y +% 100; // u8 で折り返す
	@printf("{}\n", d);
}
```

```text
12 -4
true
44
```

### 変換

- **大きさが減る変換は暗黙にはしない**。`as` で書く（`u16` → `u8` は下位のバイト）
- 同じ大きさで符号だけ違う変換（`i8` ↔ `u8`、`i16` ↔ `u16`）は暗黙に通す（ビットはそのまま）。`x = x + vx`（`x:u8`、`vx:i8`）は書ける
- **型に入らない定数はエラー**。切り詰めるなら `as` で書く（`-1 as u8` は 255）

```fc error
#fc 4
function main():void
{
	var w:u16 = 1000;
	var x:u8 = w;
}
```

```text
narrow.fc:5:2: error: cannot convert u16 to u8 implicitly (narrowing; write `x as u8`)
```

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	var w:u16 = 1000;
	var x = w as u8; // 下位のバイト
	var m = -1 as u8;
	var v:i8 = -3;
	var p:u8 = 10;
	p = p + v; // 同じ大きさの符号違いは暗黙に
	@printf("{} {} {}\n", x, m, p);
}
```

```text
232 255 7
```

### シフト

- シフトの結果は**左辺の型**（右辺の型は関係しない）
- 結果が必ず 0 になるシフト（量が定数で左辺のビット数以上の `<<` と、符号なしの `>>`）はエラー。左辺を広げてから書く（`hi as u16 << 8`）。
  代入先で広がる式（上の `addr`）は広い型で計算するので書ける

```fc error
#fc 4
function main():void
{
	var hi:u8 = 1;
	var h = hi << 8;
}
```

```text
shift.fc:5:10: error: shifting u8 left by 8 always gives 0
```

### 比較

- 符号の違う整数の大小の比較で、どちらかの値の読み替えが起きるもの（`u8` と `i8`、`u16` と `i8` など）はエラー。`as` で揃える
- `==` / `!=` は、同じ大きさならビットで比べる（`u8` の 250 と `i8` の -6 は等しい）
- 型のない定数との比較は相手の型で比べる（`vx < 0`）

```fc error
#fc 4
function main():void
{
	var x:u8 = 1;
	var v:i8 = -1;
	if (x < v) {
	}
}
```

```text
cmp.fc:6:6: error: ordered comparison of signed i8 and unsigned u8
```

### 割り算

割り算と余りは**床除算**（商を負の無限大の向きに丸め、余りの符号は割る数に合わせる。Python と同じ）。2 のべき乗の定数で割るとシフトとマスクになる。

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	var a:i8 = -7;
	@printf("{} {} {}\n", a / 2, a % 2, 7 / -2);
}
```

```text
-4 1 -4
```

### 型を書かない配列と変数

- `var n = 式;` の型は式の型
- 型を書かない配列リテラルの要素の型は、要素の値が全部入るいちばん小さい型（`[128, -1]` は `i16`。16 ビットに入らなければエラー）

## 宣言

### 変数と定数

```fc
var x:u8; // 型を書く
var y = 10; // 型を初期値から決める (関数の中だけ)
var buf:[16]u8;
var a:u8, b:u8; // まとめて宣言
const N = 4; // 定数 (コンパイル時に決まる)
const TABLE:[?]u8 = [1, 2, 3]; // 配列の定数 (ROM に置く)
const MSG = "hello"; // 文字列の定数
```

- **グローバル変数**（関数の外の `var`）は 0 で始まり、初期値は書けない。初期値は関数の中で代入するか、`const` にする
- 関数の中の宣言はブロック（`{ … }`）ごとのスコープ
- 文字列は文字の数だけの `u8` の配列の定数で、終端の 0 は付かない（`"abc"` は `@len`・`@sizeof` とも 3。`var s = "abc";` も 3 バイトの配列）
- 0 終端の文字列が要る所（`*const u8` で受ける関数。`console.write_z` など）には `"abc\0"` と書く。0 で終わらない文字列をポインタに
  するのはエラー。文字の表として使うなら `@ptr(s)` でポインタにする
- 型を書かない文字列の配列は slice の表（`[?][]const u8`）になる: `const NAMES = ["ab", "cde"];`。同じ長さの行の 2 次元配列に詰めて
  持つなら型を書く（`const T:[2][3]u8 = ["ab", "cd"];`。短い行は 0 で詰める）

`@(build)` を付けた定数は、ビルドのときに fc.toml の `[define.モジュール名]` か `fcc build -D モジュール名.名前=値` で値を変えられる
（値は `true` / `false`・整数・文字列）。

```fc
const DEBUG = false @(build);
const QUEUE_SIZE = 128 @(build);
```

### 関数

```fc run
#fc 4
use console;

// 引数の後ろのほうには既定の値を書ける
function add(a:u8, b:u8 = 10):u8
{
	return a + b;
}

function main():void
{
	console.init();
	@printf("{} {}\n", add(1), add(1, 2));
	// 名前の無い関数
	var twice = ->fn(x:u8):u8 {
		return x * 2;
	};
	@printf("{}\n", twice(21));
}
```

```text
11 3
42
```

- 引数と戻り値の型は必須。戻り値のある関数は、どの道を通っても `return` で終わること
- 既定の値は定数の式。既定の値の付いた引数の後ろに、付いていない引数は置けない。関数ポインタ経由の呼び出しでは省けない
- 再帰する関数も書ける（その関数の変数は静的なフレームでなくスタックに置く）
- 本体の無い関数はアセンブリで書いた関数の宣言で、呼び出しの規約を `@(abi: …)` で書く（[アセンブリ](#アセンブリ)）
- ローカル変数を指すアドレスを `return` すると警告（ローカル変数の場所は、ほかの関数と使い回すため）

### struct

```fc run
#fc 4
use console;

struct Point {
	x:u8;
	y:i16;
}

const ORIGIN = Point{x: 0, y: 0};

function main():void
{
	console.init();
	var p:Point = {3, -4}; // 型が分かっていれば名前を省ける
	var q = Point{x: 5}; // 書かなかったフィールドは 0
	p.x += 1;
	@printf("{} {} {} {}\n", p.x, p.y, q.y, @sizeof(Point));
	@printf("{}\n", q == ORIGIN);
}
```

```text
4 -4 0 3
false
```

- フィールドは宣言の順に詰めて置く（揃えの隙間は無い）
- ポインタ `pp:*Point` のフィールドも `pp.x` と書く
- 代入・引数・戻り値は全体を写す。`==` / `!=` はバイトで比べる
- 全部のフィールドが定数のリテラルは ROM のデータになる

### enum

```fc run
#fc 4
use console;

enum Dir:u8 {
	UP,
	DOWN = 4,
	LEFT,
	RIGHT,
}

function name(d:Dir):[]const u8
{
	switch (d) {
	case .UP:
		return "up";
	case .LEFT, .RIGHT:
		return "side";
	default:
		return "down";
	}
}

function main():void
{
	console.init();
	@printf("{} {} {}\n", Dir.LEFT, @len(Dir), name(.RIGHT));
}
```

```text
5 4 side
```

- 基になる型（`:u8`）は省ける（`u8`）。値を省いたメンバーは前の値 + 1、最初は 0
- 型の分かる所では `.LEFT` と書ける。`@len(Dir)` はメンバーの数

### soa

`soa` は struct のフィールドごとの配列を作る（structure of arrays）。6502 では `lda フィールド,y` の形で読めるので、
struct の配列より速い。

```fc run
#fc 4
use console;

struct Enemy {
	x:u8;
	hp:u8;
}

soa Enemies:[8]Enemy;

function main():void
{
	console.init();
	var e = &Enemies[2]; // 要素のハンドル (1 バイトの添字)
	e.x = 7;
	e.hp = 3;
	Enemies[3].x = 9;
	@printf("{} {}\n", Enemies[2].x, Enemies[3].x);
}
```

```text
7 9
```

- `*Enemies` はハンドルの型（1 バイト）。関数に渡せる
- 要素は 256 個まで。配列のフィールドを持つ struct は `soa` にできない

### ストレージの別名

`alias 名前:型 = 変数;` は、グローバル変数の領域を別の型の変数として読み書きする（領域を足さない）。同時に使わない作業域を使い回す。

```fc run
#fc 4
use console;

struct Work {
	x:u8;
	y:u8;
}

var memblock:[8]u8;
alias work:Work = memblock;

function main():void
{
	console.init();
	work.x = 5;
	@printf("{} {}\n", memblock[0], @sizeof(work));
}
```

```text
5 2
```

## モジュール

```fc
use score; // score.name で使う
use score as sc; // 別の名前で
use add, MAX from score; // 名前を取り込む
use * from score; // public の宣言を全部取り込む
public use score; // このモジュールを使う側にも見せる
```

- 宣言は何も付けなければそのモジュールの中だけで見える。ほかのモジュールに見せるものには `public` を付ける
- `use` は「ソースのディレクトリ → fc.toml の `[lib.*]` → 標準ライブラリ（どのターゲットでも使えるもの → ターゲットのもの）」の順に探す
- モジュールどうしが互いに `use` してよい
- トップレベルの宣言（関数・定数・変数・struct など）は、宣言より前の行から使える
- 名前は「自分の宣言 → 取り込んだ名前 → `*` で取り込んだ名前 → 組み込み」の順に探す

### モジュールの属性

ファイルの頭のほうに `@(…);` と書くと、モジュール全体の指定になる。

| 属性 | 意味 |
|---|---|
| `@(bank: "名前")` | モジュールを fc.toml の `[bank.名前]` に置く（`"fixed"` は固定の所）。数で書けばバンクの番号（負なら後ろから） |
| `@(bss: "SEG")` | グローバル変数を置くセグメント |
| `@(near);` | 別のバンクから呼んでも far call にしない（いつも見えている所にある） |
| `@(farcall);` | far call を使う（`main` のモジュールに書く） |
| `@(static_zp: N)` / `@(static_ram: N)` | 静的フレームに使うゼロページ・RAM の大きさ（`main` のモジュールに書く） |

グローバル変数の一部だけを別のセグメントに置くときは、`@(bss: "SEG") { … }` で囲む（入れ子にでき、内側が優先。個々の変数の
`@(segment: …)` がいちばん優先）。囲んでも名前の見え方は変わらない。

```fc
@(bss: "BSS_EX") {
	var work:[64]u8;
	var flags:u8;
}
```

## 文

```fc
if (x < 10) {
	a = 1;
} else if (x < 20) {
	a = 2;
} else {
	a = 3;
}
while (a > 0) {
	a -= 1;
}
loop {
	if (done()) {
		break;
	}
}
for (var i:u8 = 0; i < 10; i += 1) {
	total += i;
}
x++;
```

- 条件には整数も書ける（0 が偽）。`&&` と `||` は左から順に、結果が決まったらそこで止める
- `loop { … }` は `break` で抜けるまで繰り返す
- `for (初期化; 条件; 次へ)` は C と同じ。`continue` は「次へ」に進む。どの部分も省ける
- `x++` / `x--` は `x += 1` / `x -= 1` の短い書き方で、文としてだけ書ける（式の値にはならない）

### for-each

```fc run
#fc 4
use console;

struct Enemy {
	x:u8;
	hp:u8;
}

const S = "hello";
var enemies:[3]Enemy;

function main():void
{
	console.init();
	for (var c in S[1..3]) {
		@printf("{:c}", c);
	}
	for (var i, c in S) {
		if (i == 0) {
			@printf(" {}={:c}", i, c);
		}
	}
	for (var i in 0..3) {
		@printf(" {}", i);
	}
	for (var p in &enemies) {
		p.hp = 3; // ポインタで回すと書き換えられる
	}
	@printf(" {}\n", enemies[2].hp);
}
```

```text
el 0=h 0 1 2 3
```

- `for (var x in A)` は配列・slice の要素を、`for (var i, x in A)` は添字と要素を回す。回す変数は書き換えられない
- `for (var p in &A)` は要素へのポインタを回す（要素を書き換えられる）
- `for (var i in a..b)` は a から b の手前まで、`a..=b` は b を含む。`for (var i:u16 in 0..300)` のように型を書ける

### switch

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	for (var x in [1, 5, 50, 200]) {
		switch (x) {
		case 1, 2:
			@printf("small ");
		case 3..10:
			@printf("mid ");
		case 100..=250:
			@printf("big ");
			fallthrough;
		default:
			@printf("other ");
		}
	}
	@printf("\n");
}
```

```text
small mid other big other 
```

- `case` には値を並べられ、範囲（`a..b` は b を含まない、`a..=b` は含む）も書ける
- case の終わりで switch を抜ける。`fallthrough;` を case の最後に書くと、次の case の本体へ進む
- case ごとにスコープがある
- 整数の case が 10 個以上で密に並んでいれば、表で飛ぶ（case の数によらず約 33 サイクル）

### ラベル

`loop` / `while` / `for` / `switch` にはラベルを付けられる。`break ラベル;` はその文を抜け、`continue ラベル;` はそのループの次の繰り返しへ進む。

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	outer: for (var i:u8 = 0; i < 3; i += 1) {
		for (var j in 0..3) {
			if (j == 1) {
				continue outer;
			}
			@printf("{}{} ", i, j);
		}
	}
	@printf("\n");
}
```

```text
00 10 20 
```

### コンパイルのときの分岐

`@if (条件) { … } else { … }` は、どちらかだけをコンパイルする。条件はリテラルと `@(build)` の定数だけ。選ばれなかった側は名前も解決しない。
トップレベルでも関数の中でも書け、スコープは作らない。

```fc
const DEBUG = false @(build);

@if (DEBUG) {
	use debug_menu;
}
```

## 式

優先順位（上が強い）:

| 演算子 | 意味 |
|---|---|
| `.` `()` `[]` | フィールド・モジュールの名前、呼び出し、添字 |
| `!` `~` `-` `+` `*` `&` | 前置: 論理の否定、ビットの反転、符号、ポインタの指す先、アドレス |
| `as` | 型の変換 |
| `*` `/` `%` `*%` | 掛け算・割り算・余り（`*%` は左の項の型で計算する） |
| `+` `-` `+%` `-%` | 足し算・引き算（`+%` `-%` は左の項の型で計算する） |
| `<<` `>>` | シフト |
| `<` `>` `<=` `>=` | 大小の比較 |
| `==` `!=` | 等しいか |
| `&` | ビットの AND |
| `^` | ビットの XOR |
| `\|` | ビットの OR |
| `&&` | 論理の AND |
| `\|\|` | 論理の OR |
| `=` `+=` `-=` `*=` `/=` `%=` `&=` `\|=` `^=` `<<=` `>>=` | 代入（`a op= b` は `a = a op b`） |

C と同じく、`&` は `==` より弱い（`a & MASK == 0` は `a & (MASK == 0)`。警告が出るので括弧を付ける）。

### 添字と slice

- `a[i]` の添字は `u8` か `u16`（`u8` のほうが速い）。範囲の外は未定義（定数の添字が配列の外ならエラー）
- 添字の式 `a[i + k]`（k は定数）は、`i + k` が 8 ビットで折り返さないものとみなす（`i + k` が 256 以上になるのは範囲の外と同じく未定義。
  最適化が「`a` の k 先を添字 `i` で」読むため）。折り返しを当てにするリングバッファは、変数で進めてから引く（`j = i + k; a[j]`）
- `a[lo..hi]` / `a[lo..]` / `a[..hi]` / `a[..]` は配列・slice の一部の slice。`a[lo..=hi]` は hi を含む
- 長さの分かる配列は slice に暗黙に変換できる
- `@len(s)` は長さ、`@ptr(s)` は先頭のポインタ、`@slice(p, n)` はポインタと長さからの slice

### ポインタ

- `&x` はアドレス、`*p` は指す先。struct へのポインタのフィールドは `p.x`
- `p + n` は要素 n 個分進める（`p[n]` と `*(p + n)` は同じ）。同じ型のポインタの差は要素の数
- ポインタを進められるのは、同じ配列（か変数）の中と、末尾の 1 つ先まで。それを超えた値は未定義（C と同じ。最適化がこれを前提にする）
- `*void` は算術できない。`null` はどのポインタとも比べられる

### 型の変換

| 書き方 | 意味 |
|---|---|
| `x as T` | 数の変換（大きくするときは、元が符号付きなら符号を広げる。小さくするときは下位のバイト。同じ大きさの符号違いはビットのまま） |
| `@bitcast(T, x)` | ビットの読み替え（大きさの同じもの: ポインタどうし、ポインタと `u16`、同じ大きさの整数） |

`as` は前置の演算子の次に強い（`-x as u16` は `(-x) as u16`、`a + b as u16` は `a + (b as u16)`）。

## 組み込み

`@` で始まる名前は組み込み。

| 組み込み | 働き |
|---|---|
| `@len(x)` | 配列・slice の長さ、enum のメンバーの数 |
| `@sizeof(T)` / `@sizeof(x)` | 型・変数の大きさ（バイト） |
| `@ptr(s)` / `@slice(p, n)` | slice の先頭のポインタ / ポインタと長さからの slice |
| `@bitcast(T, x)` | ビットの読み替え |
| `@min(a, b)` / `@max(a, b)` / `@clamp(x, lo, hi)` | 小さいほう・大きいほう・範囲に収めた値（その場に比べるコードを出す） |
| `@copy(dst, src)` | 短いほうの長さだけ写して、写した数を返す |
| `@format(dst, "書式", …)` | `dst` に書式どおりに書き、書いた部分の slice を返す |
| `@try_format(dst, "書式", …)` | `@format` と同じだが、足りなければ止まらずに長さ 0 の slice を返す |
| `@printf("書式", …)` | `console` に書式どおりに出す |
| `@assert(式 [, "文言"])` | 式が偽なら、ファイル・行・式を出して止まる |
| `@assert_eq(実際, 期待)` | 違えば、両方の値を出して止まる |
| `@run_tests()` | すべての `@(test)` の関数を順に呼ぶ（`fcc test` が使う） |
| `@incbin("file")` | ファイルの中身を `u8` の配列の定数にする |
| `@include("file")` | アセンブリ（`.asm` / `.inc`）や CHR（`.chr`）をモジュールに入れる |
| `@lz4(data)` / `@rle(data)` | 定数の配列をコンパイルのときに圧縮する（`lz4.unpack` / `rle.unpack` で展開） |
| `@textmap("表" [, "訳.po"])` | 独自のフォントの文字表から、文字列を文字のコードの配列に変える変換器を作る |
| `@asm("命令", …)` | インラインアセンブラ（引数ごとに 1 行） |
| `@log("書式", …)` | エミュレータ側で値を出すログ（ROM は変わらない。`fcc build -g`） |
| `@bank("名前")` | fc.toml の `[bank.名前]` のバンクの番号 |

### 書式

`@format` と `@printf` の書式は定数の文字列で、コンパイルのときに分解する（実行のときに書式を読まない）。

::: v-pre

| 書き方 | 意味 |
|---|---|
| `{}` / `{0}` | 次の引数 / 0 番目の引数 |
| `{:5}` | 幅 5 で右に寄せる（幅は 31 まで） |
| `{:05}` | 0 で埋める（符号の後ろに: `-05`） |
| `{:x}` / `{:X}` / `{:b}` | 16 進（小文字 / 大文字）/ 2 進 |
| `{:c}` | 1 バイトの整数を 1 文字として |
| `{:d}` | bool を 0 / 1 で |
| `{{` / `}}` | `{` / `}` そのもの |

:::

整数は型の符号で 10 進、bool は `true` / `false`、`[]u8` の文字列はそのまま出す。

```fc run
#fc 4
use console;

var line:[20]u8;

function main():void
{
	console.init();
	var hp:u8 = 7;
	var s = @format(line, "HP {:3}/{:03}", hp, 50);
	@printf("[{}] {} {:X} {:b} {:c} {{}}\n", s, @len(s), 255, 5, 'A');
	var t = @try_format(line[..4], "{}", 12345);
	@printf("{}\n", @len(t));
}
```

```text
[HP   7/050] 10 FF 101 A {}
0
```

### テスト

```fc test
#fc 4

function twice(x:u8):u8
{
	return x * 2;
}

function test_twice():void @(test)
{
	@assert_eq(twice(3), 6);
	@assert(twice(0) == 0, "zero");
}
```

`@(test)` の関数は引数も戻り値も無い。普通のビルドでは ROM に入らない。`fcc test` で走らせる。

## 属性

宣言の後ろに `@(名前: 値, …)` と書く。真か偽の属性は値を省ける（`@(inline)`）。

### 関数の属性

| 属性 | 意味 |
|---|---|
| `@(inline)` / `@(noinline)` | 呼び出しをその場に展開する / しない（小さな関数は印が無くても展開する） |
| `@(test)` | テストの関数 |
| `@(interrupt)` | 割り込みから呼ぶ関数（その変数の場所をほかの関数と使い回さない） |
| `@(abi: "frame" / "stack" / "cc65")` | 呼び出しの規約（[アセンブリ](#アセンブリ)） |
| `@(symbol: "名前")` | アセンブリから見える名前を決める（本体が無ければ、その名前のアセンブリの定義を呼ぶ） |
| `@(near)` | 別のバンクから呼んでも far call にしない |
| `@(zeropage: false)` | 関数の変数をゼロページでなく RAM に置く |

### 変数の属性

| 属性 | 意味 |
|---|---|
| `@(address: 0x2007)` | 決まった番地に置く（I/O のレジスタなど。volatile になる） |
| `@(volatile)` | 読み書きのたびにメモリを読み書きする（割り込みやアセンブリが書き換える変数） |
| `@(segment: "名前")` | 置くセグメント（fc.toml の `[ram.名前]` など） |
| `@(symbol: "名前")` | アセンブリから見える名前を決める |

アセンブリから見える名前は、何も付けなければ `_モジュール名_名前`（`score.best` なら `_score_best`）。

## アセンブリ

`@include("file.asm")` で ca65 のアセンブリをモジュールに入れ、本体の無い関数の宣言で呼ぶ。本体の無い関数は呼び出しの規約を書く。

| 規約 | 渡し方 |
|---|---|
| `@(abi: "frame")` | 関数ごとの固定の領域 `F_名前` に、戻り値（先頭）→ 引数（宣言の順、下位のバイトが先）を置いて `jsr`。レジスタは使わない。A / X / Y は壊してよい。`F_名前__引数名` で引数の場所を参照できる。`@(scratch: N)` で作業用の N バイトを足せる |
| `@(abi: "stack")` | fc のスタックで渡す（`S+k,x`） |
| `@(abi: "cc65")` | cc65 の `__fastcall__`（引数は 0〜1 個で A か A/X、戻り値は A か A/X） |

```fc
function fill_raw(dst:*u8, n:u16, v:u8):void @(abi: "frame");
@include("mem.asm");

var status:u8 @(symbol: "status");

function reset():void
{
	@asm("lda #0", "sta status");
}
```

## バンクと far call

マッパーで ROM のバンクを切り替えるプログラムは、fc.toml の `[target]` と `[bank.*]` でバンクを決め、モジュールを `@(bank: "名前")` で置く
（[fc.toml](./fc-toml)）。`main` のモジュールに `@(farcall);` を書くと、**別の切り替えるバンクにあるモジュールの関数**の呼び出しを、
コンパイラがバンクを切り替えて呼んで戻す形（far call）にする。呼ぶ側の書き方は普通の呼び出しと同じ。

- 固定の所・同じモジュール・`@(near)` の関数への呼び出しは普通の `jsr`
- `farfn` の関数ポインタはバンクの番号を持ち、呼ぶときに切り替える（`fn` のポインタのバンクは呼ぶ側が決める）
- 別のバンクのデータを読むときのバンクは、呼ぶ側が決める
- `fcc build -d` で far call になった所を並べる

## 関数の変数の置き場所

再帰しない関数の引数・戻り値・変数は、関数ごとの決まった場所（静的フレーム）に置く。呼び出しの関係を見て、同時に使われない関数どうしで
同じ場所を使い回すので、使う領域は「いちばん深い呼び出しの連鎖の分」くらいで済む。ゼロページに入るだけ置き、残りは RAM に置く。

再帰する関数と `@(abi: "stack")` の関数の変数はスタックに置く。関数ポインタで呼ぶ関数や `@(interrupt)` の関数は、呼び出しのたびに
自分の場所へ引数を写す。

## 使われないものは出力しない

`main`・割り込み・アドレスを取られた関数から呼び出しをたどって届かない関数は、ROM に入らない（`public` でも同じ）。
使われない private の変数と配列の定数も領域を取らない。`fcc build -d` で出力しなかった関数を並べる。
