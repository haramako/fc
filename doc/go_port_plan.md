# fc Go 移植計画

対象ブランチ: `agent/golang`
Ruby版(現行)を「正解オラクル」として、**生成アセンブラの一致**を全フェーズの合格条件とする厳密クローン移植。

> **このドキュメントは進行管理を兼ねた生きた計画書です。**
> セッションをまたぐ場合、まずこのファイルを読み「進捗サマリ」と「次にやること」から再開する。
> フェーズ完了ごとにチェックボックス・進捗サマリ・作業ログを更新してコミットすること。

---

## 進捗サマリ

最終更新: 2026-08-28 / 現在のフェーズ: **Phase 7 (未着手)**

| Phase | 内容 | 状態 | 完了日 |
|---|---|---|---|
| 0 | 基盤整備・差分ハーネス | ✅ 完了 | 2026-08-28 |
| 1 | レキサ + パーサ | ✅ 完了 | 2026-08-28 |
| 2 | 基盤データ構造 | ✅ 完了 | 2026-08-28 |
| 3 | HLC (AST→IR) | ✅ 完了 | 2026-08-28 |
| 4 | レジスタ割付 | ✅ 完了 | 2026-08-28 |
| 5 | LLC (IR→asm) | ✅ 完了 | 2026-08-28 |
| 6 | ドライバ・テンプレート | ✅ 完了 | 2026-08-28 |
| 7 | 6502エミュレータ | ⬜ 未着手 | - |
| 8 | テスト移行 | ⬜ 未着手 | - |
| 9 | 切替 | ⬜ 未着手 | - |

状態記号: ⬜ 未着手 / 🔄 進行中 / ✅ 完了 / ⏸️ 保留

**次にやること**: Phase 0 の最初の未チェック項目から。

---

## 自律実行ルール

本移植は**エージェントが完全自律で最後まで実行**する。以下を厳守すること。

1. **合格条件を満たすまで次のフェーズに進まない。** 各フェーズ末尾の検証コマンドが通ることが唯一の判定基準。「たぶん動く」で先へ進まない。
2. **フェーズ完了時に必ず**: チェックボックスを `[x]` に、進捗サマリの状態を ✅ と完了日に、作業ログに1行追記 → その上でコミット。
3. **コミット**: `agent/golang` に直接。フェーズ完了ごとに1コミット。メッセージは `go-port: Phase N <内容>`。`master` へのマージは Phase 9 でまとめて行う。
4. **Ruby版は完全凍結（例外なし）**。`lib/` `bin/fcc` `fclib/` `share/` を変更してはならない（変更するとオラクルが壊れる）。ダンプは `tools/dumper.rb` がモンキーパッチで取得するため、Ruby版本体の変更は不要になった。タグ `ruby-frozen` が凍結時点。
5. **生成物はコミットしない**: `.fc-build/` `a.*` `coverage/`（すべて `.gitignore` 済み）。`testdata/golden/` は**コミットする**。
6. **詰まったときの手順**:
   1. Ruby版の該当箇所を読み直して挙動を正確に再現する（Ruby版が仕様書）
   2. 差分が出たら差分の少ないファイルから1件ずつ潰す。まとめて直そうとしない
   3. 3回試して解決しない場合は、その項目を ⏸️ 保留にして作業ログに理由を書き、**同フェーズ内の他の項目を先に進める**
   4. **停止して質問してよいのは**、計画に記載のない設計判断が必要になった場合のみ（例: Ruby版の挙動自体がバグに見えるが仕様として再現すべきか判断できない）
7. **リグレッション禁止**: 一度通ったフェーズの検証コマンドは、後続フェーズでも通り続けること。壊したらその場で直す。

---

## 確定方針

