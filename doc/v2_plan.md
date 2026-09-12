# fc v2 作業計画 — Go らしい実装への転換（R0〜R3）

対象ブランチ: **`feature/v2`**（`agent/golang` 45c2d78 から 2026-09-12 に分岐）
実装担当: AI エージェント（Opus）。本書はその**作業指示書**であり、人間のレビュー資料でもある。
背景と全体像は [go_evolution_plan.md](go_evolution_plan.md)、環境・ハマりどころは
[development_notes.md](development_notes.md) を参照。

> **生きた計画書。** セッション開始時はまず「進捗」と「次にやること」を読んで再開する。
> ステップ完了ごとにチェックボックス・進捗表・作業ログ（§9）を更新してコミットする。

---

## 進捗

最終更新: 2026-09-12 / 状態: **計画策定完了・未着手**

| Phase | 内容 | 目安 | 状態 |
|---|---|---|---|
| R0 | 検証基盤の切り替え（`-update`、ast golden 廃止、ベンチ） | 0.5日 | ⬜ |
| R1 | 型付きフロントエンド・型付き IR（a〜g の 7 ステップ） | 5〜7日 | ⬜ |
| R2 | エラー処理の近代化（位置情報・error 値化） | 1日 | ⬜ |
| R3 | パッケージ構成・API・決定性・テスト並列化 | 2〜3日 | ⬜ |

状態記号: ⬜ 未着手 / 🔄 進行中 / ✅ 完了 / ⏸️ 保留

**次にやること**: R0-1 から順に。

---

## 0. エージェント向け運用ルール

### 0.1 不変条件（ガードレール）

これらは本計画（R0〜R3）の全期間で守る。破ったコミットは作らない。

| # | 不変条件 | 補足 |
|---|---|---|
| G1 | **挙動 golden は常に緑**: `TestGoldenStdout`（stdout + 終了コード）、`TestErrorsFC`、`TestExampleMiku/Castle`（ROM バイト一致）、`internal/nes` の全テスト | 意図的な挙動変更は R0〜R3 では**禁止**（R4 以降の仕事）。唯一の例外は R1-a のコメント字句 2 件（§4.2 参照、コーパス上の出力無変化を確認済み） |
| G2 | **asm/bin golden は差分ゼロ** | 差分が出たら移行ミスとみなし原因を特定して戻す。`-update` で上書きして通すのは禁止。例外は (a) R0-3 の初回再生成、(b) R3-c のラベル正規化導入時、の 2 回のみ |
| G3 | ir/allocir golden は **R1-d（型付き IR 導入）のコミットで形式を変えて再生成**する。それまでは差分ゼロを維持 | R1-c（sema 書き換え）の合格判定に ir golden を使うため、先に消さない |
| G4 | ast golden（`testdata/golden/ast/`、`.pos` 含む）は **R0-2 で削除** | Ruby の S 式構造に紐付いており、型付き AST 後は意味を持たない |
| G5 | **Ruby 資産に触らない**: `ruby/`, `tools/dumper.rb`, `tools/gen_golden.rb`, `test/test-all`, タグ `ruby-frozen` / `go-strict-clone` | R0-4 で「役目終了」の注記を**先頭コメントに追加する**のみ可 |
| G6 | **CLI の外部挙動を維持**: `fcc build/compile/run` のサブコマンドとフラグ、終了コード、出力先 `.fc-build/_<mod>.s` `.inc` `.o` `base.o` `runtime*.o` `ld65.cfg`、stdout の内容 | castle 実プロジェクトの Rakefile が `fcc compile -t nes main.fc` → `ca65 data.asm` → 独自 `ld65.cfg` の手順で `.fc-build/` を直接参照する |
| G7 | **新規コードは Go らしく書く**: `any` + 型アサーションのデータ構造を新設しない、`Sym`/`OMap`/`Canon` を新規コードで使わない、エラーは `error` 値、パッケージレベルの可変状態を増やさない、公開識別子に doc comment | 移行期間中の「新旧の境界」にだけ `any` を許す（R1-b の lower.go 等）。境界は削除予定であることをコメントに明記 |
| G8 | **テストの並列安全**: リポジトリ内の `examples/*/.fc-build` を複数テストで共有しない（一時ディレクトリに複製してビルド） | `go test ./...` はパッケージ並列。development_notes.md 参照 |

### 0.2 作業の進め方

- **1 ステップ = 1 コミット**が原則（大きいステップは意味のある単位で分割可。ただし各コミットで全テスト緑）。
- コミット前に必ず: `go build ./... && go vet ./... && go test ./...`（全部緑）。
- コミットメッセージ: `v2(R1-b): 型付きASTとgoyaccパーサを追加、lowerで旧ASTへ変換` のように **`v2(<ステップ>):` プレフィックス**。本文に合格条件をどう確認したかを 1〜3 行。
- **差分が出たら**: 原因を突き止めて修正する。理解できない差分を `-update` で消さない。どうしても解けなければ §7 未決事項に書いて、そのステップを ⏸️ にして次へ進まず報告する。
- **迷ったら出力を変えない側に倒す**。設計判断が必要で本書に答えがないものは §7 に追記し、**最も保守的な選択**（現行挙動維持）で先へ進む。ブロックしない。
- 各ステップの終わりに本書の進捗表・チェックボックス・§9 作業ログを更新（同じコミットに含める）。
- ドキュメント/コードは UTF-8 (BOM なし)・LF。**PowerShell の `Get-Content`/`Set-Content` でファイルを書かない**（development_notes.md の罠）。編集は Edit/Write ツールで。
- `parser.go` は生成物: `go generate ./internal/...`（`goyacc` は `~/go/bin` に導入済み）。`parser.y` を変えたら必ず再生成してコミットに含める。

### 0.3 検証コマンド

| 目的 | コマンド | 所要 |
|---|---|---|
| 全部 | `go test ./...` | 約 75 秒（fc 20s + nes 16s + ビルド） |
| コンパイラ golden だけ | `go test ./internal/fc -run 'TestGolden'` | 数秒 |
| 実プロジェクト ROM | `go test ./internal/fc -run 'TestExample'` | 約 5 秒 |
| NES スモーク/自動プレイ | `go test ./internal/nes` | 約 16 秒（Mesen が無ければ当該テストは Skip） |
| golden 再生成（R0 以降） | `go test ./internal/fc -run 'TestGolden' -update` | |
| 静的検査 | `go vet ./...`（現状クリーン。維持する） | |
| ベンチ（R0-5 以降） | `go test ./internal/fc -run xxx -bench BenchmarkCastle -benchmem` | |

