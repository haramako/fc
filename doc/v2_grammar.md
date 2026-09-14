# 文法 v2 設計メモ

作成: 2026-09-13（Opus 5）、**2026-09-14 レビュー済み（Q1〜Q6 決定、VERSION_STR 廃止、break の仕様変更を追加）**。
[go_evolution_plan.md](go_evolution_plan.md) F-fmt の「文法 v2 の設計」項。
決定済みの前提は [v2_decisions.md](v2_decisions.md) §1〜§4・§6。判断の記録は末尾 §9。

---

## 1. 目的と原則

v2 は「**v1 ソースを `fcc migrate` で機械的に移行できる、破壊的変更の一括適用**」である。

- **P1. 破壊的変更だけを v2 に入れる。** 追加的な機能（新演算子・新文など）はバージョンを上げずに
  いつでも足せるので v2 の範囲にしない。v2 に入れる基準は「v1 と意味が変わる、または v1 の構文を削る」
- **P2. 移行元は現時点の v1 ソース**（castle / miku / fclib / test）。手直しを前提にしない
  （[v2_decisions.md](v2_decisions.md) §6）。castle の `macro.rb` だけは Ruby なので例外（§6.3）
- **P3. 移行の検証は asm 一致。** 文法 v2 の変更はすべて名前解決・表記の変更であり、生成コードは変えない。
  移行前後で各モジュールの `.s`/`.inc` が一致することを受け入れ条件にする
- **P4. AST は共通**（v2_plan.md C2）。v1/v2 の差はパーサの受理範囲と sema の名前解決規則だけ。
  フォーマッタ・マイグレータは同じ `syntax.File` を扱う
- **P5. 混在可。** 1 プログラム内に v1 モジュールと v2 モジュールが共存できる。規則は**宣言側モジュールの
  バージョン**で決まる（§4）。これでモジュール単位の段階的移行ができる

---

## 2. 現状の事実（コーパス計測 2026-09-12〜13）

| 項目 | 数 | 備考 |
|---|---|---|
| `.fc` ファイル | 53 | test 13 / fclib 10 / miku 5 / castle 41（うち複数ファイル構成の en1〜en8） |
| `use X;` / `use * from X;` / `use X as Y;` | 143 / 70 / 1 | 選択的インポートは構文的に到達不能（`parser.y` `id_list: tID`） |
| `public:` / `private:` ラベル | 0 / 21 | `public:` は未使用。`private:` は castle のみ |
| 宣言への `public` キーワード | 378 | function 219 / const 137 / var 26。`private` キーワードは 0 |
| `mod.name` で private に届く参照 | 38 箇所 28 シンボル | castle のみ。§6 参照 |
| `include("*.asm")` / `("*.chr")` / `("*.inc")` | 16 / 12 / 1 | 拡張子でキンド判定 |
| `include("*.rb")` / `include macro("*.rb")` | 3 / 3 | fclib 4 + castle 2。マクロは 7 個（§6.3） |
| `options(...)` のキー | segment 62 / address 56 / fastcall 46 / bank 26 / symbol 6 / mapper 2 / org 1 / char_banks 1 / bank_count 1 | モジュール用 29 文、関数・変数用が残り |
| `loop()` | 22 | すべて `loop(){` 形 |
| ラムダ | 3 | `->int(a:int){...}` |
| 型名 | int 1140 / void 500 / int16 92 / sint 72 / sint8 20 / uint16 11 / uint 9 / int8 8 / uint8 4 | `int` = `uint8` の別名 |

---

## 3. 変更一覧

凡例: **[決定]** v2_decisions.md で決定済み / **[提案]** 本書の提案（要判断）/ **[見送り]** v2 に入れない

### 3.1 バージョン宣言 [決定]

```
#fc 2
```

ファイル先頭行。宣言なし = v1。`#` は v1 レキサでトークンにならないため v1 処理系は v2 ファイルを誤読せず
エラーになる。（v2_decisions.md §4）