| 項目 | 決定 |
|---|---|
| Rubyマクロ機構 | **Go組み込みマクロに固定**。`printf`/`times`/`cos`/`unittest_run_tests` をGoネイティブ実装し、`include("stdio.rb")` 等をファイル名キーでそのGo実装に解決する。fclib の `.fc` ソースは無変更。 |
| 忠実度 | **厳密クローン優先**。設計改善・最適化の見直しは移植完了後に別途。 |
| asm比較 | **IRコメント行を除外して比較**。`^\s*; \d{4}:` に一致する行を両者から除去してから比較する。それ以外の行（`;;;===` 等の構造コメント含む）は完全一致させる。Go版は独自形式のIRコメントを出してよい。 |
| オラクルのダンプ | **`tools/dumper.rb` のモンキーパッチで取得**（Ruby版本体は完全無変更）。ASTはパース直後の生AST、IRはHLC完了直後、alloc-IRは割付+delete_unuse直後(optimize_pointer前)。正規形は `doc/go_port_dump_format.md`（正典は dumper.rb 実装）。 |
| Goコード配置 | **リポジトリ直下**。`go.mod` は `C:\Work\fc` 直下、module path は `github.com/haramako/fc`。`fclib/` `share/` を `embed` で直接取り込む。 |
| コメント言語 | **日本語**（既存Rubyコード踏襲）。Ruby版のコメントはそのまま移植する。 |
| 依存方針 | 実行時は標準ライブラリのみ。goyacc は生成専用で、生成された `parser.go` はコミットする。 |
| コミット運用 | `agent/golang` に直接、フェーズごとに1コミット。 |
| CI | **整備しない**（ローカル検証のみ）。`.travis.yml` は ruby 1.9.2 / cc65 リンク切れで死んでいるため Phase 9 で削除。 |

### 移植しないもの（スコープ外）

| 対象 | 理由 |
|---|---|
| `x6502` ターゲット | 外部エミュレータ前提で現環境に未導入。`share/x6502/` `fclib/x6502/` も対象外。Go版は `-t x6502` をエラーとして拒否する。 |
| デバッグHTML出力 (`-d`) | `HtmlOutput` / `share/main.html.erb`。コンパイル結果に影響しない。Go版は `-d` をデバッグログ出力のみに割り当てる。 |
| コードサイズ退行検出 | `size_prev.txt` との1.1倍比較。asm一致で上位互換のため冗長。`size_prev.txt` / `size_cur.txt` は移植対象外。 |
| `.fcm`(分割コンパイルキャッシュ) | `lib/fc/hlc.rb:151` で `if false &&` により無効化済み。`Fixnum` 参照も Ruby 3.2 以降で壊れた死コード。 |
| `bin/emu6502`(node製) | Ruby製 `lib/r6502` に置換済みで未使用。 |

---

## 環境メモ（検証済み 2026-08-28）

| 項目 | 状態 |
|---|---|
| Go | 1.24.5 windows/amd64 (`go` コマンド利用可) |
| goyacc | `$(go env GOPATH)/bin/goyacc.exe` にインストール済み |
| Ruby | 3.3.7 x64-mingw-ucrt |
| gems | racc 1.8.1 / rspec 3.13.1 / simplecov 0.22.0 / rake 13.3.1 すべて導入済み |
| cc65 | `C:\Applications\cc65-snapshot-win32\bin\` に ca65 / ld65 あり（PATH通過済み） |

**注意: `bundle exec` は壊れている**（shim が ruby を見つけられない）。検証は素の `ruby` / `rspec` を直接使うこと。

動作確認済みのオラクル実行コマンド:

```bash
cd test && ruby ../bin/fcc build test_basic.fc
```

```bash
cd test && ruby ../bin/fcc run test_basic.fc
```

```bash
rspec test/fc/test_base.rb test/fc/test_allocator.rb
```

---

## 移植元の構成

| 層 | ファイル | 行数 | 役割 |
|---|---|---|---|
| CLI | `bin/fcc` | 53 | オプション解析、`build`/`compile`/`run` |
| ドライバ | `lib/fc/compiler.rb` | 265 | パイプライン統括、ca65/ld65起動、リンク、実行 |
| 字句 | `lib/fc/parser_ext.rb` | 100 | StringScanner ベースのレキサ |
| 構文 | `parser.y` → `lib/fc/parser.rb` | 180 (生成1702) | racc LALR文法 |
| 中間 | `lib/fc/hlc.rb` | 880 | AST→IR、スコープ/型/const評価、モジュール、マクロ |
| 割付 | `lib/fc/allocator.rb` | 332 | live range、ZP/A/条件レジスタ割付 |
| 生成 | `lib/fc/llc.rb` | 1015 | IR→ca65アセンブラ |
| 基盤 | `lib/fc/base.rb` | 455 | Type/Value/Scope/Module/Lambda |
| エミュ | `lib/r6502/` | 843 | `-e` 実行・テスト用6502エミュレータ |
| テンプレ | `share/` | - | ERB (base.asm / ld65.cfg × nes,emu) |

手書きRuby実質 約5,700行（生成パーサ・スコープ外分を除く）。

## パッケージ配置

```
go.mod                   module github.com/haramako/fc
cmd/fcc/                 CLI (bin/fcc 相当、オプション互換)
internal/fc/
  token.go lexer.go      parser_ext.rb 相当
  parser.y parser.go     goyacc生成 (parser.go はコミットする)
  ast.go                 型付きAST
  types.go               Type(インターン付き)
  value.go               Value / CastedValue / PointeredArray
  scope.go module.go
  hlc.go consteval.go    AST→IR
  macro.go               Go組み込みマクロ登録(stdmacro/stdio/math/unittest)
  ir.go
  allocator.go
  llc.go
  driver.go              ca65/ld65起動、テンプレート、リンク
  embed.go               fclib/ share/ の embed.FS
  golden_test.go         全フェーズの差分テスト
