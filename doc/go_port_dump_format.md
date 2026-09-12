# golden ダンプ正規形の仕様

Ruby版(オラクル)の `tools/dumper.rb` が定義する形式。Go版はこの形式のダンプを実装し、
`testdata/golden/` と比較する。

> **2026-09-12 (feature/v2 R0) 以降**: golden は Go 自身の出力をスナップショットとして保持し、
> `go test ./internal/fc -run 'TestGolden|TestExample' -update` で再生成する。Ruby オラクル
> (`tools/gen_golden.rb` / `tools/dumper.rb`) は役目を終え凍結。本書の形式は現行 Go 実装 (`irdump.go`)
> の記述として残すが、**ir / allocir の形式は R1-d (型付き IR 導入) で変わる予定**。

移植期の注: この仕様の正典は `tools/dumper.rb` の実装であり、本ドキュメントはその要約だった。

## 基本シリアライズ

| 値 | 形式 |
|---|---|
| nil | `nil` |
| true / false | `true` / `false` |
| 整数 | 10進 (`123` / `-5`) |
| Symbol | `:name` |
| 文字列 | `"..."` バイト単位エスケープ: `\"` `\\` `\n`、0x20-0x7e はそのまま、その他は `\xNN`(大文字16進) |
| 配列 | `(e1 e2 ...)` |
| Hash | `{k1 v1 k2 v2 ...}` 挿入順 |
| Type | `#"<to_s>"` 例: `#"uint8"` `#"uint8*"` `#"uint8[4]"` `#"fastcall uint8(uint8)"` |

## AST ダンプ (`ast/**/*.ast`) — 廃止 (feature/v2 R0-2)

Ruby の S 式構造に紐付いた形式のため、型付き AST への移行に伴い golden ごと削除した。
後継はフォーマッタの冪等性・往復テスト (go_evolution_plan.md F-fmt)。以下は記録として残す。

パース直後(HLC適用前)の生AST。トップレベル文ごとに1エントリ、`pretty_sexp` で整形:
compact形式(要素をスペース区切りで並べたもの)が80文字以下ならその1行、超える配列は
`(` の後に先頭要素(短ければ同一行)、以降の要素を改行+インデント(深さ=スペース数)で並べ `)` で閉じる。

`.pos` ファイル: `pos_info`(構造的等値で collapse される Ruby Hash)のエントリを挿入順に、
行番号のみを1行ずつ出力。

## 値の形式 (IR / alloc-IR 内)

- 現在のLambdaの `vars` にある Value → `{l<index> <id>}` (index は `lmd.vars` 内の位置)
- literal → `{lit <id|nil> <val> <type>[ <base_string>]}` (val: 整数 / `:symbol` / nil)
- array_literal → `{arr <id> <type> (<要素>...)[ <base_string>]}`
- global → `{g <id> <type> <val>[ <base_string>]}` (val: `:symbol` / 文字列 / `mod:<id>` / `macro` / nil)
- module → `{mod <id>}`
- 表にない local → `{l? <id> <type>}`
- CastedValue → `{cast <type> <offset> <内側>}`
- PointeredArray → `{pa <内側>}`
- Lambda参照 → `{lambda <id>}`

## IR ダンプ (`ir/<name>.ir`)

HLC完了直後(LLC適用前)。先頭に `(option <k> <v>)` (hlc.options、挿入順)。
続いてモジュールごと(hlc.modules の挿入順=登録順)に:

```
(module <id>
 (options {<k> <v> ...})
 (include_asms ("..." ...))
 (include_chrs ("..." ...))
 (modules (<id> ...))
 (defs
  (def <sym> <kind> <type> <val>)   ; kind: equ/bss/block/code
  ...)
 (vars
  (var <i> <id> <kind> <type> val=<val>[ lt=<local_type>][ pub])
  ...)
 (lambda <id> <type> opt={...}
  (args (<value>...))
  (result <value|nil>)
  (vars ...)                        ; local は val= なし
  (defs ...)
  (ops
   0000 (<op名> <引数>...)          ; 4桁連番 + op
   ...)
 )...
)
```

## alloc-IR ダンプ (`allocir/<name>.air`)

`Fc.allocate_register` + `delete_unuse` 直後、`optimize_pointer` 適用前。
LLCの処理順(モジュール順 × defs内のcode順、extern はスキップ)に:

```
(alloc-lambda <モジュールid> <sym> <lambda id> frame_size=<n>
 (vars
  (var <i> ... loc=<location>[ addr=<address>][ cond=<reg>,<positive>][ unuse])
  ...)
 (ops
  0000 (<op>...)                    ; delete_unuse で消えた op は nil
  ...)
)
```

## asm (`asm/<name>/<mod>.s|.inc`)

`.fc-build/_<mod>.s` / `_<mod>.inc` から IRコメント行(正規表現 `^\s*; \d{4}:`)を除去し、
末尾に改行1個を付けたもの。それ以外の行(`;;;===` 等の構造コメント含む)は完全一致対象。

## bin / stdout

- `bin/<name>.bin` : emuターゲットの `a.bin` そのまま。`bin/test_basic_nes.nes` : NESターゲット。
- `stdout/<name>.txt` : `fcc run` 相当の実行出力。`stdout/<name>.exit` : 終了コード+改行。

## キー名

emuテストは `<name>`、NESビルドは `<name>_nes`。NES は asm/bin のみ(実行しない)。