実装: レキサが先頭行の `#fc <n>` を読んで `File.Version` に入れる。それ以外の `#` 行はエラー。
`fcc fmt` は v1 ファイルを v1 のまま整形する（バージョンを変えるのは `migrate` だけ）。

### 3.2 可視性 [決定]

| | v1 | v2 |
|---|---|---|
| 宣言のデフォルト | public（`Module.CurrentPublic` 初期値） | **private** |
| `public:` / `private:` ラベル | 以降のデフォルトを切り替える | **削除**（構文エラー） |
| `private` キーワード | 文法上あるが未使用 | **削除**（デフォルトなので不要） |
| `mod.name`（ドット参照） | private にも届く | **public のみ**（規則 S7） |
| `use * from mod` | public のみ取り込む | 同じ |
| `use X;` が作るモジュール束縛 | public（glob で再輸出される） | **private**。再輸出は `public use X;` で明示（S6） |

`public` は `var` / `const` / `function` / `use` の前に置く。castle の `common.fc` は
`public use nes; public use ppu; ...` になる（24 モジュールが `use * from common` 経由で `nes.x` を参照している）。

### 3.3 `use` の選択的インポート [決定]

```
use a, b from mod;          // 新設。mod の public 宣言 a, b を非修飾名で取り込む
use * from mod;             // 維持
use mod;                    // 維持。モジュール束縛 `mod` を作る
use mod as m;               // 維持
public use mod;             // 新設（§3.2）。束縛を再輸出する
```

名前衝突の規則は S1〜S6 + S7（v2_decisions.md §1.1）。`use a from mod as x;`（リファレンスにある改名付き
選択的インポート）は**入れない**（用途がなく、`use mod as m; m.a` で足りる。Q1 決定）。

### 3.4 `include` [決定]

| 形 | v1 | v2 |
|---|---|---|
| `include("x.asm")` / `("x.inc")` | アセンブラをモジュールに取り込む | 維持 |
| `include("x.chr")` | CHR データ | 維持 |
| `include("x.rb")` / `include macro("x.rb")` | Ruby マクロ | **削除**。§6.3 の置換へ。移行期間中は警告付きで受理 |
| `include kind("...")` のキンド指定 | `macro` のみ実用 | **削除**（拡張子で決まる） |

### 3.5 マクロの置換 [決定・実装済み]

> 実装時の変更 (2026-09-14): **`cos` は関数にせず組み込みマクロのまま**にした。castle が `math.cos` を 2 箇所で
> 使っており、関数にすると呼び出し側の asm が変わる (P3 違反)。組み込みはグローバルスコープにあるので
> `math.cos(x)` は math のスコープから親に辿り着いて従来どおり書ける。関数化はインライン関数 (F2) と一緒に。

v2_decisions.md §3 で「汎用マクロは作らず用途別の小機能に置換」と決定済み。表記（Q2 決定）:

| v1 | v2 | 備考 |
|---|---|---|
| `printf(...)` （`include("stdio.rb")` が必要） | `printf(...)` **組み込み**。include 不要 | 引数型で `print` / `print_int16` に振り分ける現行動作のまま。`stdio` モジュールの `print`/`print_int16` を参照するので、`use stdio` 系が無いモジュールでは「stdio が見つからない」エラー |
| `unittest_run_tests()` （`include("unittest.rb")`） | `unittest_run_tests()` **組み込み** | スコープ内の `test_*` を列挙して呼ぶ。`stdio.init/print/exit` 参照は組み込み側の特権で行う（§3.2 の S7 の例外ではなく、コンパイラ内部の参照） |
| `cos(x)` （`include("math.rb")`） | `math.fc` に **普通の関数** `function cos(x:int):int { return sin(x + 64); }` | 呼び出し箇所は 0。インライン化は F2 で |
| `times(n){...}` | **削除** | 使用 0 |
| castle `_T("…")` / `_M("…")` | `const _T = textmap("../tmp/font/text.chr.txt");` を宣言し、**呼び出し側は `_T("…")` のまま** | `textmap(path)` は組み込みで、型 `textmap` の定数を作る。textmap 型の値の呼び出し `_T("…")` は定数式で、文字列を表で変換した `[..., 0]` の `int[]` リテラルになる。**63 + 8 箇所の呼び出しを変えずに済む** |
| castle `VERSION_STR()` | **廃止**（2026-09-14 決定）。castle 側を固定文字列 `_M("VERSION 0.5.0")` に変更済み（`title.fc`、マクロは `macro.rb` と Go 側から削除。ROM 一致を確認） | ビルド時定数の注入機構（`inctext` 等）は作らない。必要になったら追加的に入れる |