internal/r6502/          memory / cpu / instructions / opcodes
tools/gen_golden.rb      Ruby版から golden を生成
testdata/golden/
  ast/  ir/  allocir/  asm/  bin/  stdout/
```

`fclib/` `share/` `test/` はリポジトリ直下のまま両実装で共用する。移植完了時にRuby版を `ruby/` へ退避。

---

## フェーズ詳細

### Phase 0 — 基盤整備・差分ハーネス  ✅ 完了 2026-08-28

主作業は Ruby 側の正規形ダンパー（AST/IR/割付後IR）の実装。ここの仕様が曖昧だと以降全フェーズの判定が揺れるため、時間をかけてよい。

- [x] `go mod init github.com/haramako/fc` とパッケージディレクトリの雛形作成（`cmd/fcc/main.go` スタブ含む）
- [x] Ruby版を移植開始時点のタグ（`ruby-frozen` = 8358b15）で凍結
- [x] `tools/dumper.rb` 作成: モンキーパッチによる AST / IR / alloc-IR ダンパー（Ruby版本体は無変更）。ASTは `Parser#parse` 直後の生AST + `pos_info` 行番号（`const_eval` の破壊的書き換えの影響を受けない）。IRは `Hlc#compile` 完了直後。alloc-IRは `Llc#alloc_register`（=割付+delete_unuse）直後・optimize_pointer 前
- [x] ダンプ正規形の仕様化: Ruby の `inspect` に依存しない独自S式。文字列はバイト単位エスケープ。`doc/go_port_dump_format.md` に明文化（正典は dumper.rb）
- [x] `tools/gen_golden.rb` 作成: `test/test_*.fc` 全件（`errors.fc` は Phase 8 専用素材、`cycle_use.fc` はモジュールなので除外）について ast / ir / allocir / asm / bin / stdout を `testdata/golden/` に生成
- [x] AST golden は追加で `test/cycle_use.fc` と `fclib/**/*.fc`（`fclib/x6502/` はスコープ外につき除外）も生成
- [x] NES ターゲット経路の golden: `test_basic.fc -t nes` のビルドが通ることを確認し、asm / bin を `test_basic_nes` として保存（実行はしない）
- [x] asm 正規化関数（`^\s*; \d{4}:` の行を除去）を dumper.rb / 仕様書に明文化
- [x] `internal/fc/golden_test.go` の骨格作成（各フェーズ用サブテストを Skip で用意）
- [x] golden 一式を生成してコミット。**2回生成してバイト一致（ld65バイナリ含め決定的）を確認済み**

**合格条件（このコマンドが通ること）**:

```bash
ruby tools/gen_golden.rb && go build ./... && go test ./...
```

golden が再生成しても差分ゼロで、`go test` が（空実装でも）グリーンであること。

---

### Phase 1 — レキサ + パーサ  ✅ 完了 2026-08-28