### 0.4 スコープ外（やらないこと）

- Ruby 由来の癖・バグの修正（§2.5 の表。R4 の仕事）。「ついでに直す」をしない。
- CI / リリース（R5）、言語機能追加（F2）、最適化（F3）。
- フォーマッタ本体・文法 v2・モジュール単位コンパイル本体（F-fmt / F-mod）。
  ただし**それらを可能にする設計制約（§1.3）は本計画で満たす**。
- 複数エラー同時報告（挙動変更を伴うため R2 から外した。§8 参照）。

---

## 1. ゴール・非ゴール・設計制約

### 1.1 ゴール

Ruby 厳密クローンとして書かれた現行実装の **Ruby 模倣構造**を Go の型付き構造に置き換える。
**生成アセンブラ・ROM・実行結果は一切変えない。**

置き換える対象（現状 → 目標）:

| 現状 | 目標 |
|---|---|
| AST = `[]any`（先頭 `Sym` がノード種別） | 型付き AST ノード（位置情報・コメント保持） |
| `pos_info` = AST ノードの**構造的等値**をキーにした `*OMap` | 各ノードが `Pos`/`End` を持つ（同一内容の文が同じ行に潰れるバグが構造的に消える） |
| IR = `[][]any`（`op[0]` が `Sym` の opcode） | `ir.Op` 構造体（opcode enum + 型付きオペランド） |
| `Sym`（文字列型の擬似シンボル）、`*OMap`（挿入順 Hash）、`Canon()`（構造キー） | enum / struct フィールド / 順序が必要な箇所だけ明示的な ordered 構造。`Canon` 全廃 |
| `panic(*CompileError)` + 各所で recover、位置は行番号のみ | 回復点は 1 箇所、境界は `error`、位置は file:line:col |
| グローバル `typeCache`、`Hlc` が全モジュールを抱える、カウンタはコンパイラ全体で共有 | インスタンス持ち、モジュール単位の sema、モジュール内で閉じた採番 |
| 単一パッケージ `internal/fc`（9,000 行） | `syntax` / `types` / `ir` / `sema` / `regalloc` / `codegen` / `driver` |

### 1.2 非ゴール

§0.4 のとおり。特に**挙動変更ゼロ**を強調する: 本計画の成功判定は「リファクタ前後で asm が同一」であり、
asm が「良くなった」ことは失敗である。

### 1.3 将来要件から逆算した設計制約（R1/R3 で必ず満たす）

後続計画（文法 v2 + フォーマッタ + マイグレーション、モジュール単位コンパイル）のために、
今のうちに構造へ織り込む制約。後付けが最も高くつくものを選んである。

| # | 制約 | 効く場所 | 理由 |
|---|---|---|---|
| C1 | **ロスレス構文木**: 全ノードが `Pos`/`End` を持つ。コメントは位置付きで保持される。sema は AST を**変異させない** | R1-a, R1-b, R1-c | フォーマッタ（`fcc fmt`）はコメントと元の構造を必要とする。`const_eval` の破壊的書き換えは廃止 |
| C2 | **文法バージョン中立な AST**: ノードは「意味」で命名（`VarDecl`、`kVAR` ではない）。表層文法を知るのはパーサとプリンタだけ | R1-b | 文法 v2 のパーサが**同じ AST** を生成できれば、マイグレーション = 「v1 で読んで v2 で印字」になり別機能が要らない |
| C3 | **frontend と sema の分離**: `syntax` パッケージは `sema`/`ir`/`types` を import しない。マクロ展開は sema 側 | R1-b, R3-a | フォーマッタは sema なしで動く必要がある |
| C4 | **モジュール単位の sema**: sema の入力は「1 モジュールの AST + 依存モジュールの `ir.ModuleInterface`」。importer が依存モジュールの内部（Lambda 本体・private）に触れない | R1-c（構造の準備）, R3-b | 分割コンパイルの前提。全モジュールを 1 プロセスで処理するのは当面そのままでよい |
| C5 | **決定性はモジュール内で閉じる**: tmp/label の採番はモジュール（さらに lambda）単位。コンパイラ全体のカウンタ禁止 | R3-c | モジュールを単独で再コンパイルしても同じ出力になる。**現行はコンパイラ全体で連番**（§2.4）なので切替時にラベル名が変わる → R3-c の正規化手順で吸収 |
| C6 | **エラーは値**: 内部で panic を使ってよいが回復点は 1 箇所に明文化。境界は `error`。全エラーが位置（file:line:col）を持つ | R2 | エディタ連携・複数エラー報告（将来）の前提 |
| C7 | **ライブラリとして使える**: 作業ディレクトリ非依存（`os.Chdir`/`t.Chdir` に頼らない）、stdout/stderr は `io.Writer` 注入 | R3-d, R3-e | `fcc check`/watch、テスト並列化、エディタ連携 |

---

## 2. 現状の構造（着手前に読むこと）

### 2.1 ファイル地図

