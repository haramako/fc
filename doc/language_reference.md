# FC 言語リファレンス（文法 v2）

FC は NES（6502）向けの静的型付き言語で、C 風の構文を持つ。コンパイラ `fcc` は FC ソースを
ca65 アセンブリに変換し、ld65 でリンクして NES ROM（`-t nes`）または内蔵エミュレータ用バイナリ
（`-t emu`、既定）を作る。

本書は **文法 v2** を正典として記述する。v1 は 2026-09-19 に削除した（差は末尾の「v1 との違い」に
歴史として残す）。
「注」として記す。

---

## 1. ソースファイルとモジュール

- 1 ファイル = 1 モジュール。モジュール名はファイル名から `.fc` を除いたもの（`stdio.fc` → `stdio`）
- 先頭行の `#fc 2` は任意（無くても v2。`#fc 1` はエラー）。`fcc fmt` は書いてあれば残す。開発中の fc 3 は `#fc 3`
  （[v3_plan.md](v3_plan.md)。fc 2 と混ぜられ、`fcc migrate` で fc 2 → fc 3 に書き換える）
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

#### 宣言順と依存関係

トップレベルの `function`・`const`・`var`・`struct`・`soa` と `use` の束縛は、
同じモジュール内で宣言より前から参照できる。`block { ... } options(bss: ...)` 内の変数も対象。
相互 `use` でも、公開宣言を先に収集してから依存関係を解決する。

```fc
const HANDLERS = [draw];
const SIZE = COUNT * sizeof(Item);
var items:[COUNT]Item;

function draw():void { /* ... */ }
struct Item { x:uint8; y:uint8; }
const COUNT = BASE + 1;
const BASE = 3;
```

定数計算、`sizeof`、配列長（多次元配列・関数シグネチャを含む）は、必要な依存先から評価する。
値やサイズの確定に循環があると、`cyclic declaration dependency` と依存経路を報告する。
ポインタ・関数ポインタ・SoA ハンドルでつながる再帰型は、参照先の実体サイズが不要なので許される。
例えば `struct Node { next:*Nodes; }` と `soa Nodes:[16]Node;` はどちらの順にも書ける。

関数内のローカル宣言は従来どおり宣言以降で有効で、実行時の評価順も変わらない。
同名の glob 取り込みの優先順位は引き続き `use` の記述順。
`options` の上書きや `include` の並び、struct のフィールド順は意味を持ち、順序自由化の対象ではない。
この機能は任意の関数をコンパイル時に実行する機能を追加するものではない。

### 1.4 `include` — アセンブラと CHR データ

```
include("util.asm");      // ca65 ソースをこのモジュールのアセンブラ出力に取り込む（.asm / .inc）
include("font.chr");      // CHR データ（拡張子で判定）
```

ファイルは検索パス（ソースのディレクトリ → `fclib/` → `fclib/<target>/`）から探す。

### 1.5 `options` — モジュール属性

ファイルの先頭付近に書く。

```
options(bss: "BSS_EX");       // このモジュールのグローバル変数・可変 soa の既定セグメント（§4.1）
options(bank: -1);            // 配置バンク（負数は末尾から）
options(org: 0xa000);         // 配置アドレス
options(mapper: "MMC3");      // iNES マッパ（"MMC0" / "MMC3" / 番号）— メインモジュールで
options(bank_count: 4);       // PRG バンク数 — メインモジュールで
options(char_banks: 1);       // CHR バンク数 — メインモジュールで
options(fastcall_reg: 16);    // extern の fastcall 関数が使うゼロページ領域 FC_FASTCALL_REG の大きさ（既定 16、16〜128）— メインモジュールで
options(static_zp: 64);       // 静的フレーム（§4.5）のゼロページ側 FC_SZP の大きさ（既定 64、0〜256）— メインモジュールで
options(static_ram: 512);     // 静的フレームの RAM 側 FC_SRAM の大きさ（既定 512、0〜8192）— メインモジュールで
options(farcall: true);       // far call（§4.4）を有効にする — メインモジュールで
options(near: true);          // このモジュールは常にマップされている扱い（far call の対象にしない）
options(base: "data.asm");   // fc の base.s を生成せず、このファイル（Dir 相対）をアセンブルして土台にする — メインモジュールで
options(linker_config: "../ld65.cfg"); // 自前のリンカ設定（Dir 相対）。fc は ld65.cfg を生成しない — メインモジュールで
options(link: "../res/sound/bgm.o ../nsd/lib/NSD.lib"); // 追加でリンクするオブジェクト / ライブラリ（空白区切り、Dir 相対）— メインモジュールで
```