- [x] レキサ: 記号トークン（`<=` `>=` `==` `+=` `-=` `!=` `->` `<<` `>>` `&&` `||` ほか、`parser_ext.rb:24` の順序を厳守）
- [x] レキサ: 数値リテラル（16進 `0x` / 2進 `0b` / 10進。到達不能な `-?` 分岐、Ruby `to_i` の部分パース・`_` 区切りも1:1再現）
- [x] レキサ: 識別子とキーワード判定（`include` 〜 `private` の23語、`parser_ext.rb:38`）
- [x] レキサ: 文字列3種（`"""..."""` / `"..."` / `'...'`）と `\n` `\xNN` エスケープ（`""` `"""` のみ、`''` は無変換。文字列内改行は行番号にカウントしない、も再現）
- [x] レキサ: コメントスキップ（`//` / `/* */`）と行番号カウント（`//\n` が次行を飲み込む /m の癖も再現）
- [x] `parser.y` → goyacc へ移植（優先順位を逆順に変換して1:1、**競合ゼロ** = racc の `expect 0` と一致。未定義非終端子 `id` の死にルールも再現）
- [x] AST定義: **型付きASTはやめ、Rubyと同じ動的構造（`[]any` / `Sym` / `int` / `string` / `*OMap` / `nil`）を採用**。理由: HLC の `const_eval` 破壊的書き換え・マクロが生ASTを組み立てる挙動・`+=` の部分木共有(エイリアシング)を1:1で再現するため。厳密クローン優先の方針に基づく変更
- [x] `pos_info` の保持機構（Ruby Hash と同じ構造的等値キー・挿入順・後勝ち上書きを `OMap` で再現。goldenの `.pos` も全件一致 = racc/goyacc の先読みタイミングも一致）
- [x] AST正規形ダンプ（`DumpAST` / `DumpPos`）をGo側に実装（CLIフラグ化は Phase 6）
- [x] `parser.go` の生成をコミット、生成手順を `//go:generate goyacc -o parser.go -p fc parser.y` で記録
- [x] ソース読み込みは CRLF→LF 変換（Ruby `File.read` テキストモードの再現。`ReadSource`）

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenAST
```

`test/test_*.fc` および `fclib/**/*.fc`（x6502除外）の全件で AST 差分ゼロ。→ **達成**（25ファイル、`.ast` と `.pos` の両方一致）

---

### Phase 2 — 基盤データ構造  ✅ 完了 2026-08-28

- [x] `Type`（インターンキャッシュ、`to_s`、`BASIC_TYPES`、pointer/array/lambda/fastcall。長さ省略配列の length/size nil は -1 で表現）
- [x] `Value` / `CastedValue` / `PointeredArray`（Delegator の委譲は `UnderlyingValue` / `ValKind` 等の明示ヘルパで再現。`new_int` の「-128 が sint16」境界も再現）
- [x] `Scope`（`use` による横断検索、`@finding` 再帰防止、`find!` / `id_list`）
- [x] `Module` / `Lambda`
- [x] `CompileError`（filename / line_no 付与）… Phase 1 で実装済み
- [x] `test/fc/test_base.rb` を Go test へ移植（TestType 8例 + TestValue/TestScope を追加）

**合格条件**:

```bash
go test ./internal/fc -run "TestType|TestValue|TestScope"
```

→ **達成**
（`test_allocator.rb` の移植は Phase 4 へ。対象の `LiveRangeCalculator` / `Allocator` が Phase 4 で実装されるため、ここでは移植できない）

---

### Phase 3 — HLC (AST→IR)  ✅ 完了 2026-08-28

- [x] `type_eval`
- [x] `const_eval`（AST破壊的書き換え・Ruby整数演算(floor除算/剰余)・`unpack('c*')`の符号付きバイトまで再現）
- [x] `rval` / `lval`（`+=`の部分木共有、`:deref`エラーの未代入`left`参照(空文字列)などの癖も再現）
- [x] `compile_statement`（全19種）
- [x] `compile_module` / `compile_lambda` / `compile_block`（コンパイル中のlambda追加を拾うindexループ）
- [x] スコープ操作（`in_scope` / `attach_scope` / `add_var` / `add_def` / `add_def_module`）
- [x] 一時変数・ラベル採番 — 全golden一致で番号一致を確認
- [x] `cast` / `make_compatible` / `TypeUtil`
- [x] include分岐（`.asm` / `.inc` / `.chr` / `.rb`）
- [x] マクロ機構（`macros.go`: ファイル名キー → Go組み込みマクロ登録）
- [x] マクロ `times`（現行Ruby版では展開結果が不正で未使用の死にマクロ。同じ構造を返す形で1:1移植）
- [x] マクロ `printf` / `cos` / `unittest_run_tests`（`@scope.id_list` 順序も一致）
- [x] `asm` 組み込みマクロ
- [x] IR正規形ダンプ（`DumpIR` / `irdump.go`）をGo側に実装

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenIR
```