| ファイル | 行 | 役割 | 本計画での行き先 |
|---|---|---|---|
| `internal/fc/lexer.go` | 289 | レキサ（Ruby の正規表現レキサの厳密移植。コメントは**捨てる**） | R1-a で `internal/syntax/lexer.go` に新実装、旧は R1-c で削除 |
| `internal/fc/parser.y` → `parser.go` | 202 / 1361 | goyacc 文法。アクションが `[]any{Sym("if"), ...}` を組む | R1-b で `internal/syntax/parser.y` に書き直し（型付きノード生成） |
| `internal/fc/parser_driver.go` | 66 | goyacc と Lexer の接続、`pos_info` 記録、`ParseSrc` | R1-b で置換 |
| `internal/fc/sexp.go` | 161 | `Sym`、`OMap`、`Canon`、S 式ダンプ（`DumpAST`/`DumpPos`/`SexpStr`） | `DumpAST/DumpPos` は R0-2 で削除。`Sym/OMap/Canon` は R1-e で全廃 |
| `internal/fc/base.go` | 118 | `OMap` 実装、`CompileError`、`ToS`（Ruby `to_s`）、`cons` | R1-e / R2 で解体 |
| `internal/fc/types.go`, `type_util.go` | 154 / 49 | `Type`、`TypeOf(ast any)`、グローバル `typeCache`、互換型計算 | R1-e で `internal/types` に。インターンはインスタンス持ち |
| `internal/fc/value.go` | 231 | `Value`（変数・リテラル・レジスタ割付結果）、`CastedValue`、`PointeredArray` | R1-d で `internal/ir` に |
| `internal/fc/module.go` | 69 | `Module`、`Lambda`、`Def` | R1-d で `internal/ir` に |
| `internal/fc/scope.go` | 78 | `Scope`（`Declares *OMap`、`uses`、循環 use 防止フラグ） | R1-e で typed に、R3-a で `sema` へ |
| `internal/fc/hlc.go` | 1152 | **HLC**: AST → IR。`compileStatement`/`constEval`/`lval`/`rval`/`typeEval`/`cast`。モジュール読み込み（`use`/`include`）もここ | R1-c で AST 入力を typed に、R1-d で IR 出力を typed に、R3-a で `sema` へ |
| `internal/fc/macros.go`, `textconv.go` | 139 / 108 | 組み込みマクロ（`stdmacro.rb`/`stdio.rb`/`math.rb`/`unittest.rb`/castle の `macro.rb` をファイル名キーで Go 実装に固定）。マクロは AST（`[]any`）を返す | R1-f で typed AST ビルダーに |
| `internal/fc/allocator.go` | 509 | live range 計算、A/cond レジスタ割付、`DeleteUnuse` | R1-d で typed IR 対応、R3-a で `regalloc` へ |
| `internal/fc/llc.go` | 1203 | **LLC**: IR → ca65 アセンブラ。`CompileLambda` の巨大 switch、ピープホール（`index`+`pget/pset` 融合）、`extendJump` | R1-d で typed IR 対応、R3-a で `codegen` へ |
| `internal/fc/irdump.go` | 315 | IR / alloc-IR の golden ダンプ（Ruby `dumper.rb` 互換形式） | R1-d で typed IR 用に書き直し |
| `internal/fc/driver.go` | 431 | `Compiler.Build`: parse→HLC→LLC→ca65→base.asm/ld65.cfg 生成→ld65→（emu 実行） | R3-a で `driver` へ、R3-d で cwd 非依存に |
| `cmd/fcc/main.go` | 160 | CLI（`build`/`compile`/`run`）、`FC_HOME` 解決（`embed.FS` 展開含む） | R3-e で公開 API 経由に |
| `embedfs.go`（root, package `fcdata`） | 42 | `fclib/` `share/` の embed | 変更なし |
| `internal/r6502`, `internal/nes` | 862 / 585 | 6502 エミュレータ、ヘッドレス NES ランナー | 変更なし |
| `*_test.go` | | golden / examples / errors / allocator / base / textconv | R0 で `-update`、R3-d で並列化 |

### 2.2 データ表現

**AST**: `[]any`。先頭要素が `Sym` のノード種別。例: `[]any{Sym("if"), cond, then, else}`。
リーフは `int` / `string` / `Sym`（識別子）/ `*OMap`（options）/ `nil`。
`+=` はパーサで `(load X (add X rhs))` に脱糖され、**`X` の部分木が 2 箇所から共有**される（parser.y:132）。

**pos_info**: `*OMap`、キーは文 AST の `Canon()`（構造的等値）、値は `[filename, lineNo]`。
同じ内容の文が複数あると後勝ちで潰れる（エラー行番号ずれの原因）。`Hlc.updatePos` が
`curFilename/curLineNo` を更新し、`CompileError` に付与する用途**のみ**。asm には影響しない。

**const_eval**（`hlc.go:591`）: `[]any` を受けて `*Value` または AST を返す。**引数 AST を in-place で
書き換える**（`ast[1] = h.constEval(ast[1])` 等、8 箇所）。`compileLambda` は本体 AST を `deepCopyAST`
してから処理する。上記の `+=` 部分木共有と in-place 書き換えが組み合わさると、2 回目の訪問では
既に `*Value` に置換済みのノードを見る。**R1-c で純関数化するとき、この経路の IR が変わらないことを
`test_var` / `test_stat` の ir golden で確認する**（`+=` を含むのはこの 2 ファイル）。

**IR**: `Lambda.Ops [][]any`。`op[0]` が opcode の `Sym`。opcode 一覧（llc.go の switch より）:

```
label if jump return
push_result push_arg call push_fastcall_result push_fastcall_arg fastcall
load sign_extension add sub and or xor mul div mod shift_left shift_right uminus eq lt not
asm index ref pget pset index_pget index_pset
```

（`index_pget/index_pset` は `Llc.optimizePointer` のピープホールが生成する）
オペランド: `*Value` / `*CastedValue` / `*PointeredArray` / `*Lambda` / ラベル `string` / `int` / `nil`。

**Value**: `Kind`（local/global/literal/array_literal/module）、`Type`、`Id`、`Val`、`Opt *OMap`、
`BaseString`、割付結果（`Address`、`Location`: frame/reg/mem/none/a/cond/fastcall_reg、`LiveRange`、
`CondReg`、`CondPositive`）。`CastedValue`/`PointeredArray` は Value のラッパ（Ruby の Delegator の再現）。

**Module**: `Vars`、`Lambdas`、`Options *OMap`、`IncludeChrs/Asms/Headers`、`Modules *OMap`（use 先）、
`Scope`、`Defs []*Def`（kind: equ/bss/block/code）。**Lambda**: `Args`（compile 前は `[]any{id, *Type}`、
後は `*Value`）、`Type`、`Opt`、`Ast`、`Ops`、`Vars`、`Bank`、`Result`、`Defs`、`Asm`、`FrameSize`。

**Type**: `TypeOf(ast any)` が `[]any{Sym("pointer"), Sym("uint8")}` のような AST か `*Type` を受け、
`str`（`to_s`）でグローバル `typeCache` にインターン。`*Type` のポインタ同値で型比較している箇所がある
（例 `lmd.Type.Base == TypeOf(Sym("void"))`）→ インターンを維持する必要がある。

**Scope**: `Declares *OMap`（宣言順を保つ）、`uses []*Scope`（`use * from`）、`finding` フラグで
相互 use の無限再帰を防止。`IdList()` の列挙順が asm に影響する可能性があるため順序を保つ。

**マクロ**: `Hlc.defmacro(name, MacroFn)`。`include("stdio.rb")` 等は**ファイル名キー**で
`macros.go` の Go 実装に解決される。`MacroFn` は引数（`*Value` 列）とブロック AST を受けて AST を返す。

### 2.3 パイプライン（`Compiler.Build`、driver.go）

1. `NewHlc(libPath)` → `hlc.Compile(main.fc)`: `compileModule` が `use`/`include` に出会うと
   **再帰的に**相手モジュールのトップレベルを処理（宣言登録）。全モジュール登録後に
   全 Lambda 本体をコンパイル（`hlc.go:55`）。