`base` を自前で持つプロジェクトは、fc の領域（`L` / `reg` / `FC_FASTCALL_REG` / `FC_SZP` / `FC_SRAM` / `FC_SP` / `FC_FARCALL`、
iNES ヘッダ、VECTORS）を同じ名前で定義する（examples/castle/src/data.asm が例。不足はリンク時の `.assert` で検出される）。
この 3 つで、独自のバンク構成やサウンドドライバを持つプロジェクトも `fcc build -t nes -o game.nes main.fc` だけでビルドできる
（`-g` / `--size-report` / `fcc watch` もそのまま使える）。

`bank: N` は fc が ld65.cfg を生成する構成では配置先のバンク。自前の ld65.cfg を使う構成では配置に使われないが、
far call の判定に「N ≥ 0 なら切替バンク、無しか負なら固定バンク」として使われる（§4.4）。

---

## 2. 型

| 型（fc 3 / fc 2 の名前） | サイズ | 範囲 | 備考 |
|---|---|---|---|
| `u8`（fc 2 は `int` / `uint` / `int8` / `uint8` も） | 1 | 0〜255 | |
| `i8`（fc 2 は `sint` / `sint8` も） | 1 | -128〜127 | |
| `u16`（fc 2 は `int16` / `uint16` も） | 2 | 0〜65535 | |
| `i16`（fc 2 は `sint16` も） | 2 | -32768〜32767 | |
| `bool` | 1 | | `true` / `false`、比較演算（`==` `!=` `<` …）と論理演算（`&&` `\|\|` `!`）の結果の型。値は 0 / 1。整数（`uint8`）と互換で、`if` の条件や `&`、整数の引数・戻り値・代入にそのまま使える（`var f = a < b;` の `f` は `bool`。`int` にしたいなら `var f:int = a < b;`）。整数との相互変換はビット列そのままで、整数を bool に入れても 0/1 に正規化しない（非ゼロ = 真。`b == true` ではなく `if (b)` で判定する） |
| `void` | 0 | | 戻り値なし |

fc.toml の `[target]`（mapper / prg / chr）と `[bank.<名前>]`（slot / index / segments）、`[ram.<名前>]`（start / size）、
`[linker] extra`（ld65.cfg の断片）を書くと、fc が ld65.cfg と iNES のヘッダを作る。モジュールは `@(bank: "名前")`（"fixed" は
常に見えている領域）、手動の切り替えの番号は `@bank("名前")`。マッパーは NROM / MMC3 / UxROM / MMC1（[v3_plan.md](v3_plan.md) §3）。

fc 3 の `@if (条件) { … } else @if (…) { … } else { … }` はコンパイル時に片方を選ぶ（条件はリテラルと `@(build)` の const だけ。
選ばれなかった側は名前解決しない。トップレベルでも関数の中でも書け、新しいスコープは作らない）。`const DEBUG = false @(build);` は
fc.toml の `[define.<モジュール名>]` と `fcc build -D モジュール名.DEBUG=true` で値を上書きできる（[v3_plan.md](v3_plan.md) §1）。

fc 3 では `options(...)` を `@(...)` と書く（真偽値の属性は `@(inline)` のように値を省ける。宣言の後ろならその宣言の属性、
`@(...);` はモジュールへの指定、`@(bss: "…") { … }` は中の宣言の既定値。[v3_plan.md](v3_plan.md) §5 C）。

fc 3 では組み込みを `@sizeof(T)` / `@bitcast(T, x)` / `@incbin("f")` / `@include("f", key: value, …)` / `@asm(…)` /
`@textmap(…)` / `@min` / `@max` / `@clamp` / `@run_tests()` と書き、`sizeof` / `min` などは普通の名前として使える
（[v3_plan.md](v3_plan.md) §5 A）。

fc 3 では整数型の名前は `u8` / `i8` / `u16` / `i16` だけで、fc 2 の名前はエラー（`fcc migrate` が書き換える）。`u8` / `i8` /
`u16` / `i16` は fc 3 では変数・関数・型などの名前として宣言できない。エラーメッセージ・IR のダンプの型の表示はどちらの版でも
短い名前（[v3_plan.md](v3_plan.md) §7）。

