# fc Go 移植計画

対象ブランチ: `agent/golang`
Ruby版(現行)を「正解オラクル」として、**生成アセンブラの一致**を全フェーズの合格条件とする厳密クローン移植。

> **このドキュメントは進行管理を兼ねた生きた計画書です。**
> セッションをまたぐ場合、まずこのファイルを読み「進捗サマリ」と「次にやること」から再開する。
> フェーズ完了ごとにチェックボックス・進捗サマリ・作業ログを更新してコミットすること。

---

## 進捗サマリ

最終更新: 2026-08-28 / 現在のフェーズ: **Phase 0 (未着手)**

| Phase | 内容 | 状態 | 完了日 |
|---|---|---|---|
| 0 | 基盤整備・差分ハーネス | ⬜ 未着手 | - |
| 1 | レキサ + パーサ | ⬜ 未着手 | - |
| 2 | 基盤データ構造 | ⬜ 未着手 | - |
| 3 | HLC (AST→IR) | ⬜ 未着手 | - |
| 4 | レジスタ割付 | ⬜ 未着手 | - |
| 5 | LLC (IR→asm) | ⬜ 未着手 | - |
| 6 | ドライバ・テンプレート | ⬜ 未着手 | - |
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
4. **Ruby版は凍結**。唯一の例外は Phase 0 の `--dump-ast` / `--dump-ir` 追加のみ。それ以外で `lib/` `bin/fcc` `fclib/` `share/` を変更してはならない（変更するとオラクルが壊れる）。
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
| オラクルのダンプ | **`bin/fcc` に `--dump-ast` / `--dump-ir` を追加**（Ruby凍結の唯一の例外。`lib/fc/hlc.rb` へのフック追加を含む）。 |
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

### Phase 0 — 基盤整備・差分ハーネス  ⬜ 未着手  (1日)

主作業は Ruby 側の正規形ダンパー（AST/IR/割付後IR）の実装。ここの仕様が曖昧だと以降全フェーズの判定が揺れるため、時間をかけてよい。

- [ ] `go mod init github.com/haramako/fc` とパッケージディレクトリの雛形作成
- [ ] Ruby版を移植開始時点のタグ（`ruby-frozen`）で凍結
- [ ] `bin/fcc` に `--dump-ast` を追加（`lib/fc/hlc.rb:180` の `Parser.new(src,path).parse` **直後、コンパイル開始前**の生ASTを出力する。`const_eval` が AST を破壊的に書き換えるため、コンパイル後のダンプは Phase 1 の合格判定に使えない。`pos_info` の行番号も併せて出力し、Phase 1 の時点で行番号バグを捕捉できるようにする）
- [ ] `bin/fcc` に `--dump-ir` を追加（各 `Lambda#ops` を正規形で出力）
- [ ] `bin/fcc` に `--dump-alloc-ir` を追加（レジスタ割付後の `ops` を出力）
- [ ] ダンプ正規形の仕様化: AST/IR/割付後IR のダンプは Ruby の `inspect` に**依存しない**独自の正規形（S式 or JSON）を Ruby 側ダンパーとして定義し、Go はそれに合わせる。文字列は一意にエスケープしたバイト列として出力する（`\xNN` 由来のバイナリを含むため）
- [ ] `tools/gen_golden.rb` 作成: `test/test_*.fc` 全件（= `test-all` と同じ glob。`errors.fc` はコンパイル失敗が正の Phase 8 専用素材、`cycle_use.fc` は `test_cycle.fc` から use されるモジュールなので、どちらも単体生成しない）について ast / ir / allocir / asm / bin / stdout を `testdata/golden/` に生成
- [ ] gen_golden.rb: AST golden は追加で `fclib/**/*.fc`（`fclib/x6502/` はスコープ外につき除外）も生成する（Phase 1 の合格条件が参照）
- [ ] gen_golden.rb: NES ターゲット経路のビルド golden を最低1本生成する（候補: `test_basic.fc` を `-t nes` でビルドし asm / bin を `*_nes` として保存。ビルドが通らなければ最小の NES 用ソースを `test/` に新設）。`test-all` は emu のみのため、これがないと `share/nes` テンプレと `fclib/nes/` が Phase 9 まで未検証になる。stdout golden は不要（r6502 で NES は実行できない）
- [ ] asm 正規化関数を決定（`^\s*; \d{4}:` の行を除去）し、Ruby側・Go側で共有する仕様として明文化
- [ ] `internal/fc/golden_test.go` の骨格作成（各フェーズ用のサブテストを空実装で用意）
- [ ] golden 一式を生成してコミット