2. `NewLlc(optLevel)` を**1 つ**作り、全モジュールを `llc.Compile(mod)` → `.fc-build/_<mod>.s` `.inc`
3. `ca65 -g` で各 `.s` → `.o`、`share/runtime.asm`・`fclib/<target>/runtime_init.asm` もアセンブル
4. `share/<target>/base.asm.erb` 相当を Go テンプレートで生成 → `base.o`
5. `ld65.cfg` 生成 → `ld65 -m <out>.map -o <out>`
6. `--run` なら r6502 で実行（emu ターゲット）

castle は 3 の後に独自 `data.asm` と独自 `ld65.cfg` でリンクする（`fcc compile` = 3 まで）。

### 2.4 採番と決定性の現状（C5 と衝突する点）

- `Hlc.curTmpCount`: **コンパイラ全体で 1 本**。`$N`（tmp 変数名）と `@<name>_N`（HLC ラベル）に使う。
  モジュール間で連番が続く
- `Llc.labelCount`: `NewLlc` は 1 回だけ → `@N`（LLC ラベル）も**全モジュール通し番号**
- モジュール処理順は `Hlc.Modules`（OMap）の登録順 = `use` に出会った深さ優先順

これらをモジュール単位に閉じるとラベル名が変わる（ROM バイトは変わらない）。R3-c で扱う。

### 2.5 保存すべき Ruby 由来の癖（R0〜R3 では直さない）

「直したくなる」ものの一覧。**直すと asm か挙動が変わる**ので触らない。R4 の対象。

| 場所 | 癖 |
|---|---|
| llc.go:464 | `lt` の符号判定が `op[2]` しか見ない（`or` の優先順位バグの忠実再現） |
| llc.go:771 | `storeA` が CastedValue の `location==:a` を考慮しない |
| allocator.go:41, 80, 289, 317 | PointeredArray の live range、`pget` の `op[3]` 欠落、`allocateCond` の location 残留、CastedValue の `==` 比較 |
| hlc.go:1036 | Ruby 版で未代入の `left` を参照して空文字列になるエラーメッセージ |
| hlc.go:800 | 整数演算は Ruby の floor 除算・floor 剰余 |
| hlc.go:242〜 | 非 void 関数の return 忘れが素通り（コメントアウトされた raise） |
| lexer.go:4 | 到達不能な数値の `-?`、`0b2` 容認、`\xZZ`→0、**文字列内改行を行番号に数えない** |
| lexer.go:15 | 常に CRLF→LF 変換して読む（Ruby のテキストモード再現） |
| types.go | `Value.new_int` の「-128 が sint16」境界 |
| 全体 | `Scope.IdList()` / `OMap` の列挙順（Ruby Hash の挿入順） |

例外として R1-a で直すもの（出力無変化を確認済み、§4.2）: `//\n` が次行を飲み込む、`/**/` 非対応。

### 2.6 テスト基盤の現状

- golden は `testdata/golden/` に `ast/`(50) `ir/`(14) `allocir/`(14) `asm/`(108) `bin/`(14) `stdout/`(26: .txt+.exit) `examples/`(2 ROM)。
  生成は **`ruby tools/gen_golden.rb`（Ruby オラクル）のみ** → Go 側に再生成手段が無い
- テキスト golden はチェックアウト時 CRLF（`core.autocrlf=true`、`.gitattributes` 無し）。比較は
  `normalizeText` で LF 化してから。**Go から書き出すときは LF で書き、`git status` がクリーンになることを確認**
- golden テストは `t.Chdir(repo/test)` に依存（`include`/`use` の相対パス解決のため）
- `TestErrorsFC`: `test/errors.fc` を `//@<正規表現>` 区切りで分割し、各断片が CompileError になり
  メッセージが正規表現に一致することを確認。**行番号は見ていない**（位置情報の改善は golden に影響しない）
- 実プロジェクト: `TestExampleMiku`（`fcc build` 相当）、`TestExampleCastle`（`fcc compile` → `ca65 data.asm` → `ld65`）
- `internal/nes`: `TestSmokeMiku` `TestSmokeCastle` `TestPlayCastle` `TestMesenPlayCastle`（Mesen 無ければ Skip）

---

## 3. R0 — 検証基盤の切り替え（0.5 日）

ゴール: golden を **Go 自身の出力から再生成できる**ようにし、Ruby オラクル依存を断つ。
合格条件（フェーズ全体）: `go test ./internal/fc -run TestGolden -update` → `git status` で
`testdata/golden/` に差分なし（ast 削除以外）。`go test ./...` 緑。

### R0-1 `.gitignore` と `.gitattributes`

- [ ] 未コミットの `.gitignore` の `/fcc`（Linux バイナリ）を含める
- [ ] `.gitattributes` を追加: `testdata/golden/** text eol=lf` に加え `*.bin binary` `*.nes binary` `*.o binary`。
      目的: Windows/Linux どちらで `-update` しても diff ノイズが出ないこと
- 合格: `git status` クリーン（`.gitattributes` 追加で既存ファイルが renormalize 差分を出す場合は
  `git add --renormalize .` して内容差分がないことを確認してから同コミットに含める）

### R0-2 ast golden の廃止

- [ ] `testdata/golden/ast/` を削除
- [ ] `TestGoldenAST`、`DumpAST`、`DumpPos`、`pretty_sexp` 相当（sexp.go の AST 専用部分）を削除。
      `SexpStr`/`EscStr` は irdump が使うので残す
- [ ] `doc/go_port_dump_format.md` の「AST ダンプ」節に「廃止（feature/v2 R0-2）」を追記
- 合格: `go test ./...` 緑

### R0-3 `-update` フラグ

- [ ] `golden_test.go` に `var update = flag.Bool("update", false, "golden を現在の出力で書き換える")`
- [ ] `compareText(t, name, got, want)` を「`*update` なら `got` を LF でファイルに書いて return」に。
      書き出し先は `readGolden` と同じ相対パス → 呼び出し側で golden 相対パスを渡す形に整理
      （現状は `readGolden(t, rel)` の結果を渡している。`compareGolden(t, rel, got)` のように rel を受ける API に変える）
- [ ] バイナリ（`TestGoldenBinary` の `.bin`/`.nes`、`examples_test.go` の ROM）も同様に `-update` 対応
- [ ] `TestGoldenStdout` の `.exit` も書き出す
- [ ] **初回再生成**: `go test ./internal/fc -run 'TestGolden|TestExample' -update` を実行し、
      `git status` が **クリーン**であることを確認する（Go 出力 == Ruby オラクル出力の最終確認。
      ここで差分が出たら R0-1 の改行設定の問題か、既存の不一致。後者なら報告して止める）
- 合格: 上記クリーン。`-update` 無しで再実行して緑