全 `.fc` で IR 差分ゼロ。→ **達成**（14件: emu 13 + nes 1、初回実行で全一致）

---

### Phase 4 — レジスタ割付  ✅ 完了 2026-08-28

- [x] `calc_live_range`（use_define は挿入順保持、CastedValue は Delegator 合流を `UnderlyingValue` で再現、`pget` の `op[3]`=nil も再現）
- [x] `LiveRangeCalculator`
- [x] `Allocator`(重なり判定 / 結合)
- [x] `allocate_register`（ZP割付、fastcall_reg、frame size over 検査）
- [x] `allocate_a`
- [x] `allocate_cond`（carry/zero/negative。`:lt`符号付きサイズ2の `next` が location=:cond を残したままにする癖も再現）
- [x] `delete_unuse`
- [x] alloc-IR正規形ダンプ（`DumpAllocLambda`）をGo側に実装
- [x] `test/fc/test_allocator.rb` を Go test へ移植（`TestAllocatorUnit`）

**合格条件**:

```bash
go test ./internal/fc -run "TestGoldenAllocIR|TestAllocatorUnit"
```

全 `.fc` で割付後IR差分ゼロ（`location` / `address` / `cond_reg` / `cond_positive` を含む）。→ **達成**（14件全一致）

---

### Phase 5 — LLC (IR→asm)  ✅ 完了 2026-08-28

- [x] opディスパッチ骨格（`Compile` / `CompileLambda`、行のflatten/nil削除/インデント規則も1:1）
- [x] `label` / `jump` / `if` / `return`
- [x] `load` / `load_a` / `store_a` / `to_asm` / `byte` / `mangle`
- [x] `push_arg` / `push_result` / `call` / `call_subroutine`
- [x] `fastcall` / `push_fastcall_arg` / `push_fastcall_result`
- [x] 算術・論理（`add` `sub` `and` `or` `xor` `shift_left` `shift_right` `uminus` `not`）
- [x] `mul_div_mod`（2の累乗最適化・符号付きdiv含む）
- [x] 比較（`eq` / `lt`）と `sign_extension`。**`:lt` の `signed = a or b` が Ruby の優先順位により op[2] しか見ないバグも忠実に再現**（唯一の初回差分だった）
- [x] ポインタ・配列（`ref` / `pget` / `pset` / `index` / `index_pget` / `index_pset`）
- [x] `asm` インライン埋め込み
- [x] `emit_block` と defs出力（`equ` / `bss` / `block` / `code`）
- [x] `optimize_pointer`（`-O1` 以上）
- [x] `extend_jump`（サイズ表・正規表現を1:1移植）
- [x] `.inc` ファイルの生成

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenAsm
```

全 `.fc` で、IRコメント行（`^\s*; \d{4}:`）を除去した `.s` と `.inc` が golden と完全一致。→ **達成**（14件全一致）

---

### Phase 6 — ドライバ・テンプレート  ✅ 完了 2026-08-28

- [x] `base.asm.erb` 相当（nes / emu）— テンプレートが小さく静的なため text/template ではなく直接文字列生成で1:1再現（アセンブラは空白・コメント非依存なので binary golden で検証される）
- [x] `ld65.cfg.erb` 相当（nes / emu）
- [x] バンク／セグメント計算（`bank_count` / `char_banks` / `mapper` / `org`）
- [x] `find_share` / `find_module` のパス解決
- [x] ca65 / ld65 の `os/exec` 起動とエラーハンドリング（`CommandError` 相当）
- [x] `make_runtime` / `make_base` / `link`
- [x] `fclib` + `share` を `embed.FS` で同梱（ルート `embedfs.go`。FC_HOME が見つからない場合は一時ディレクトリへ展開 — ca65/ld65 が実ファイルを要求するため）
- [x] CLI (`cmd/fcc`) のオプション互換（`-o` `-e` `-S` `-d` `-t` `-O`、サブコマンド `build`/`b`/`compile`/`c`/`run`。FC_HOME は env → exe位置 → cwd上方探索 → embed展開 の順で解決）
- [x] `-t x6502` を明示的なエラーにする（スコープ外）

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenBinary
```