fc 3 の `[]T` / `[]const T` は slice（先頭のポインタと長さ `u8` の 3 バイトの値。要素は 255 個まで）。長さを初期値から決める配列は
`[?]T` と書く（fc 2 の `[]T`。`fcc migrate` が書き換える）。長さの分かる配列は slice に暗黙に変換でき（文字列リテラルは終端の 0 を
含めない長さ）、`s[i]` は範囲を検査しない。`a[lo..hi]` / `a[lo..]` / `a[..hi]` / `a[..]` は配列・slice の一部（定数の範囲だけ検査）。
`@len(x)` は配列の長さ（定数）・slice の長さ・enum のメンバーの数、`@slice(p, n)` はポインタから、`@ptr(s)` は先頭のポインタ、
`@copy(dst, src)` は短いほうの長さだけ写して要素数を返す（`use mem;` が要る）。const の配列を `[]T`（`[]const T` でない）に入れると
警告。const の表の中（struct のフィールド・配列の要素・`const S:[]const u8 = "..."`）では、配列・文字列のリテラルと
配列の定数・グローバル変数の名前を slice にでき、{ 先頭, 長さ } の定数になる（リテラルは無名の配列定数に切り出す。
`const T:[?]Data = [{items: [{1, 4}, {2, 3}], id: 50}]`、ジャグ配列 `const NAMES:[?][]const u8 = ["ab", "cde"]`）。
const の struct のポインタのフィールドにも、配列の定数・リテラルのアドレスを入れられる。長さ `u16` の広い slice は `[:u16]T` / `[:u16]const T`（4 バイト。メモリのコピーなど 256 要素
以上に使う。`[:u8]T` は `[]T` と同じ）。`[]T` → `[:u16]T` は暗黙、逆は `@slice(@ptr(s), n)`。256 要素以上の配列の範囲と、長さが
`u16` の `@slice(p, n)` は広い slice（[v3_slices_vector.md](v3_slices_vector.md)）。

fc 3 の `@null_fn` は何もしない関数（`rts` だけ。share/runtime.asm）。戻り値の無い関数の型なら、引数・fastcall・farfn に
よらずどれにでも入る（`ppu.irq_setup = @null_fn;`。型は `null` と同じく文脈から、無ければ `fn():void`）。呼ぶ側が引数を積み、
呼び出しの後でレジスタを戻すので、`rts` だけでどの型としても呼べる。戻り値のある型はエラー、直接の呼び出し
`@null_fn();` もエラー（何もしないので消す）。

fc 3 の `switch` は case / default ごとにスコープを作る（case の中で宣言した変数は、ほかの case と switch の後ろからは
見えない。fc 2 は囲むスコープに宣言していた。`fcc migrate` は書き換えないので、そう使っていたソースは fc 3 でエラーに
なる。宣言を switch の前に移す）。`fallthrough;` を case の最後の文に書くと、次の case（最後の case なら default）の本体へ
値の検査をせずに進む（default と、default の無い最後の case からはできない）。

fc 3 の `@log("HP: {} / {}", hp, max_hp);` は、エミュレータ側で表示するログ（NES 側の命令・サイクル・ROM は変わらない）。
書式は `{}`（順番）/ `{0}`（位置）と `{:x}` / `{:04X}` / `{:b}` / `{:c}` / `{:d}`、`{{` / `}}`。引数は変数（グローバル・
ローカル・引数）・定数・struct のフィールド・定数添字の要素で、enum はメンバー名、bool は true / false、ポインタは `$1234`
と出る。`fcc build -g` のとき ROM の隣に `.fclog.json` と Mesen 2 用の `.fclog.lua` を書き、`fcc run -g`（emu）は出力に
混ぜて出す。最適化で値が取れない地点は「?」（コンパイル時に警告）（[v3_plan.md](v3_plan.md) §9）。
| `[N]T` | N×size | | 配列。`[]T` は長さ省略（初期値か `address` から決まる） |
| `*T` | 2 | | ポインタ。`null`（v2）を入れられる（ポインタと関数ポインタのみ。SoA ハンドルには null なし） |
| `*void` | 2 | | 何を指すか問わないポインタ（v2）。どのポインタ・関数ポインタ・配列も暗黙に入る。戻すには `bitcast<*T>(p)`。参照はがし・添字・算術は不可 |
| `fn(T1, T2, ...):R` | 2 | | 関数型（値は関数のアドレス）。`fastcall` 属性は型の一部（表示名 `fastcall fn(...):R`） |
| `farfn(T1, T2, ...):R` | 3 | | バンク付き関数ポインタ（アドレス下位・上位・バンク）。呼び出しには `options(farcall: true)` が必要 |
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