`textmap` の表ファイル形式は現行 `nes_tools` の `TextConverter`（`internal/sema/textconv.go` に移植済み）と同じ。
リテラル接頭辞（`t"…"`）は**作らない**（Q2 決定）。`_T("…")` の呼び出し側 71 箇所は無変更で移行する。

### 3.6 `loop` [決定]

`loop() { ... }` → `loop { ... }`（Q3 決定）。括弧は無意味で、22 箇所すべて `loop(){` 形なので機械置換できる。
`language_reference.md` はすでに `loop {` と書いている。

### 3.7 `break` / `continue` とラベル [決定 2026-09-14]

現行（v1）の `break` は **switch を抜けず、外側のループを抜ける**（sema は `h.loops` しか見ない。ループの外なら
"cannot break without loop"）。`case` は fallthrough しないので switch を抜けるための `break` は不要だったが、
C の感覚と逆で罠になっている。コーパスでは `break;` 37 箇所のうち **7 箇所が switch の中**（castle `debug_menu.fc` 6、
`title.fc` 1）で、すべて「switch を含むループを抜ける」意図で書かれている。

v2 の仕様:

| | v1 | v2 |
|---|---|---|
| `break;` の対象 | 最も内側の**ループ** | 最も内側の **ループまたは switch**（C / Go と同じ） |
| `continue;` の対象 | 最も内側のループ | 同じ（switch は対象外） |
| ラベル | なし | **文ラベル** `L: loop { ... }` / `L: while (...) { ... }` / `L: for (...) { ... }` / `L: switch (...) { ... }` と **`break L;` / `continue L;`** |
| `case` の fallthrough | なし | なし（変更しない） |

文法: ラベルは `IDENT ':'` を繰り返し文・switch の直前に置く（Go と同じ）。v1 の `public:` / `private:` は
キーワードなので衝突しない（v2 では消える）。`break L;` の L が囲むラベルでなければエラー。
ラベルは文のスコープだけに存在し、変数名と衝突しない。

移行: マイグレータが「switch の中にあり、最も内側の breakable が switch である `break;`」を見つけたら、
その switch を囲む最も内側のループにラベル（`loop_1:` のような未使用名）を付け、`break L;` に書き換える。
7 箇所すべて機械的に処理でき、生成コードは変わらない（P3 で確認）。sema 側は `h.loops` を「ラベル付き
breakable のスタック」に一般化し、v1 モジュールでは switch をスタックに積まない。

### 3.8 C 型 `for` と `++` / `--` [決定 2026-09-14・実装済み]

v1 の `for (i, from, to) { ... }` は `i = from; while (i < to) { body; i = i + 1; }` への脱糖で、
**`continue` がインクリメントを飛ばして無限ループになる**バグがあった（castle の memo.md）。
C 型に置き換える（Q7 決定: v2 では C 型のみ、旧形式は v1 だけ）:

```
for (init; cond; step) { body }         // init: var 宣言 | 式 | 省略、cond: 式 | 省略、step: 式 | x++ | 省略
x++;  x--;  ++x;  --x;                  // x = x + 1 / x = x - 1。文としてのみ (Q8: 式の値は持たない)
```