→ **達成**（14件バイト一致: a.bin ×13 + a.nes ×1。CLI の build/compile/x6502拒否も動作確認済み）

---

### Phase 7 — 6502エミュレータ  ⬜ 未着手  (2〜3日)

- [ ] `memory.go`
- [ ] `opcode_table.go` / `instr_table.go`
- [ ] `cpu_instructions.go`（全命令）
- [ ] `cpu_execution.go`（`step_silent` 相当）
- [ ] emuターゲットのホスト呼び出し規約（`$fff0`〜`$ffff`: print / print_int / print_int_sp / exit）

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenStdout
```

全 `test_*.fc` の実行 stdout と終了コードが golden と一致。

---

### Phase 8 — テスト移行  ⬜ 未着手  (1〜2日)

- [ ] `test/test-all` 相当を Go test として再実装
- [ ] 全 `test_*.fc` のコンパイル＋実行＋終了コード検証
- [ ] `test/errors.fc` の期待エラー正規表現テスト（`//@` 区切りのパース、共通ソース `interrupt`/`main` の付加を含む）
- [ ] **エラーメッセージ文言がRuby版と完全一致することを確認**（`errors.fc` が正規表現で照合するため）
- [ ] 注意: 現行 `test-all` は errors.fc の各断片で**例外が出なかった場合を失敗にしていない**（`rescue` のみで else 無し、`test/test-all:90`）。まず同挙動で移植してグリーンを確認し、その後「コンパイルが通ってしまったら失敗」に厳格化する。厳格化で落ちる断片が見つかったら、Ruby版の実挙動を作業ログに記録して仕様として扱う
- [ ] 注意: `test-all` は先頭1ファイルのみ `debug_info: true` でビルドする（HtmlOutput 経路）。`-d` はスコープ外のため Go 版では再現不要
- [ ] 全 golden テストを1コマンドで回せるようにする

**合格条件**:

```bash
go build ./... && go test ./...
```

すべてグリーン。

---

### Phase 9 — 切替  ⬜ 未着手  (1日)

- [ ] README / `doc/` 更新（Go版の使い方、ビルド手順）
- [ ] `GOOS`/`GOARCH` マトリクスでのリリースビルド確認
- [ ] Ruby版を `ruby/` に退避（`lib/` `bin/fcc` `Gemfile` `Rakefile` `fc.gemspec` `parser.y`）
- [ ] `.travis.yml` を削除（死んでいるため）
- [ ] `.gitignore` の整理
- [ ] `master` へマージ

**合格条件**:

```bash
go build ./... && go test ./... && go vet ./...
```

Go版単体で全テストが通り、配布可能なバイナリが生成できること。

---

## 見積り

集中作業で **3〜4週間**。AI支援併用で実質 1.5〜2週間。

## 検証戦略の要点

Ruby版が唯一の仕様書であるため、各フェーズの境界に**機械的な差分チェックポイント**を置くことが成否を分ける。

| Phase | golden | テスト名 |
|---|---|---|
| 1 | `testdata/golden/ast/` | `TestGoldenAST` |
| 3 | `testdata/golden/ir/` | `TestGoldenIR` |
| 4 | `testdata/golden/allocir/` | `TestGoldenAllocIR` |
| 5 | `testdata/golden/asm/` | `TestGoldenAsm` |
| 6 | `testdata/golden/bin/` | `TestGoldenBinary` |
| 7 | `testdata/golden/stdout/` | `TestGoldenStdout` |

## 既知の落とし穴

移植中に踏みやすい箇所。着手前に必ず確認すること。

- **採番の一致**: `tmp_count` / `new_label` の連番が Ruby版とずれると IR 以降が全滅する。Phase 3 で最優先に合わせる。
- **Hash / Symbol の順序**: Ruby の Hash は挿入順を保つ。`Scope#declares` `Module#modules` `@modules` の走査順が出力順序に効くため、Go では `map` ではなく順序を保つ構造を使うこと。
- **`Scope#find` の再帰防止**: `@finding` フラグによる再入ガードがあり、これが無いと `use` の相互参照で無限再帰する。
- **`const_eval` の破壊的書き換え**: `ast[1] = const_eval(ast[1])` のように AST を直接書き換える。同じ AST ノードが2回評価される経路があるため、単純な純粋関数化は挙動を変える。
- **`CastedValue` は `Delegator`**: メソッド呼び出しが `@from` に委譲される。Go では明示的な委譲メソッドが必要。
- **`unittest_run_tests` の順序依存**: `@scope.id_list` の列挙順でテスト実行順が決まる。Scope の順序保持が効いてくる。
- **エラーメッセージ**: `test/errors.fc` が正規表現で文言を照合する。Phase 8 まで文言を変えないこと。