#### 型付きストレージ alias

既存のグローバル領域を、別の型の変数として読み書きできる。RAM の追加確保や宣言時のコピー・初期化は行わない。

```fc
var memblock:[32]uint8;
struct HogeWork { x:int; y:int; params:[4]int; }
alias work:HogeWork = memblock;

function init():void {
    work = {x:1, y:2}; // params を含む6バイトを初期化。残り26バイトはそのまま
}
function update():void {
    alias h:HogeWork = memblock; // 関数内でもグローバル領域への別名
    h.x += h.y;
}
```

- 構文は `[public] alias 名前:型 = 対象;`。型は必須。対象はグローバル var または他のストレージ alias の名前
  （`shared.work` のようなモジュール修飾も可）。対象の先頭から、宣言した型のサイズ分を参照する。
- 型・対象はサイズが確定した非空の領域で、alias のサイズが直接の対象のサイズ以下であることを検査する。
  alias の連鎖でも中間の alias より大きい範囲へ拡張できない。
- 固定フィールドへのアクセスは最適化レベルによらず直接アクセスになる。alias 用のポインタ変数は作らない。
  配列の可変添字や `&work` で取得したポインタの利用には、通常の計算・参照のコストがある。
- `&work` は `*HogeWork`、`sizeof(work)` は `sizeof(HogeWork)`。値代入・引数・戻り値は通常の値コピー。
  alias 自体の再束縛はなく、`work = ...` は実体の内容を書き換える。
- 配置と volatile 性は実体から継承する。alias 独自の options は指定しない。BSS 配置ブロック内にも置かない。
- モジュール直下では後方宣言・相互参照に対応し、循環する alias はエラー。`public alias` はモジュール直下のみ。
  関数内では宣言以降で利用できる。各モジュールは共有元だけを use すれば、独自の型で別名を宣言できる。
- 初版では対象へのフィールド指定・配列添字・ポインタ参照、ローカル変数・ROM 定数・SoA は扱わない。
  `alias` は宣言の形でのみ特別扱いし、既存の変数名や関数名としても使用できる。
- 別名を通した書き込みは元の変数や他の別名にも反映される。用途の切替・排他・初期化は利用者が管理する。

詳細は [型付きストレージ alias](v2_storage_alias.md)。アクティベーションによる有効期間の管理は未実装の別案。

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
->fn(a:int):int { return a + 1; }  // ラムダ（無名関数）
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
const M:[2][3]int = [[1, 2, 3], [4, 5, 6]];   // 二重配列（M[i][j]）
const PS:[3]*int = [S, T, "xyz"];   // ポインタの配列（要素は配列定数の名前・文字列・null・関数。アドレスが .word で並ぶ。
                                    // `options(address:)` で asm のシンボルに束縛した配列定数も要素にできる）
var a:int, b:int;              // まとめて宣言
```

グローバル変数の属性:

```
var vram:int options(address: 0x2007);        // 固定アドレス（メモリマップド I/O。自動的に volatile。数値だけ）
var flag:int options(volatile: true);         // 割り込みや asm が書き換える変数（下記）
var buf:[256]int options(segment: "BSS_EX");  // 配置セグメント
var cnt:int options(symbol: "_counter");      // fc が確保する領域のシンボル名を固定（asm から参照する）
const TBL:[]int = [1, 2] options(symbol: "_tbl"); // 配列定数のシンボル名を固定
const BGM0:[]int options(symbol: "_nsd_bgm_BGM0"); // 値なし: アセンブラ側の定義を参照（関数の本体なしと同じ規則）
```

**BSS の一括指定**: モジュール全体は `options(bss: "...");`、一部の宣言は配置ブロックで指定する。

```fc
options(bss: "BSS_EX");
var large:[256]int;                           // BSS_EX

block {
    public var a:int;                         // IRQ_DATA
    var b:int;
    block {
        var c:int;                           // SCRATCH
    } options(bss: "SCRATCH");
    var small:int options(segment: "BSS");   // 個別指定を優先
} options(bss: "IRQ_DATA");