- 意味: `{ init; loop { if (cond) { body; step; } else break; } }`。v1 の脱糖と同じ IR 形なので、
  `for (i, 0, n)` → `for (i = 0; i < n; i++)` の移行で**生成コードは変わらない**（`continue` を含む for だけ、
  v2 では step に飛ぶので変わる。コーパスには 0 件）。step のラベルは continue がある場合だけ作る
- init の `var` は for のスコープに閉じる。`var i = 0` の型は現行規則で `int`（Q9）。`var i:uint8 = 0` も可
- `++`/`--` の対象は左辺値。`y = x++` は文法上 `(y = x)++` に読めるので、代入の中の `++` はエラーにする
- 移行: `fcc migrate` は v2 ファイルにも「旧 for → C 型」だけを適用する（リポジトリ内の fclib / castle / miku /
  test/v2 は 2026-09-14 に再移行済み。asm 一致）

### 3.9 レキサの是正 [提案・バージョン非依存]

v1/v2 共通のレキサで直す（直しても正しいプログラムの意味は変わらない。v2_plan.md §2.5 の保存項目）:

- `0b2` のような不正な 2 進数リテラル → エラー（現行は Ruby `to_i` 準拠で `0` に丸める）
- `\xZZ` の不正エスケープ → エラー（現行は 0）
- 数値リテラルの `_` 区切り（`1_000`）を許す（現行は `1` と識別子 `_000` に分かれる）。これは追加的
- `//` の直後に改行が来る空コメントの取り扱いは新レキサで既に正しい

これらは文法バージョンと独立に R4 の項目として扱う。v2 の範囲には入れないが、`#fc 2` の導入と同時期に
やるとマイグレータの検証（asm 一致）で副作用を確認できる。

### 3.10 見送り（v2 に入れない）[見送り]

追加的に後から入れられるもの、または判断材料が足りないもの（Q6: いったん追加なし）:

| 項目 | 理由 |
|---|---|
| `options(...)` → `@attr` 等の表記変更 | 文法上は可能だが利用者の利益が小さい。キーの整理（`address`/`segment`/`fastcall`/`symbol`）は後で追加的にできる |
| 「複数ファイル = 1 モジュール」（castle の `en` ← `en1`〜`en8`） | §2 の検討項目。現行の `use * from en1;` 再輸出で動いており、v2 の `public use` でも同じことができる。別途設計 |
| `elsif` → `else if` | 趣味の範囲。どちらも許すのは混乱の元 |
| `'...'` と `"..."` の区別（文字リテラル） | 現行は両方文字列。`print('.')` が 1 文字**文字列**として動いている。変えると意味が変わる |
| `~`、`\|=` `&=` `^=`、`sizeof`、`goto`、ブロックスコープ、インライン関数、クロージャ | memo.txt 由来の機能追加。すべて追加的なので F2 で |
| `;` / `(` の省略 | memo.txt にあるが文法の曖昧性を生む。やらない |

---

## 4. 実装方針

### 4.1 パーサ: 1 つの文法 + バージョンゲート

v1/v2 の文法差は「削る 4 つ（可視性ラベル、`private` キーワード、`include kind`、`loop()` の括弧）＋足す 4 つ
（`use a, b from`、`public use`、`loop {`、文ラベルと `break L;`/`continue L;`）」と小さく、式・型・宣言の骨格は同じ。goyacc の文法ファイルは**スーパーセット 1 本**にし、
`parse.go` で `File.Version` を見て「v2 で削られた構文」「v1 に無い構文」をエラーにする。

- 文法ファイルを 2 本持つと、AST 構築コード（`parser.y` のアクション）が重複し、以後の変更を 2 回書くことになる
- ゲートの場所: パーサ側（構文木を作った直後、`parse.go`）。sema に v1/v2 の分岐を増やさない