### R0-4 Ruby オラクルの役目終了の明記

- [ ] `tools/gen_golden.rb`、`tools/dumper.rb` の先頭コメントに
      「2026-09-12 以降（feature/v2）は使用しない。golden は `go test ./internal/fc -run TestGolden -update` で再生成する」を追記（G5: それ以外は触らない）
- [ ] `doc/go_port_dump_format.md` 冒頭に同趣旨の注記（ir/allocir の形式は R1-d で変わる予定であることも）
- [ ] `development_notes.md` の「テストの三段構え」節: golden 再生成の記述を `-update` に更新、
      ブランチ運用節に `feature/v2` を追記

### R0-5 ベンチマーク

- [ ] `internal/fc/bench_test.go`: `BenchmarkCastleCompile` — `examples/castle/src` を一時ディレクトリに
      複製し（G8）、`BuildOptions{Target:"nes", CompileOnly:true}` で HLC+LLC+ca65 まで。
      可能なら ca65 を除いた純 Go 部分（parse+HLC+LLC）だけのベンチも分ける（`BenchmarkCastleFrontend`）
- [ ] 計測値（ns/op, allocs/op）を §9 作業ログに**基準値として記録**。以後の R1〜R3 の各フェーズ末に再計測し、
      2 倍以上の退行があれば原因を調べる（性能改善は目標ではないが、静かな退行は検知する）

### R0-6 計画ドキュメント運用の切替

- [ ] `go_evolution_plan.md` の冒頭「状態」を更新し、R0〜R3 の実行計画は本書（v2_plan.md）が正典と明記
- [ ] 本書の進捗表を更新、R0 を ✅ に

---

## 4. R1 — 型付きフロントエンド・型付き IR（5〜7 日）

### 4.1 方針: ストラングラー方式

一気に全部を書き換えず、**新旧を同時に動かして差分テストで一致を確認しながら**旧を絞殺する。
各サブステップの合格条件は「asm/bin/stdout golden 差分ゼロ」+ そのステップ固有の差分テスト。

```
R1-a  新レキサ (internal/syntax)            差分テスト: 旧レキサとトークン列一致
R1-b  型付きAST + 新パーサ + lower(typed→[]any)  差分テスト: lower(新) == 旧パース結果 (S式文字列で比較)
R1-c  sema が typed AST を直接読む。lower 削除。旧 lexer/parser/sexp の AST 部分削除
                                            合格: ir/allocir/asm golden 差分ゼロ
R1-d  型付きIR (internal/ir)。llc/allocator を対応。ir/allocir golden を新形式で再生成
R1-e  Sym/OMap/Canon 全廃。types パッケージ。インターンをインスタンス持ちに
R1-f  マクロを typed AST ビルダーに
R1-g  パッケージレベル可変状態の棚卸しと排除
```

新しいコードは最初から**最終的なパッケージ**（§6.1 の配置）に置く。`internal/fc` は縮んでいく。

### 4.2 R1-a 新レキサ（0.5 日）

新規 `internal/syntax/`:

- [ ] `token.go`: `type Pos struct{ Offset, Line, Col int }`（Line/Col は 1 始まり、Col はバイト単位）、
      `type Kind int` + 全トークン種別の enum（現 parser.y の `%token` と記号トークン）、
      `type Token struct{ Kind; Pos, End Pos; Text string; Int int /* NUMBER */ ; Str string /* STRING 展開後 */ }`
- [ ] `lexer.go`: `NewLexer(src []byte, filename string) *Lexer`、`Next() (Token, error)`、
      `Comments() []Comment`（`type Comment struct{ Pos, End Pos; Text string }`、出現順）。
      **コメントはトークンとして返さずサイドリストに溜める**（goyacc 文法を汚さない）
- [ ] 字句規則は現 lexer.go と同一にする（記号の先頭一致順、キーワード 23 語、数値リテラルの Ruby `to_i` 準拠の部分パース、
      `\xNN` 等）。**§2.5 の癖は新レキサでも再現する**（`0b2` 容認、`\xZZ`→0、到達不能 `-?`）。
      ただし以下 2 点だけは正しい定義にする（コーパスでの出力無変化を確認済み）:
      - `//` コメントは `//` から行末まで（空 `//\n` が次行を飲まない）。castle/miku の `mmc3.fc:4-5` で
        飲まれていた行は自身もコメント行なので無影響
      - `/**/`（空ブロックコメント）を受理
- [ ] 行番号について: 新レキサの `Pos.Line` は**正確**（文字列内改行も数える）。旧 `lineNo` の癖は再現しない。
      旧レキサは R1-c まで残るので golden には影響しない。R1-c で `CompileError.LineNo` の出所が新 Pos に
      変わるが、TestErrorsFC は行番号を見ないので合格条件に影響しない（§2.6）
- [ ] `lexer_test.go`: (1) フィクスチャ（コメント・文字列・数値の各形式を網羅した小さな .fc）のトークン列と
      コメント列のテーブルテスト、(2) **差分テスト**: `test/*.fc` `fclib/**/*.fc` `examples/**/*.fc` 全ファイルで
      旧 `fc.Lexer` と新レキサのトークン列（種別・値）が一致。行番号は文字列内改行の扱いが異なるので比較対象外か、
      該当ファイルのみ除外
- 合格: 上記テスト緑。golden は無関係（旧パイプラインは未変更）

### 4.3 R1-b 型付き AST と新パーサ（1.5 日）

- [ ] `internal/syntax/ast.go`: ノード型。設計原則（C1, C2）:
      - 全ノードが `Pos() Pos` と `End() Pos` を返す（`Node` インターフェース）
      - `File{ Stmts []Stmt; Comments []Comment }`
      - 文: `VarDecl{Scope, Decls []*VarSpec}`, `ConstDecl`, `FuncDecl{Scope, Name, Params, Result, Options, Body}`,
        `IfStmt`, `LoopStmt`, `WhileStmt`, `ForStmt`, `SwitchStmt{Cases, Default}`, `ReturnStmt`, `BreakStmt`, `ContinueStmt`,
        `ExprStmt`, `BlockStmt`, `UseDecl{Module, As, FromAll}`, `IncludeDecl{Path, Kind(asm/macro/chr/header…)}`, `OptionsDecl`
      - 式: `Ident`, `IntLit`, `StrLit`, `ArrayLit`, `IncbinExpr`, `BinaryExpr{Op, X, Y}`, `UnaryExpr`,
        `AssignExpr{Op(=, +=, -=), Lhs, Rhs}`（**脱糖しない**。`+=` は AssignExpr のまま。脱糖は sema、C1/C2）,
        `CallExpr`, `IndexExpr`, `SelectorExpr`(`.`), `RefExpr`(`&`相当があれば), `CastExpr`, `CondExpr` 等。現 parser.y の
        アクションが作る S 式の種類を全部洗い出して 1:1 で型を用意する
      - 型式: `TypeExpr` 系（`BasicType`, `PointerType`, `ArrayType{Len Expr|nil}`, `FuncType{Params, Result, Fastcall}`）
      - `Options{Entries []OptionEntry{Key, Value Expr}}`（順序保持）
      - ノードは**不変として扱う**（ポインタ共有してもよいが書き換えない）