var other:int;                               // 再びモジュールの BSS_EX
```

優先順位は **個別宣言の `segment` > 最も内側のブロックの `bss` > モジュールの `bss` > `BSS`**。
モジュールの指定はファイル全体に適用される（宣言より後に書いても同じ。複数指定は最後の値）。
他モジュールへは伝播しない。個別宣言では引き続き `segment:` を使い、`bss:` は指定しない。
ブロックでは `bss:` だけを受け付け、`segment:` などはエラーにする。

配置ブロックはモジュール直下または別の配置ブロック内に置き、`var`・可変 `soa`・入れ子の配置ブロックを含められる。
名前空間は増えず、内側の変数は通常のモジュール変数として参照する。`public` の意味も変わらない。
`block` はこの構文でだけ特別扱いし、既存の同名変数・関数・型を禁止しない。
ブロックの末尾には `;` が必要。関数・`const`・`soa const`・`use`・`include`・単独の `options` 文はブロックに入れられない。

BSS 指定は固定アドレス変数、関数内ローカル、コード、ROM 定数の配置に影響しない。
可変 `soa` は展開した各フィールド配列に継承する。配置ブロックは連続配置・ページ内配置を保証しない。
`bss` には空でないセグメント名の文字列を指定する。制御文字・引用符・バックスラッシュは使えない。
そのセグメントのメモリ領域は ld65.cfg で定義する（標準設定にない名前は自前の cfg が必要）。
ゼロクリア・保存領域の扱い・リンカ設定は自動では変更しない。詳細は [V2 BSS 配置](v2_bss.md)。

`options(symbol: "name")` は関数（§4.2）・変数・配列定数に共通で、「定義があればその名前で出力し、無ければ
アセンブラ側の定義を参照する」。参照のときは fc が `.global name` を出すので、同じモジュールに `include` した asm で
定義したものでも別のオブジェクトファイル（NSD の BGM データなど）でもよく、asm 側に `.global` / `.export` を書く必要は
ない。`address:` は数値の固定番地専用（以前の `address: "sym"` は `symbol: "sym"` に）。

**volatile**: 最適化はグローバル変数の値をループの間レジスタに置いたままにすることがある（§4.5 の常駐）。
読むたび / 書くたびに意味がある変数はそれをしてはいけないので、次の変数は volatile として扱われ、常に
メモリを読み書きする: (1) `options(address:)` の固定アドレス（メモリマップド I/O）、(2) インラインアセンブラや
`include` したアセンブラファイルからシンボルで参照される変数（割り込みハンドラや asm ルーチンが書き換える）、
(3) `options(volatile: true)` を付けた変数。関数呼び出し・ポインタ経由の書き込み・インラインアセンブラをまたぐ
ときは volatile でなくてもメモリに書き戻して読み直すので、普通のグローバル変数は指定なしで正しく動く。
明示が要るのは、fc からは見えない経路（別にアセンブルする asm、DMA など）で値が変わる変数だけ。

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

引数の末尾には、定数式のデフォルト値を指定できる。

```fc
function draw(x:uint8, y:uint8, color:uint8 = DEFAULT_COLOR):void { ... }
const DEFAULT_COLOR = 1;