### 4.2 sema: 宣言側モジュールのバージョンで規則を切り替える

| 規則 | 決め手 |
|---|---|
| 宣言のデフォルト可視性、ラベルの効果 | 宣言しているモジュールのバージョン |
| `mod.name` が private に届くか | **参照先** `mod` のバージョン（v1 モジュールなら届く） |
| `use X;` の束縛が再輸出されるか | `use` を書いたモジュールのバージョン（v1 なら public） |

これで「v2 の main が v1 の fclib を使う」「v1 の castle が v2 化した 1 モジュールを使う」の両方が動く。
`ir.Module` に `Version int` を持たせる。v1 の規則は移行期間中ずっと維持する（P2）。

### 4.3 組み込み（`printf`、`unittest_run_tests`、`textmap`）

現行の `macros.go` は `include("x.rb")` のファイル名で登録している。これを**常時登録の組み込み**に変える:

- `printf` / `unittest_run_tests` / `cos`(削除) / `times`(削除): ファイル名キーをやめ、`Program` 生成時に登録
- `textmap(path)`: 新設。定数式。`types.Textmap` 型の値を返す。この値を関数のように呼ぶ式 `T("…")` は
  定数式で `int[]` リテラルに評価される（現行 `_T` の挙動）。表の「未知の文字を追加登録する」破壊的挙動は
  **定数ごとに閉じる**（`const _T = textmap(...)` の値が表を 1 つ持つ）。現行も `include("macro.rb")` は
  `common.fc` の 1 回だけで表は 1 つ、`_T` は `use * from common` で配られているので、v2 でも同じ 1 表を全モジュールが
  共有し、登録順 = コンパイル順も変わらない。移行検証の asm 一致で最終確認する

v1 モジュールでの `include("x.rb")` は受理して警告を出し、何もしない（組み込みが常時有効なので）。
castle の `include("macro.rb")` だけは「置換が必要」と警告する。

### 4.4 フォーマッタ

`fcc fmt` は v1/v2 両方を整形する。v2 固有の出力: `#fc 2` を先頭行に、`public use mod;`、`use a, b from mod;`。
ラベルは v2 の AST には現れない（パーサが拒否する）。

---

## 5. `fcc migrate` の設計

```
fcc migrate [--visibility=minimal|preserve] [--lib DIR]... <main.fc> [<main2.fc>...]
```

1. **解析**: 引き数の main すべてを v1 sema でフルコンパイルする（解析だけでよいが、現行の sema は
   コンパイルしながら解決するので、そのまま走らせて参照を記録するのが確実）。記録するもの:
   - `(参照元モジュール, 参照先モジュール, シンボル)` — ドット参照と glob 経由の非修飾参照
   - `(モジュール, use 束縛名)` — その束縛が他モジュールから glob 経由で参照されたか（`public use` が要るか）
   - `include("x.rb")` の出現箇所
   - switch の中にあり最も内側の breakable が switch である `break;`（§3.7。囲むループにラベルが要る）
2. **書き換え**（AST 上で。ファイルごと）:
   - 先頭に `#fc 2`
   - `ScopeLabel` を削除。その効果（以降のデフォルト）は `VarDecl.PublicPos` / `FuncDecl.PublicPos` に展開する
   - 可視性: `minimal` なら 1 で参照された宣言だけ `public`、`preserve` なら v1 の実効可視性を `public` に写し、
     さらに 1 のドット参照先を `public` にする。fclib は `--lib` で指定し、**preserve** で扱う（利用者が
     複数なので参照の和集合では決まらない）。既定は minimal（Q4 決定）
   - `use X;` で 1 の再輸出判定が真なら `public use X;`
   - `include("*.rb")` の文を削除（castle の `include("macro.rb")` は削除せず警告。§6.3）
   - `loop()` → `loop`
   - switch 内の loop-break: 囲むループに `loop_N:` ラベルを付け `break loop_N;` に（§3.7）