- [ ] `internal/syntax/parser.y`: 現 `internal/fc/parser.y` を**規則と優先順位は 1:1 のまま**、
      アクションだけ typed ノード生成に書き直す。`%union` にノード型別フィールドを置く（`any` で受けて
      アサーションしない）。`//go:generate goyacc -o parser.go -p syntax parser.y`
- [ ] `internal/syntax/parse.go`: `Parse(src []byte, filename string) (*File, error)`。
      構文エラーは `*syntax.Error{Pos, Msg}`。メッセージは現行同様 `parse error ...`（errors.fc が `/parse error/` で照合）
- [ ] `internal/fc/lower.go`（**一時的な境界コード**、ヘッダに「R1-c で削除」と明記）:
      `Lower(*syntax.File) (ast []any, posInfo *OMap)` — typed AST を現行の `[]any` S 式に変換。
      `+=` の脱糖（`X` 共有含む）もここで再現。`posInfo` は文ノードの `Pos().Line` から作る
      （キーは現行どおり `Canon`。collapse 挙動もそのまま再現される）
- [ ] `fc.ParseSrc` の中身を `syntax.Parse` + `Lower` に差し替える。旧 parser.y/parser.go/parser_driver.go/lexer.go は
      **差分テストのためにまだ残す**
- [ ] `lower_test.go` 差分テスト: コーパス全 .fc について `SexpStr(Lower(syntax.Parse(src)))` ==
      `SexpStr(旧ParseSrc(src))`、かつ posInfo の S 式も一致
- [ ] `ast_test.go`: 全ノードの Pos/End が単調（親は子を包含）であることをコーパス全体で検査する汎用テスト
- 合格: 差分テスト緑、`go test ./...` 差分ゼロ（golden は Lower 経由で完全一致するはず）

### 4.4 R1-c sema が typed AST を直接読む（2 日・最大の山）

- [ ] `hlc.go` の AST を読む全関数（`compileStatement`, `compileBlock`, `constEval`, `typeEval`, `lval`, `rval`,
      `compileLambda`, `compileModule`、マクロ呼び出し境界）を `syntax` ノードの型 switch に書き換える
- [ ] **`constEval` の純関数化**: AST を書き換えず、評価結果（`*Value` または「まだ評価できない式」）を返す。
      `deepCopyAST` 廃止。**ハザード**: §2.2 の `+=` 部分木共有 + in-place 書き換えの挙動。現行で
      2 回目の訪問が `*Value` を見ることで副作用（tmp の採番・IR の順序）が変わっている可能性がある。
      対処: `test_var` `test_stat` の ir golden が差分ゼロであることを最初に確認する。差分が出たら
      「共有ノードの評価結果をキャッシュして 2 回目は同じ `*Value` を返す」等、**現行と同じ IR になる**
      純関数的実装に調整する（正しい挙動に直すのではない）
- [ ] `pos_info` 廃止: `Hlc.curFilename/curLineNo` を「処理中ノードの Pos」から取る。`CompileError` に Col も持たせる
      （R2 で整理するが、ここで Pos を通しておく）
- [ ] マクロ境界: `MacroFn` はこの時点ではまだ `[]any` を返してよいが、sema 側で typed に変換せず済むよう
      **R1-f を先に小さく済ませるか**、一時的に「マクロ結果 `[]any` → typed」の逆 lower を置く。推奨は
      R1-c の中で `MacroFn` の戻り値を typed ノードにしてしまうこと（マクロは 5 ファイル・100 行程度）。
      その場合 R1-f は「マクロ API を綺麗にする」だけになる
- [ ] 旧コード削除: `internal/fc/lexer.go`, `parser.y`, `parser.go`, `parser_driver.go`, `lower.go`, `deepCopyAST`,
      `sexp.go` の AST 関連（`Canon` は OMap が使うので R1-e まで残る）
- 合格: **ir/allocir/asm/bin/stdout golden 全差分ゼロ**、TestErrorsFC 緑、examples ROM 一致、nes 緑。
  ベンチ再計測（§9 に記録）

### 4.5 R1-d 型付き IR（1.5 日）

- [ ] `internal/ir/`: `value.go`（`Value`, `CastedValue`, `PointeredArray` を移動。`Kind`/`Location`/`CondReg` を enum に）、
      `module.go`（`Module`, `Lambda`, `Def`。`Def.Kind` enum）、`op.go`:
      ```go
      type OpCode uint8 // Label, If, Jump, Return, PushResult, ..., IndexPset
      type Operand interface{ isOperand() } // *Value, *CastedValue, *PointeredArray, *Lambda, Label, Int
      type Op struct { Code OpCode; Args []Operand }  // まずはフラットに。opcode 毎の struct 化は任意
      ```
      `Lambda.Ops []Op`。`Lambda.Args` の「compile 前は `[]any{id, *Type}`、後は `*Value`」二相性を
      `Params []Param{Name, Type}` + `ArgVars []*Value` に分ける
- [ ] `hlc.emit` を typed に、`llc.CompileLambda` の switch・`allocator.go`・`optimizePointer`・`DeleteUnuse` を
      `Op`/`Operand` に対応。`isSameAny`/`isSameValue`/`symIn` 等の `any` 比較ヘルパは型 switch に
- [ ] `irdump.go` を typed IR 用に書き直す（形式は自由だが、Value の参照を `{l<index> <id>}` のように
      **安定に**表せること。ポインタ値を出さない）。`-update` で ir/allocir golden を新形式で再生成（G3 の例外）。
      再生成直前に**旧形式で差分ゼロ**であることを確認してからダンパを差し替える（順序厳守）
- 合格: asm/bin/stdout golden 差分ゼロ、examples ROM 一致、nes 緑。ir/allocir は新形式で再生成済み・再実行で差分ゼロ

### 4.6 R1-e Sym / OMap / Canon の全廃、types パッケージ（1 日）