draw(10, 20);       // draw(10, 20, DEFAULT_COLOR) と同じ
draw(10, 20, 3);    // 明示した値を使う
```

- デフォルト付き引数の後ろには、必須引数を置けない。途中の引数だけの省略や名前付き引数は扱わない。
- デフォルト値は宣言側のスコープで解決する。後方の定数も使え、別モジュールや呼び出し側の同名変数に影響されない。
  数値・定数計算・`sizeof`・`null`・関数シンボル・文字列や配列・struct の定数に対応する。
  実行時の関数呼び出し、変数の読み出し、他の引数の値への依存は不可。
- 省略分を呼び出し側で補い、型変換・引数の受け渡しは明示指定時と同じ。ABI、`fn` / `farfn` のサイズは変わらない。
  デフォルト式は呼ばれない関数や全引数を明示する場合も検査する。
- デフォルト値は関数宣言の情報であり、関数型には含めない。関数シンボルへの直接呼び出しとその定数別名では省略可能。
  `fn` / `farfn` の変数、関数表、戻り値を介する呼び出しには全引数が必要。最適化によってこの条件は変わらない。
- 本体なしの extern 宣言にも指定できる。ラムダ・関数型の引数欄にはデフォルト値を指定しない。
- 文字列・配列リテラルをポインタのデフォルトにすると、宣言側モジュールの ROM に格納する。
  名前付き配列定数は元のアドレスを使う。データのバンク管理は通常のポインタ引数と同じく呼び出し側の責任。

- 引数・戻り値の型は必須。`void` 関数は `return;`、それ以外は `return expr;`。
  非 void 関数は本体が必ず `return` で終わらなければならない（最後の文が `return`、両枝が `return` で終わる
  `if`/`else`、`break` の無い `loop`・`while (1)`・`for (;;)`、`default` 付きで全 case が `return` で終わる `switch`）
- `fastcall`: **本体を持つ関数では意味を持たない**（非再帰の関数は全部静的フレーム §4.5 になる。互換のため受理する。
  以前の「中から他の関数を呼べない」制限も無い）。本体の無い extern 関数（asm 定義）に付けると、引数・戻り値を
  ゼロページの `FC_FASTCALL_REG`（既定 16 バイト、`options(fastcall_reg: N)`）で渡す規約になる。
  base.asm を自前で持つプロジェクトは `FC_FASTCALL_REG: .res N` と `FC_FASTCALL_REG_SIZE = N`（`.export … : absolute`）を
  合わせる（不足はリンク時の `.assert` で検出される）
- `options(abi: "stack")`: 静的フレームにせず、スタック（`S+n,x`）の規約のままにする（§4.5）
- `options(interrupt: true)`: 割り込みハンドラから呼ばれる関数（§4.5）
- `options(zeropage: false)`: 静的フレームを RAM 側に置く
- `options(symbol: "...")`: 生成するシンボル名を固定する（割り込みベクタなど）。本体なしなら asm 側の定義の参照で、
  fc が `.global` を出す（同じモジュールに `include` した asm でも別のオブジェクトファイルでもよい）
- `options(near: true)`: far call（§4.4）の対象にしない（呼ぶ側はマップ済みと仮定して `jsr` する）
- `options(abi: "cc65")`: 本体の無い extern 関数を **cc65 の `__fastcall__` 規約**で呼ぶ（NSD など cc65 向けの asm
  ライブラリ用）。引数は 0 か 1 個で、1 バイトなら A、2 バイトなら A（下位）/ X（上位）で渡す。戻り値は void か
  1 バイト（A）/ 2 バイト（A/X）。2 個以上の引数は cc65 のパラメータスタックが要るので不可。呼び先は A/X/Y を壊してよい。
  アドレスは取れない（関数ポインタ不可）。別バンクからは呼べない（far call のトランポリンが A/Y を壊す。常にマップされて
  いるなら `near: true` を付ける）
- `options(inline: true)`: 呼び出しをその場に展開する（`jsr`/`rts` と引数の受け渡しが消え、展開先で最適化される。
  `abs` / `rand` / 数命令の I/O ラッパ向け）。本体を持ち、再帰でなく、`interrupt` でないこと。展開しない呼び出しも
  ある（結果は同じ）: 別のモジュールの関数で本体に呼び出しを含むもの（far call の判定が呼び先のモジュール基準なので）、
  他の呼び出しの引数の中（`g(f(x))` の `f`）、関数ポインタ経由、**別の切替バンクの関数で本体が ROM のデータ（const の表、
  文字列）を読むもの**（コードだけ写すと表は元のバンクに残り、far call のバンク切替も消えて別の表を読む。RAM の変数だけを
  触る本体は写す）。どこからも呼ばれなくなった関数は出力されない（§4.7）。
  印が無くても、本体が小さく（12 命令以下）ループ・呼び出し・asm・アドレス取得・配列 / struct のローカルが無い関数は
  自動で展開される（6 命令以下は常に、それより大きいものは呼び出しが 2 か所以下のとき）
- `options(noinline: true)`: 自動インラインの対象にしない（呼び出しのまま残す）

### 4.4 far call（バンクをまたぐ呼び出し）

### 4.5 呼び出し規約と静的フレーム

fc で本体を持つ関数のうち**再帰しないもの**は、引数・戻り値・ローカルを固定アドレスのフレーム（`F_<sym>`）に持つ。
同時に活性になりえない関数（呼び出しグラフで一方から他方へ届かない関数どうし）のフレームは重ねて置かれるので、
使う領域は「呼び出しの連鎖 1 本分の合計」程度で済む。領域はゼロページ側 `FC_SZP`（`options(static_zp: N)`。
呼び出しの深い関数から順に入るだけ入る）と RAM 側 `FC_SRAM`（`options(static_ram: N)`）。
fc が生成する base.asm のゼロページ配置は `$00-$0F` L（stack 関数のレジスタ領域）、`$10-$1F` reg、`$20-$2F` FC_FASTCALL_REG、
`$30-$6F` FC_SZP、`$70-$7F` は `options(segment: "ZEROPAGE")` の変数用、`$80-$FF` スタック S。**`$00-$7F` に
`options(address:)` で固定番地の変数を置いてはいけない**（fc の領域と重なる。固定番地の ZP 変数が要るプロジェクトは
castle のように base.asm を自前で持つ）。リンクの後に、`@(address:)` の変数が RAM のセグメント（fc の ZP・BSS・スタック、
fc.toml の `[ram.*]`、ほかの変数）と重なっていないかを確かめ、重なればエラー。fc.toml の `[ram.*]` も fc の領域（`$00-$01FF`、
`$0200-$06FF`）と重なればエラー（OAM は `$0700` かカートリッジの RAM に置く）。重なりを意図するなら storage alias を使う。
base.asm を自前で持つプロジェクトは `FC_SZP: .res N` / `FC_SRAM: .res M` と `FC_SZP_SIZE` / `FC_SRAM_SIZE` の
`.export … : absolute`、それにスタックの空き先頭 `FC_SP: .res 1`（`.exportzp`）を合わせる（不足はリンク時の `.assert` で
検出される。配置は `.fc-build/_frames.inc`）。

| 種類 | 対象 | 引数の渡し方 |
|---|---|---|
| static | 本体を持つ非再帰の関数（既定） | 呼び出し側が `F_g+k` に直接書き `jsr` |
| entry | static のうち、アドレスを取られた関数（関数ポインタ・`const` の表・インラインアセンブラからの参照）と `options(interrupt: true)` | スタック経由（下の stack と同じ）。プロローグで自分のフレームに写す |
| stack | 再帰する関数、`options(abi: "stack")`、本体の無い extern 関数 | スタック `S+k,x`（呼び出し側が `ldx FC_SP` で X をスタックの空き先頭にしてから書く）。extern 関数は X を保存すること |
| fastcall | extern で `fastcall` 指定 | `FC_FASTCALL_REG` |
| cc65 | extern で `options(abi: "cc65")` | 唯一の引数を A（1 バイト）/ A,X（2 バイト）、戻り値を A / A,X（cc65 の `__fastcall__`。§4.2） |

再帰の判定は呼び出しグラフの閉路で、関数ポインタ経由の呼び出しは「同じ関数型でアドレスを取られた関数の全部」
への呼び出しとみなす。`bitcast` で関数ポインタの型を変えて呼ぶ再帰は検出できない（`options(abi: "stack")` を付ける）。
`options(interrupt: true)` の関数から届く関数は全部 static でなければならず（X が何を指すか分からないため）、
そのフレームは他のどの関数とも重ねない。

### 4.7 使われない関数の除去

`main`、`options(interrupt: true)` / `options(symbol: ...)` の関数、アドレスを取られた関数（関数ポインタへの代入、
`const` の表、`include` した asm やインラインアセンブラからの参照）から呼び出しをどう辿っても届かない関数は、
コンパイルはされるが**出力されない**（コードも静的フレームも消える。`public` でも同じ: プログラム全体で判断する）。
`fcc build -d` の要約に `unused (not emitted): N functions: ...` と出る。asm 側から `jsr` したい fc の関数は
`options(symbol:)` を付けるか、asm ファイルを `include` してその中で参照する。出力されない関数のインラインアセンブラは
ca65 に渡らないので、その中の誤りは検出されない（fc の型検査などは通常どおり行われる）。

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
- 通常の `fn` 経由ではバンクは呼ぶ側が管理する。`farfn` 経由ではトランポリンで切替・復帰する。
  どちらも他バンクのデータ参照や割り込みからの安全な再入は管理しない。
- `fcc build -d` で far call になった箇所の一覧が出る

設計の経緯は [v2_farcall.md](v2_farcall.md)。

### 4.2.1 バンク付き関数ポインタ

```fc
options(farcall: true);
use events;
var callback:farfn(uint8):void;

