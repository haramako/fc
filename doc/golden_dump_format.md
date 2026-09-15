# golden ダンプ正規形の仕様

`internal/driver` の golden テスト（`testdata/golden/ir`, `allocir`）が比較する IR ダンプ（`internal/ir/irdump.go`）の形式。
golden は Go 自身の出力をスナップショットとして保持し、`go test ./internal/driver -run 'TestGolden|TestExample' -update` で
再生成する。

## 基本シリアライズ

| 値 | 形式 |
|---|---|
| nil | `nil` |
| true / false | `true` / `false` |
| 整数 | 10進 (`123` / `-5`) |
| シンボル | `:name` |
| 文字列 | `"..."` バイト単位エスケープ: `\"` `\\` `\n`、0x20-0x7e はそのまま、その他は `\xNN`(大文字16進) |
| 配列 | `(e1 e2 ...)` |
| マップ | `{k1 v1 k2 v2 ...}` 挿入順 |
| Type | `#"<String()>"` 例: `#"uint8"` `#"*uint8"` `#"[4]uint8"` `#"fastcall fn(uint8):uint8"`（2026-09-14 から v2 の前置形。以前は `uint8*` `uint8[4]` `fastcall uint8(uint8)`） |

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