- [ ] `internal/types/`: `Type` 移動。`TypeOf(ast any)` を廃止し `Void()`, `Bool()`, `Int(size, signed)`, `PointerTo(t)`,
      `ArrayOf(t, n)`, `Func(params, result, fastcall)`, `Module()`, `Macro()` のコンストラクタに。
      **インターンは `*types.Universe`（または `Interner`）インスタンス**が持ち、`Compiler` が 1 つ作って sema に渡す。
      ポインタ同値比較に依存する箇所（`== TypeOf(Sym("void"))` 等）は `t.Kind == types.Void` に置換
- [ ] `Value.Opt *OMap` → 明示フィールド（`LocalType`, `Segment`, `Address`, `Extern`, `Symbol`, `Bank` …。
      現在 `Opt` に入るキーを `grep 'Opt.GetOr\|opt.GetOr\|Set(Sym(' ` で全部洗い出す）
- [ ] `Module.Options`, `Hlc.Options` → `type Options struct` + 元の挿入順が必要な箇所（base.asm 生成、irdump）は
      順序付きスライス
- [ ] `Scope.Declares *OMap` → `map[string]*Value` + `order []string`
- [ ] `Sym` 型を削除（識別子は `string`、種別は enum）。`ToS` は残っていれば削除
- [ ] `Canon`, `OMap`, `base.go` の Ruby 互換ヘルパ削除
- 合格: 全 golden 差分ゼロ（irdump の出力形式は R1-d で決めたものを維持。Options の列挙順は旧 OMap の挿入順と同じにする）

### 4.7 R1-f マクロ API（0.5 日）

- [ ] `MacroFn` を `func(ctx *MacroContext, args []*ir.Value, block *syntax.BlockStmt) (syntax.Stmt, error)` 等の typed API に。
      `macros.go`/`textconv.go` の 5 マクロ群を書き直す。マクロの展開結果にも Pos を付与（呼び出し位置を継承）
- [ ] `include("xxx.rb")` のファイル名キー解決はそのまま（G6: castle が `include macro("macro.rb")` を使う）
- 合格: 全 golden 差分ゼロ（castle ROM 一致が `_T`/`_M`/`VERSION_STR` の検証になる）

### 4.8 R1-g パッケージレベル可変状態の排除（0.5 日）

- [ ] `grep -n '^var ' internal/**/*.go` で棚卸し。`typeCache`（R1-e で解消済みのはず）、その他
      `regexp.MustCompile` 等の不変なものは残してよい。可変なものは `Compiler`/`Sema`/`Codegen` のフィールドに
- [ ] `BuildPath`（`.fc-build`）は定数のままでよいが、R3-d でオプション化する前提でアクセスを 1 箇所に集約
- 合格: `go vet`、全 golden 差分ゼロ。R1 を ✅ に、ベンチ再計測

---

## 5. R2 — エラー処理の近代化（1 日）

- [ ] `internal/syntax/pos.go`（または `token.go`）: `Position{Filename string; Line, Col int}` と `String()`（`file:line:col`）
- [ ] `CompileError` → `sema.Error{Pos syntax.Position; Msg string}`（`Error()` は **`Msg` のみ**を返し続ける —
      TestErrorsFC の正規表現は `Msg` を想定。位置は `Pos` フィールドで提供し、整形は CLI の責務）
- [ ] panic/recover の棚卸し: `panic(&CompileError{...})` 全箇所を列挙。方針は「内部は panic 継続可、ただし
      **回復点は `sema.CompileModule`（R3-b で導入、暫定は `Hlc.Compile`）の 1 箇所**」。`parse` の `lexPanic` は
      `syntax.Parse` が error で返すようにして廃止
- [ ] 全エラーに Pos が付くことを保証: `sema` 内でノードを処理する入口で「現在位置」を設定し、panic 時に付与。
      位置が取れないエラー（ファイル読み込み等）は `Position{Filename}` のみ
- [ ] CLI 出力: `main.go` の `%s:%d: %s` を `%s:%d:%d: error: %s` に（列を追加。ここは golden 対象外）
- [ ] `errors_test.go` を拡張: 各断片について **Pos.Line が断片内の該当行を指す**ことも検査する
      （collapse バグが直っているので正しい行になるはず。ただし断片中のどの行が「正しい」かは断片ごとに
      人手で決める必要がある → まず `Line > 0` であることのみ検査し、厳密な期待行は `errors.fc` に
      `//@` 行のオプション記法で書けるようにする: 例 `//@can't init global variable @2`）
- 合格: 全 golden 差分ゼロ、TestErrorsFC（拡張後）緑

---

## 6. R3 — パッケージ構成・API・決定性・並列化（2〜3 日）

### 6.1 R3-a パッケージ分割（1 日）

目標配置（R1 で新規コードは既にここにある）:

```
internal/syntax    token, lexer, ast, parser(.y/.go), parse, (printer ← F-fmt で追加)
internal/types     Type, コンストラクタ, Universe(インターン), 互換型計算 (type_util)
internal/ir        Value, CastedValue, PointeredArray, Op, Lambda, Module, Def, ModuleInterface, dump
internal/sema      旧 hlc: AST→IR, Scope, constEval, マクロ, textconv
internal/regalloc  旧 allocator: live range, A/cond 割付, DeleteUnuse
internal/codegen   旧 llc: IR→asm, ピープホール, extendJump, base.asm/ld65.cfg テンプレート
internal/driver    旧 driver: パイプライン, ca65/ld65 起動, FC_HOME 解決 (main.go から移す), emu 実行
internal/r6502, internal/nes  変更なし
cmd/fcc            CLI のみ
```

- [ ] import 方向: `syntax` ← `sema` ← `driver`、`types`/`ir` ← `sema`/`regalloc`/`codegen` ← `driver`。
      **`syntax` は他の internal を import しない**（C3）。`go vet` に加えて import 規則のテスト
      （`go list -deps` を使う小さなテスト、または `depguard` 相当の簡易チェック）を 1 本置く
- [ ] 移動は機械的に（`gopls rename`/`gofmt -r`、または手作業）。**移動コミットではロジックを変えない**
- 合格: 全 golden 差分ゼロ

### 6.2 R3-b モジュール単位の sema（C4）（0.5〜1 日）

- [ ] `ir.ModuleInterface{ Id; Path; Options; Exports []*Value(public) ; Types…}`: importer が見てよいものだけ
- [ ] `sema.CompileModule(file *syntax.File, deps Resolver) (*ir.Module, error)` を導入。`Resolver` は
      `use`/`include` に対して依存モジュールを**宣言済み状態**で返す（現行の「`use` に出会ったら相手のトップレベルを
      即処理、本体は全モジュール登録後」という 2 相構造と解決順序は**そのまま維持**する。`test_cycle` が相互 use を
      テストしており、順序依存の部分可視性が現行仕様であるため）