function setup():void {
    callback = events.run;
}
function update():void {
    if (callback != null) {
        callback(1);
    }
}
```

関数シンボルを型の指定された文脈に入れると、リンク時にアドレスと `.bank(symbol)` を組にする。
`var f = events.run;` とだけ書けば従来の 2 バイトの `fn`。既存の fn **変数**からバンクは復元できないため、
farfn への暗黙変換はできない。farfn → fn / `*void` / 整数への暗黙変換も不可。

配列、struct / SoA のフィールド、引数、戻り値、コピーに対応する。`==` / `!=` はバンクを含む 3 バイトを比較し、
`null` は全バイト 0。null 呼び出しの実行時検査は行わない。算術・順序比較は不可。
呼び先は引数より先に評価して保持し、引数の評価後に `FC_FARCALL` へ転送する。

明示的な farfn は `near` 属性の有無によらず実配置のバンクを使う。バンク番号は 0..255（リンク時検査）。
呼び出しは通常の stack / Entry ABI を使い、cc65 ABI / legacy fastcall の関数は格納できない。
型の宣言やコピーだけなら `options(farcall: true)` は不要。
詳細・制約・検証は [farcall 対応の関数ポインタ](v2_far_function_pointers.md)。

### 4.3 ラムダ

```
const add2 = ->fn(a:int):int { return a + 2; };
var cb:fn():void = ->fn():void { ... };
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
```

`switch` は case を上から `cmp; bne` で比べる（値ごとに 5 サイクル）。整数の case が **10 個以上あって密に並んでいる**
（最大 − 最小 + 1 が case の数の 2 倍以下、かつ 255 以下。タグは 1 バイト）ときはジャンプテーブルになり、case の数に
よらず約 33 サイクルで飛ぶ（飛び先 − 1 の表を `pha; pha; rts` で辿る。表は 1 値あたり 2 バイト）。

```
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