## 作業ログ

セッションをまたいだ際の引き継ぎメモをここに追記する。

- 2026-08-28: **Phase 6 完了**。driver.go (Build パイプライン/ca65/ld65/テンプレート/リンク)、embedfs.go (fclib+share の embed)、cmd/fcc (bin/fcc 互換CLI)。TestGoldenBinary 14件バイト一致。
- 2026-08-28: **Phase 5 完了**。llc.go (全op/emit_block/optimize_pointer/extend_jump)。初回差分は1箇所のみ: `:lt` の `signed = op[2].type.signed or op[3].type.signed` が Ruby の `or` 優先順位で op[2] しか効かないバグ — 再現して 14件全一致。
- 2026-08-28: **Phase 4 完了**。allocator.go (calc_live_range/allocate_register/allocate_a/allocate_cond/Allocator/delete_unuse)。初回の失敗は pget の op[3] 範囲外のみ、修正後 TestGoldenAllocIR 14件全一致 + TestAllocatorUnit グリーン。
- 2026-08-28: **Phase 3 完了**。hlc.go/macros.go/irdump.go を実装、TestGoldenIR 14件が初回実行で全一致(採番・マクロ展開・const_eval破壊的書き換え・スコープ順序すべて一致)。Phase 1 でASTを厳密に合わせたことが効いた。
- 2026-08-28: **Phase 2 完了**。Type/Value/Scope/Module/Lambda/TypeUtil を移植、test_base.rb 移植 + TestValue/TestScope 追加、全グリーン。教訓: PowerShell 5.1 の `Get-Content`/`Set-Content` で UTF-8 ファイルを処理すると文字化けする(計画書を一度破壊して前コミットから復元した)。**ファイル編集は必ず Edit ツールを使うこと**。
- 2026-08-28: **Phase 1 完了**。レキサ(parser_ext.rbの癖含め1:1)・goyaccパーサ(競合ゼロ)・動的AST(`[]any`/`Sym`/`OMap`、型付きASTから方針変更)・S式ダンパー実装。AST golden 25ファイル全一致(.pos含む)。唯一の初回差分はソースのCRLF起因で、Ruby `File.read` テキストモード相当の CRLF→LF 変換(`ReadSource`)を入れて解決。
- 2026-08-28: **Phase 0 完了**。`ruby-frozen` タグ付与。ダンプは bin/fcc 改造ではなく `tools/dumper.rb` のモンキーパッチ方式に変更（Ruby版は完全無変更で凍結、計画の該当箇所を更新）。golden 全生成（ast 25ファイル / ir・allocir・asm・bin・stdout ×13テスト / NESビルド1本）、2回生成でバイト一致を確認。`go build ./... && go test ./...` グリーン。`.gitignore` の `doc/` 除外を解除し `*.nes` に golden 用の負パターンを追加。オラクル(test-all)グリーンも確認済み。
- 2026-08-28: 計画レビュー反映。gen_golden の対象を `test_*.fc` に修正（`errors.fc` / `cycle_use.fc` 除外）、fclib AST golden と NES ビルド golden を追加、`--dump-ast` の出力タイミングを「parse 直後・コンパイル前」に確定、ダンプ正規形は inspect 非依存と明文化、`test_allocator.rb` 移植を Phase 2→4 に移動、キーワード数を 23 に訂正、レキサ数値 `-?` の到達不能を注記、errors.fc の「例外なし=素通り」挙動を Phase 8 に注記。Phase 0 見積りを 1日に変更。
- 2026-08-28: 計画策定。方針決定（Go組み込みマクロ / 厳密クローン / asmはIRコメント行除外で比較 / リポジトリ直下 / 日本語コメント / 完全自律 / CIなし）。スコープ外を確定（x6502, デバッグHTML, サイズ退行検出, .fcm, emu6502）。環境確認済み（Go 1.24.5, goyacc, Ruby 3.3.7, cc65, bundle exec は破損）。未着手。