- [ ] `Scope.uses` が相手モジュールの `Scope` を直接持つ構造を、`ModuleInterface` 経由に変える
      （列挙順・可視性は現行と同一に）
- [ ] 駆動側（driver）が依存グラフを持ち、`sema` はモジュール横断の状態（`Modules` OMap 相当）を持たない
- 合格: 全 golden 差分ゼロ（特に `test_cycle`、castle）。§7 に v2 の `use` 設計で見直すべき点を書き出す

### 6.3 R3-c 採番の決定性（C5）（0.5 日）— G2 の例外 (b)

ラベル名だけが変わり ROM バイトは変わらない作業。**2 コミットに分ける**:

1. [ ] `golden_test.go` に `normalizeLabels(asm string) string` を追加: `@<name>_N` / `@N` / `$N` を
       ファイル内出現順に `@<name>_L1`… `@L1`… `$T1`… へ正規化。`compareGolden` の asm 比較はこれを通す。
       **この時点では golden との差分ゼロ**（正規化は現行出力にも適用されるので恒等的に一致する。
       golden ファイル自体は書き換えない）→ コミット
2. [ ] `sema` の tmp/label カウンタを Lambda 単位（または Module 単位）に、`codegen` のラベルカウンタを Module 単位に。
       asm 差分は**正規化後ゼロ**、`bin`/ROM/stdout は**バイト一致**（ラベル名は機械語に出ない）。
       確認後 `-update` で asm golden の生テキストを新しいラベル名で再生成 → コミット
- 合格: 上記。§9 に「どのラベルがどう変わったか」を 1 行記録

### 6.4 R3-d 作業ディレクトリ非依存とテスト並列化（C7）（0.5 日）

- [ ] `driver.BuildOptions` に `Dir`（ソースの基準ディレクトリ）、`BuildDir`（既定 `<Dir>/.fc-build`）、
      `Stdout/Stderr io.Writer` を追加。`include`/`use` の相対パス解決は `Dir` 基準に。
      **CLI の既定値は現行どおり**（cwd と `.fc-build`）で G6 を守る
- [ ] テストから `t.Chdir` を除去し、`t.Parallel()` を解禁（golden の各サブテスト、errors 断片）。
      examples テストは一時ディレクトリ複製方式（G8）のまま並列化
- 合格: `go test ./internal/fc -race` 緑（`-race` は R3-d 以降常用）、テスト時間を §9 に記録

### 6.5 R3-e 公開 API と CLI（0.5 日）

- [ ] `pkg/fc`（または `internal/driver` を直接）: `type Compiler`, `func New(opts) *Compiler`,
      `func (c *Compiler) Build(ctx, src string, opt BuildOptions) (*Result, error)`。`Result` に生成物パス、
      マップ、診断（`[]sema.Error`）
- [ ] `cmd/fcc/main.go` をこの API の薄い皮に。`resolveFCHome` は driver へ
- [ ] 未使用フラグ `-S`（Ruby 版でも実質未使用）は**残す**（G6。機能するかは問わないが受理はする）
- 合格: 全テスト緑、`fcc build/compile/run` の手動確認（examples/miku と castle の手順を 1 回ずつ）。R3 を ✅ に

---

## 7. 未決事項（ユーザー判断待ち・エージェントは追記のみ）

実装中に見つかった「仕様として決めるべき点」をここに溜める。決まるまでは現行挙動維持。

- [ ] **相互 `use` の可視性**（R3-b）: 現行は `use` 到達順に依存した部分可視性。v2 の `use` 設計で
      「DAG 強制」か「宣言フェーズの分離で順序非依存」かを決める
- [ ] `use * from` の継続可否（v2）: 名前空間汚染の観点で見直し候補
- [ ] `include("xxx.rb")` のマクロ機構（ファイル名キーの Go 実装固定）を v2 でどう一般化するか
- [ ] 文法バージョン宣言の表記（ファイル先頭 `#fc 2` 等）と、宣言が無いファイルを v1 と見なすか
- （実装中に追記）

---

## 8. 将来フェーズへの引き継ぎメモ

本計画完了後の最初の仕事は F-fmt（フォーマッタ + 文法バージョン機構）で、次に文法 v2、その後に F-mod。
詳細は [go_evolution_plan.md](go_evolution_plan.md) Part B。本計画で用意しておくもの:

- F-fmt: `syntax.File` がコメントと Pos/End を持つ（C1）→ `syntax/printer.go` を書くだけで `fcc fmt` になる。
  冪等性テスト `fmt(fmt(x)) == fmt(x)` と往復テスト `parse(fmt(x)) ≡ parse(x)`（Pos を除いた構造比較）が
  ast golden の後継になる。マイグレーションは「v1 パーサ → 共通 AST → v2 プリンタ」（C2）
- F-mod: `sema.CompileModule` + `ir.ModuleInterface`（C4）が入口。残る作業はインターフェースの
  シリアライズ、依存グラフの永続化、内容ハッシュによるキャッシュ。`use` の文法変更は v2 文法に同梱する
  （マイグレーションを利用者に 2 回強いないため）
- 複数エラー報告（R2 から除外）: 文単位のリカバリは「最初のエラー以降の挙動」が変わるため挙動変更扱い。
  F-fmt の後、`fcc check` と一緒に入れる

---

## 9. 作業ログ

### 2026-09-12 — 計画策定

- `agent/golang` 45c2d78 から `feature/v2` を分岐
- Fable 5.1 で R0〜R3 を再計画。主な変更点（旧 go_evolution_plan.md の R0〜R3 との差分）:
  - R1 を 7 サブステップに分解し、ストラングラー方式（新旧並走 + 差分テスト）を明文化
  - 将来要件（フォーマッタ / 文法 v2 / モジュール単位コンパイル）から設計制約 C1〜C7 を導出し R1/R3 に配置
  - レキサのコメント保持を R1 の先頭（R1-a）に置いた。`//\n` 飲み込みバグ修正はコーパス上の出力無変化を
    確認済み（castle/miku の `mmc3.fc:4-5` のみ該当、飲まれる行もコメント）
  - R2 から「複数エラー報告」「メッセージ文言改善」を除外（挙動変更のため R4 以降）
  - R3-c のラベル採番変更は「正規化を先に入れてから切り替える」2 コミット手順に
  - ir/allocir golden は削除ではなく R1-d で新形式に再生成する方針に（レジスタ割付の回帰検知を残す）
- 基準値: `go test ./...` 約 75 秒（fc 20s, nes 16s）。`go vet` クリーン