- `a[i]`: 配列/ポインタの添字。添字は 1 バイト（2 バイトの添字は不可）。範囲外の添字は未定義。添字の式 `i + k`（k は
  定数）は 8 ビットで折り返さないものとみなす（`i + k > 255` は範囲外と同じ扱い。最適化が `a[i + k]` を「`a` の k 先を
  添字 `i` で」に畳むため。折り返しを当てにする `[256]` のリングバッファは、添字の式でなく変数で進める:
  `j = i + k; a[j]`。変数の加算は普通に折り返す）
- `*p` / `&x`: ポインタ演算。`p + n` / `p - n` / `p++` は要素 n 個分進める（`*T` なら n × `@sizeof(T)` バイト。`p[n]` と
  `*(p + n)` は同じ）。同じ型のポインタの差 `p - q` は要素数（u16）。`*void` の算術はできない（2026-09-26 まではバイト単位だった）。
  ポインタの加減算は同じ配列（または変数）の中と末尾の 1 つ先までで意味を持つ。
  それを超えて進めた値は未定義（C と同じ。最適化はこれを前提にする: 例えばループのカウンタとポインタを 1 つにまとめる
  ときに「ポインタ + 残りの要素数」が折り返さないとみなす）
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
| `printf(a, b, ...)` | 引数の型で `stdio.print`（`*u8`）/ `print_slice`（`[]u8`。長さの分だけ）/ `print_int16`（符号なしの整数・bool・enum）/ `print_sint16`（符号付き。負なら `-`）を呼び分ける。それ以外の型（struct など）はエラー。`stdio` モジュールが必要 |
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
| `mem` | `set(dst, value, size)`, `zero(dst, size)`, `copy(dst, src, size)`, `compare(a, b, size)`（size は u16。0 なら何もしない / 等しい）, `strcpy(dst, src)` など |
| `math` | `sin(x)`, `atan(y, x)`, `rand()`, `sign(i)`, 乗算テーブル |
| `nes`（NES） | PPU / APU / コントローラのレジスタ定義 |
| `pad`（NES） | コントローラ入力 |
| `unittest` | `assert_true(cond, msg)`, `assert_equal(a, b, msg)` |
| `rle` / `lzw` / `inflate` | 圧縮データの展開 |

---

## 9. v1 との違い（歴史）

v1 の処理系と `fcc migrate` は 2026-09-19 に削除した（v1 のソースが残っていれば fc 0.1 系の `fcc migrate` で
変換する）。v1 だけにあった構文（後置の型、`<T>x`、`loop()`、`for (i, from, to)`、`public:` ラベル、
`include macro(...)`）は今も文法が受理して「v2 ではこう書く」というエラーを出す。

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