3. **出力**: フォーマッタで印字（v2 はフォーマット済みの状態で始まる）。CRLF は入力に合わせる
4. **検証**（マイグレータ自身が行う）: 移行前・後のプログラムをそれぞれコンパイルし、全モジュールの
   `.s`/`.inc` を比較。差分があれば失敗として報告し、ファイルは書き換えない（`--force` で書く）

castle の `macro.rb` の置換（§6.3）は 2 行の手作業で、マイグレータは「ここに書く」というメッセージを出す:
```
const _T = textmap("../tmp/font/text.chr.txt");
const _M = textmap("../tmp/font/misc_text.chr.txt");
```

---

## 6. 移行の影響（castle / miku / fclib / test）

### 6.1 castle（41 モジュール）

- `private:` 21 箇所 → 削除、以降の宣言はデフォルト private でそのまま。ラベルより前にある宣言（`options` の
  直後など）は v1 では public だったので、minimal では参照されているものだけ `public` が付く
- ドット参照で private に届いていた 28 シンボル → `public` 付与（menu 2 / game 6 / bg 2 / event 14 / sound 2 / my 1 / event2 1）
- `common.fc` の `use nes; use ppu; ...` → `public use ...`（24 モジュールが依存）。`en_vtbl.fc` の
  `use * from en1; ... en8;`: 現行 `Scope.Find` は glob 先の glob も辿る（glob-of-glob 再輸出）が、`en.fc` が
  `en_vtbl` 経由で使うのは `PROCESS` / `NEW_FUNC`（en_vtbl 自身の public 定数）だけで、en1〜en8 のシンボルへの
  直接参照は 0。v2 では glob は再輸出しない（必要なら `public use * from X;`）。castle に他の依存があるかは
  マイグレータの解析で判明する
- `include("macro.rb")` → 手で 2 行（§5）
- switch 内の `break;` 7 箇所（debug_menu 6、title 1）→ 囲むループにラベル + `break L;`（§3.7）

### 6.2 miku（5 モジュール）・test（13）・fclib（10）

- ドット参照の private 依存 0。`private:` 0。minimal で `public` が付くのは実際に使われている宣言だけ
- fclib は preserve（API なので）。`include("stdio.rb")` 3 箇所・`("unittest.rb")` 1・`("math.rb")` 1 →
  削除。`math.fc` に `cos` 関数を追加（マイグレータではなく手で。呼び出し 0 なので asm 不変）
- test の `include("unittest.rb")` 相当は fclib/unittest.fc 側。test 側は `use * from unittest;` のまま

### 6.3 castle `macro.rb` の特別扱い

Ruby ファイルなので AST 変換の対象外。P2 の唯一の例外として、手作業 2 行（§5）で置き換える。
`_T` / `_M` の 71 箇所の呼び出しは無変更。`VERSION_STR` は 2026-09-14 に廃止済み（§3.5）。

---

## 7. 検証計画

1. `fcc migrate` を castle / miku / test の各 main と `--lib fclib` に対して実行
2. 移行後のツリーで `fcc compile` し、全モジュールの `.s`/`.inc` が移行前と一致（P3）。castle は ROM も
3. `go test ./...` の golden: `test/*.fc` は **v1 のまま残し**、v2 版を `test/v2/` に複製して両方回す（Q5 決定。
   v1 パーサの回帰を残すため）。v2 版の ir/allocir/asm は v1 版と一致すること（名前解決の変更はシンボル名・番号に影響しない）
4. 混在: castle の 1 モジュールだけ v2 にしてビルドが通ること（P5）
5. `fcc fmt` が v2 ファイルで冪等・往復同値（既存の `TestFormatCorpus` に v2 コーパスを足す）

---

## 8. 作業順序（見積り）

