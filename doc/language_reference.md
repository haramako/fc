# FC 言語リファレンス（文法 v2）

FC は NES（6502）向けの静的型付き言語で、C 風の構文を持つ。コンパイラ `fcc` は FC ソースを
ca65 アセンブリに変換し、ld65 でリンクして NES ROM（`-t nes`）または内蔵エミュレータ用バイナリ
（`-t emu`、既定）を作る。

本書は **文法 v2**（ファイル先頭に `#fc 2`）を正典として記述する。v1（宣言なし）との差は
末尾の「v1 との違い」にまとめた。実装上の細かい挙動で仕様として確定していないもの（Ruby 版由来の癖）は
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
```

---

## 2. 型

| 型 | サイズ | 範囲 | 備考 |
|---|---|---|---|
| `int` / `uint` / `int8` / `uint8` | 1 | 0〜255 | `int` は `uint8` の別名 |
| `sint` / `sint8` | 1 | -128〜127 | |
| `int16` / `uint16` | 2 | 0〜65535 | |
| `sint16` | 2 | -32768〜32767 | |
| `bool` | 1 | | 条件式の結果。整数と互換 |
| `void` | 0 | | 戻り値なし |
| `T[N]` | N×size | | 配列。`T[]` は長さ省略（初期値か `address` から決まる） |
| `T*` | 2 | | ポインタ |
| `R(T1, T2, ...)` | 2 | | 関数型。`fastcall` 属性は型の一部 |

- 整数リテラルの型は値で決まる: 0〜255 → `uint8`、256 以上 → `uint16`、-1〜-127 → `sint8`、-128 以下 → `sint16`
  （注: `-128` が `sint16` になる境界は Ruby 版由来）
- 型変換は明示的に `<type>expr`（例 `<sint8>x`、`<int16>y`）。サイズが合わない代入はエラー
- 配列は暗黙にポインタへ変換される（`int[]` を `int*` の引数に渡せる）

---

## 3. リテラル

```
123      0x7f      0b1010   1_000  // 10 進 / 16 進 / 2 進（桁の間の '_' は区切り）
"text"   'text'                    // 文字列（どちらも同じ。末尾に終端の 0 が付く）
"\n"     "\x41"                   // エスケープは \n と \xNN（16 進 2 桁）だけ。他の \ はそのまま
[1, 2, 3]                          // 配列リテラル（定数）
incbin("data.bin")                 // ファイルの中身を配列定数として埋め込む
->int(a:int) { return a + 1; }     // ラムダ（無名関数）
```

文字列は終端 0 付きの `uint8[]` 定数（ポインタ `uint8*` としても使える）。

---

## 4. 宣言

### 4.1 変数・定数

```
var x:int;                     // 型指定
var y = 10;                    // 型推論（初期値の型）
var a:int[16];                 // 配列
var p:int* = &x;               // 初期化は関数内のみ（グローバル変数は初期化できない）
const N = 4;                   // 定数（コンパイル時に確定）
const T:int[] = [1, 2, 3];     // 配列定数（ROM に置かれる）
var a:int, b:int;              // まとめて宣言
```

グローバル変数の属性:

```
var vram:int options(address: 0x2007);        // 固定アドレス（メモリマップド I/O）
var buf:int[256] options(segment: "BSS_EX");  // 配置セグメント
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
- `fastcall`: 引数をスタックではなくレジスタ/ゼロページで渡す。fastcall 関数の中から他の関数は呼べない
- `options(symbol: "...")`: 生成するシンボル名を固定する（割り込みベクタなど）

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
| `.` `()` `[]` | メンバ参照、呼び出し、添字 |
| `!` `-` `+` `*` `&` `<type>` | 前置: 否定、符号、デリファレンス、アドレス、型変換 |
| `*` `/` `%` | 乗除（除算・剰余は切り捨て。注: 負数は Ruby 準拠の床除算） |
| `+` `-` | 加減 |
| `<<` `>>` | シフト（2 バイト値のシフト量は定数のみ） |
| `<` `>` `<=` `>=` | 比較 |
| `==` `!=` | 等値 |
| `&` | ビット AND |
| `^` | ビット XOR |
| `\|` | ビット OR |
| `&&` | 論理 AND（短絡） |
| `\|\|` | 論理 OR（短絡） |
| `=` `+=` `-=` | 代入（右結合） |

- `a[i]`: 配列/ポインタの添字。添字は 1 バイト（2 バイトの添字は不可）
- `*p` / `&x`: ポインタ演算
- `mod.name`: モジュールの public 宣言
- `f(args)`: 関数呼び出し。引数の数と型を検査する
- 定数式（リテラル・`const`・それらの演算）はコンパイル時に畳み込まれる

---

## 7. 組み込み

`include` なしで常に使える。

| 名前 | 働き |
|---|---|
| `asm("lda #1", "sta $2000")` | インラインアセンブラ（各引数が 1 行） |
| `printf(a, b, ...)` | 引数の型で `stdio.print`（`uint8*`）/ `stdio.print_int16`（整数）を呼び分ける。`stdio` モジュールが必要 |
| `unittest_run_tests()` | `stdio.init()` の後、スコープ内の `test_*` 関数を宣言順に呼び、`stdio.exit(0)` する（`stdio` が必要） |
| `cos(x)` | `math.sin(x + 64)` に展開（`math` モジュールが必要）。`math.cos(x)` とも書ける |
| `incbin("file")` | ファイルを配列定数として埋め込む |
| `textmap("table.txt")` | 文字表を読み、文字列→文字コード配列の変換器（定数）を作る |

### 7.1 `textmap`

```
public const _T = textmap("../tmp/font/text.chr.txt");
...
print(_T("こんにちは"));   // 文字表のコード列 + 終端 0 の int[] 定数になる
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
| `break` | 最も内側の**ループ**を抜ける（switch は対象外） | 最も内側のループ**または switch**を抜ける |
| ラベル付き `break` / `continue` | なし | あり |
| `include("x.rb")` / `include macro("x.rb")` | Ruby マクロを読み込む | 廃止。`printf` 等は組み込み、文字表は `textmap` |
| `include kind("...")` | キンド指定可 | 廃止（拡張子で決まる） |

v1 と v2 のモジュールは 1 つのプログラムに混在できる。規則は宣言側モジュールのバージョンで決まる。