**合格条件（このコマンドが通ること）**:

```bash
ruby tools/gen_golden.rb && go build ./... && go test ./...
```

golden が再生成しても差分ゼロで、`go test` が（空実装でも）グリーンであること。

---

### Phase 1 — レキサ + パーサ  ⬜ 未着手  (2〜3日)

- [ ] レキサ: 記号トークン（`<=` `>=` `==` `+=` `-=` `!=` `->` `<<` `>>` `&&` `||` ほか、`parser_ext.rb:24` の順序を厳守）
- [ ] レキサ: 数値リテラル（16進 `0x` / 2進 `0b` / 10進。regex には先頭 `-?` が付くが、記号 regex が先に走るため `-` は常に記号として消費され、この分岐の `-` は実際には到達しない。**到達しない分岐ごと1:1で再現し、Go 側で「直して」挙動を変えないこと**）
- [ ] レキサ: 識別子とキーワード判定（`include` 〜 `private` の23語、`parser_ext.rb:38`）
- [ ] レキサ: 文字列3種（`"""..."""` / `"..."` / `'...'`）と `\n` `\xNN` エスケープ
- [ ] レキサ: コメントスキップ（`//` / `/* */`）と行番号カウント
- [ ] `parser.y` → goyacc へ移植（優先順位宣言 `prechigh`〜`preclow` を1:1で）
- [ ] 型付きAST定義（`ast.go`）
- [ ] `pos_info`(ファイル名・行番号) の保持機構
- [ ] `--dump-ast`(S式正規形) をGo側に実装
- [ ] `parser.go` の生成をコミット、生成手順を `//go:generate` で記録

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenAST
```

`test/test_*.fc` および `fclib/**/*.fc`（x6502除外）の全件で AST 差分ゼロ。

---

### Phase 2 — 基盤データ構造  ⬜ 未着手  (1〜2日)

- [ ] `Type`（インターンキャッシュ、`to_s`、`BASIC_TYPES`、pointer/array/lambda/fastcall）
- [ ] `Value` / `CastedValue` / `PointeredArray`
- [ ] `Scope`（`use` による横断検索、再帰防止フラグ相当、`find!` / `id_list`）
- [ ] `Module` / `Lambda`
- [ ] `CompileError`（filename / line_no 付与）
- [ ] `test/fc/test_base.rb` を Go test へ移植

**合格条件**:

```bash
go test ./internal/fc -run "TestType|TestValue|TestScope"
```

移植した examples 相当がすべて通ること。
（`test_allocator.rb` の移植は Phase 4 へ。対象の `LiveRangeCalculator` / `Allocator` が Phase 4 で実装されるため、ここでは移植できない）

---

### Phase 3 — HLC (AST→IR)  ⬜ 未着手  (4〜6日 / 最難関)

- [ ] `type_eval`
- [ ] `const_eval`（**AST破壊的書き換えの挙動を明示的に再現すること**）
- [ ] `rval` / `lval`
- [ ] `compile_statement`（var/const/if/loop/while/for/break/continue/return/switch/exp/function/options/use/include/public/private/block/blank）
- [ ] `compile_module` / `compile_lambda` / `compile_block`
- [ ] スコープ操作（`in_scope` / `attach_scope` / `add_var` / `add_def` / `add_def_module`）
- [ ] 一時変数の採番（`tmp_count`）とラベル採番（`new_label` / `new_labels`）— **Ruby版と同じ番号になること**
- [ ] `cast` / `make_compatible` / `TypeUtil`
- [ ] include分岐（`.asm` / `.inc` / `.chr` / `.rb`）
- [ ] マクロ機構: ファイル名キーでGo組み込みマクロ群に解決する仕組み
- [ ] マクロ `times`（`fclib/stdmacro.rb` 相当）
- [ ] マクロ `printf`（`fclib/stdio.rb` 相当）
- [ ] マクロ `cos`（`fclib/math.rb` 相当）
- [ ] マクロ `unittest_run_tests`（`fclib/unittest.rb` 相当。`@scope.id_list` の順序に依存するため要注意）
- [ ] `asm` 組み込みマクロ（`hlc.rb:30` の `defmacro :asm`）
- [ ] `--dump-ir` をGo側に実装

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenIR
```

全 `.fc` で IR 差分ゼロ。

---

### Phase 4 — レジスタ割付  ⬜ 未着手  (2日)