| # | 作業 | 依存 | 目安 |
|---|---|---|---|
| 1 | `#fc 2` プラグマ、`File.Version` / `ir.Module.Version`、パーサのバージョンゲート（ラベル・`include kind` の拒否） ✅ 2026-09-14 | — | 0.5 日 |
| 2 | sema の可視性規則をバージョンで切り替え（デフォルト private、S7、`public use`）。混在テスト ✅ 2026-09-14 | 1 | 1 日 |
| 3 | `use a, b from mod;` ✅ 2026-09-14 | 1, 2 | 0.5 日 |
| 4 | 組み込み化: `printf` / `unittest_run_tests` 常時登録、`include("*.rb")` を警告化。`textmap` ✅ 2026-09-14（v1 の `include("stdio.rb")` 等は無視。警告の出力経路がまだ無いので警告は出さない。v2 では `include("*.rb")` はエラー） | — | 1 日 |
| 4b | `loop {`、文ラベル、`break`/`continue` の v2 規則（§3.6, §3.7）。v1 モジュールは現行規則のまま ✅ 2026-09-14 | 1 | 0.5 日 |
| 4c | C 型 `for` と `++`/`--`（§3.8）、旧 for の migrate 規則、リポジトリ内 v2 ファイルの再移行 ✅ 2026-09-14 | 4b | 0.5 日 |
| 5 | `fcc migrate`（解析・書き換え・検証） ✅ 2026-09-14（castle 36 ファイル・miku 4 ファイル・test+fclib を一時コピーで移行し、asm 正規化比較と ROM バイト一致を確認） | 1〜4 | 1.5 日 |
| 6 | castle / miku / fclib / test の移行実行と検証、`language_reference.md` の v2 版改訂 — ✅ 2026-09-14 fclib・test（`test/v2/`）・リファレンス改訂。castle / miku はコピーで検証済み（ROM 一致）、リポジトリ内 examples の書き換えはオーナー判断待ち | 5 | 1 日 |

合計 5〜6 日。1〜4 は v1 の挙動を変えないので、それぞれ独立にコミットできる（golden 不変で確認）。

---

## 9. 判断の記録（オーナー、2026-09-14）

- [x] **Q1** `use a from mod as x;`（改名付き選択的インポート）→ **入れない**
- [x] **Q2** `_T("…")` の表記 → **`const _T = textmap(...)` + 呼び出し側無変更。リテラル接頭辞は不要**
- [x] **Q3** `loop()` → `loop` → **変える**
- [x] **Q4** `fcc migrate` の可視性の既定 → **minimal。fclib は preserve**
- [x] **Q5** `test/*.fc` → **v1 のまま残し、v2 版を `test/v2/` に複製して両方テスト**
- [x] **Q6** 見送り一覧からの追加 → **いったんなし**
- [x] **VERSION_STR** → **廃止**。castle 側を固定文字列に変更（§3.5）
- [x] **`break`** → **ラベル付き break を導入し、ラベルなしは switch を抜ける**（§3.7）
- [x] **Q7** `for` → **C 型 `for (init; cond; step)` に変更、旧形式は v2 で廃止**（§3.8、2026-09-14）
- [x] **Q8** `++` / `--` → **追加。文としてのみ**（式の値は持たない）
- [x] **Q9** `for (var i = 0; ...)` の `i` は **`int`**（現行の型推論）。`var i:uint8 = 0` のような型指定も可

確認済み（2026-09-13）:

- `switch` 内の `break;` は v1 では**外側ループを抜ける**（switch は抜けない）。7 箇所（§3.7）
- castle `en` → `en_vtbl` → `en1..8` の glob-of-glob: `en` は `en_vtbl` 自身の public 定数しか使っておらず、
  en1〜en8 への直接参照は 0（§6.1）。他の依存はマイグレータの解析で拾う
- `textmap` の表は現行も 1 つ（`common.fc` の 1 回の include）で、v2 の `const _T = textmap(...)` でも同じ（§4.3）