- [ ] `calc_live_range`
- [ ] `LiveRangeCalculator`
- [ ] `Allocator`（重なり判定 / 結合）
- [ ] `allocate_register`（ZP割付）
- [ ] `allocate_a`（Aレジスタ）
- [ ] `allocate_cond`（条件レジスタ carry/zero/negative）
- [ ] `delete_unuse`
- [ ] `--dump-alloc-ir` をGo側に実装
- [ ] `test/fc/test_allocator.rb` を Go test へ移植（`TestAllocatorUnit`。Phase 2 から移動）

**合格条件**:

```bash
go test ./internal/fc -run "TestGoldenAllocIR|TestAllocatorUnit"
```

全 `.fc` で割付後IR差分ゼロ（`location` / `address` / `cond_reg` / `cond_positive` を含む）。

---

### Phase 5 — LLC (IR→asm)  ⬜ 未着手  (3〜4日)

- [ ] opディスパッチ骨格（`compile` / `compile_lambda`）
- [ ] `label` / `jump` / `if` / `return`
- [ ] `load` / `load_a` / `store_a` / `to_asm` / `byte` / `mangle`
- [ ] `push_arg` / `push_result` / `call` / `call_subroutine`
- [ ] `fastcall` / `push_fastcall_arg` / `push_fastcall_result`
- [ ] 算術・論理（`add` `sub` `and` `or` `xor` `shift_left` `shift_right` `uminus` `not`）
- [ ] `mul_div_mod`（`mul` / `div` / `mod`）
- [ ] 比較（`eq` / `lt`）と `sign_extension`
- [ ] ポインタ・配列（`ref` / `pget` / `pset` / `index` / `index_pget` / `index_pset`）
- [ ] `asm` インライン埋め込み
- [ ] `emit_block`（`code` / `bss` / `equ` / `block` / `frame` / `global` / `local` / `literal` / `reg` / `fastcall_reg`）
- [ ] `optimize_pointer`（`-O1` 以上）
- [ ] `extend_jump`（分岐距離 ±128 超えの変換）
- [ ] `.inc` ファイルの生成

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenAsm
```

全 `.fc` で、IRコメント行（`^\s*; \d{4}:`）を除去した `.s` と `.inc` が golden と完全一致。

---

### Phase 6 — ドライバ・テンプレート  ⬜ 未着手  (1〜2日)

- [ ] `base.asm.erb` → `text/template`（nes / emu）
- [ ] `ld65.cfg.erb` → `text/template`（nes / emu）
- [ ] バンク／セグメント計算（`bank_count` / `char_banks` / `mapper` / `org`）
- [ ] `find_share` / `find_module` のパス解決
- [ ] ca65 / ld65 の `os/exec` 起動とエラーハンドリング（`CommandError` 相当）
- [ ] `make_runtime` / `make_base` / `link`
- [ ] `fclib` + `share` を `embed.FS` で単一バイナリに同梱
- [ ] CLI (`cmd/fcc`) のオプション互換（`-o` `-e` `-S` `-d` `-t` `-O`、サブコマンド `build`/`b`/`compile`/`c`/`run`）
- [ ] `-t x6502` を明示的なエラーにする（スコープ外）

**合格条件**:

```bash
go test ./internal/fc -run TestGoldenBinary
```

全 `test_*.fc` で `a.bin` が golden とバイト一致。加えて Phase 0 で生成した NES ビルド golden（`*_nes`）の `a.nes` もバイト一致（emu テストだけでは `share/nes` テンプレ・`bank_count` / `char_banks` / `mapper` の経路を通らない）。

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

- 2026-08-28: 計画レビュー反映。gen_golden の対象を `test_*.fc` に修正（`errors.fc` / `cycle_use.fc` 除外）、fclib AST golden と NES ビルド golden を追加、`--dump-ast` の出力タイミングを「parse 直後・コンパイル前」に確定、ダンプ正規形は inspect 非依存と明文化、`test_allocator.rb` 移植を Phase 2→4 に移動、キーワード数を 23 に訂正、レキサ数値 `-?` の到達不能を注記、errors.fc の「例外なし=素通り」挙動を Phase 8 に注記。Phase 0 見積りを 1日に変更。
- 2026-08-28: 計画策定。方針決定（Go組み込みマクロ / 厳密クローン / asmはIRコメント行除外で比較 / リポジトリ直下 / 日本語コメント / 完全自律 / CIなし）。スコープ外を確定（x6502, デバッグHTML, サイズ退行検出, .fcm, emu6502）。環境確認済み（Go 1.24.5, goyacc, Ruby 3.3.7, cc65, bundle exec は破損）。未着手。
