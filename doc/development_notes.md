# 開発メモ（環境・運用・ハマりどころ）

fc を開発するときに知っておくべきこと。残っている仕事は [roadmap.md](roadmap.md)、
言語仕様は [language_reference.md](language_reference.md)。
（2026-09 以前の計画書・作業ログは [archive/](archive/)。fc はもともと Ruby で書かれていて 2026-08〜09 に Go に
移植した。Ruby 版はタグ `ruby-frozen` に残っている。Ruby 版との互換は以後考慮しない）

## ブランチ運用

- 開発は **`feature/v2`**（2026-09-12〜。文法 v2 / struct・soa / far call / ベンチ / 最適化）で行う。
  **安定するまで master へはマージしない**（一度マージしたが取り消し済み。master = 8358b15 のまま）。
  `agent/golang` は Go 移植のブランチで、feature/v2 の親
- タグ: `v0.0.2`（2026-09-15、最適化前のベースライン）、`ruby-frozen`（Go 移植前の Ruby 版。
  `fclib/math.fc` / `share/runtime.asm` の sin / atan / rand / 乗算テーブルは `misc/table.rb` の生成物でタグから参照できる）、
  `go-strict-clone`（移植直後の基準点）
- **push 注意**: origin は公開の github.com/haramako/fc。
  `examples/castle` は製品コード（ゲームテキスト・リソース含む）なので、
  **push する前に公開可否の判断が必要**

## 開発環境

| 項目 | 状態 |
|---|---|
| Go | 1.24.5 windows/amd64 |
| cc65 | `C:\Applications\cc65-snapshot-win32\bin\` (PATH 通過済み) |
| MesenCE | 2.2.1 を `C:\Applications\MesenCE\Mesen.exe` に導入済み |

- castle 実プロジェクト側で Go 版を使うには `$env:FCC="C:\Work\fc\fcc.exe"`（Rakefile が FCC 環境変数を見る）

## PowerShell 5.1 のエンコーディング罠

- `Get-Content`/`Set-Content` は UTF-8 ファイルをシステム ANSI (Shift-JIS) で読むため、
  **ラウンドトリップすると日本語が全て文字化けする**（過去に計画書を一度破壊した）。
  ファイルの編集はエディタ/専用ツールで行い、PowerShell はビルド・git・コピーのみに使う
- 日本語を含む `.ps1` スクリプトは **UTF-8 BOM 付き**でないと PS5.1 がパースエラーになる

## テストの三段構え

```bash
go test ./...                                    # 全部 (golden + examples + NESスモーク)
```

| 層 | テスト | 時間 | 何を保証するか |
|---|---|---|---|
| golden差分 | TestGolden*（internal/driver） | 数秒 | コンパイラ出力の全段階が基準と一致 |
| ROMバイト一致 | TestExampleMiku / TestExampleCastle | 〜5秒 | 実プロジェクト2つのROMがスナップショットと一致 |
| 内蔵スモーク | TestSmoke* / TestPlayCastle（internal/nes） | 〜1秒 | 起動・NMI/IRQ・描画・自動プレイでの画面遷移 |
| 実機精度 | TestMesenPlayCastle | 〜10秒 | MesenCE 上での自動プレイ（エリア変数で判定） |

- **文法 v2**（2026-09-14〜）: 先頭行 `#fc 2`（任意）。仕様は [language_reference.md](language_reference.md)、設計の経緯は
  [v2_grammar.md](v2_grammar.md)。**v1 は 2026-09-19 に削除**（`fcc migrate`、`internal/migrate`、`.rb` マクロの互換、
  `test/*.fc` の v1 版）。`test/*.fc` は v2 だけ。`syntax.File` / `ir.Module` にバージョンは無く、v1 だけの構文は
  `syntax.checkVersion` が「v2 ではこう書く」のエラーにする
- **struct / soa**（2026-09-14〜、v2 のみ）: 設計と実装メモは [v2_types_struct.md](v2_types_struct.md)、仕様は
  [language_reference.md](language_reference.md) §2.1 / §2.2。テストは `internal/driver/struct_test.go` / `soa_test.go`
  （小さなプログラムを emu で実行）と `test/test_struct.fc` / `test_soa.fc`（unittest 形式。golden は無く、実行結果で判定）。
  `share/runtime.asm` の `__mul_16`（16 ビット乗算）は長らく `rts` だけの未実装で、このとき実装した（`j * 100` が 0 になっていた）
- **far call**（2026-09-15〜）: `options(farcall: true)` で有効。判定は `sema.Hlc.isFarCall`（呼び先モジュールの
  `ir.Module.Switchable()` = `bank` ≥ 0 かつ `near` 無し）、`ir.Op.Far`、codegen の `farCallSetup`（`.bank()` を使うので
  ld65.cfg の MEMORY に `bank = N` が要る。fc 生成の cfg は自動）。トランポリンは `fclib/<target>/farcall.asm`
  （emu / MMC0 は fc が用意）、MMC3 は `fclib/nes/farcall_mmc3.asm` を参考にプロジェクトが用意。テストは
  `internal/driver/farcall_test.go`。設計は [v2_farcall.md](v2_farcall.md)
- **エラー報告**（2026-09-15〜）: 意味解析のエラーは `panic(&diag.Error{})` のままだが、`compileStatementRecover` が文ごとに
  回復して `Program.Errors` に集める（スコープ・ループのスタックは文の前に戻す）。失敗した宣言の名前は `types.Bad` 型で束縛し、
  それに触れる式は `Suppressed` なエラーで黙って打ち切る（報告しない）。上限 `sema.MaxErrors` で `Fatal` を投げて打ち切り。
  呼び出し側は `diag.ErrorList`（`errors.As` で先頭 1 件、`diag.Errors(err)` で全部）。新しいエラーを足すときは
  「主語（`describe(v)`）と型を添える」「巻き添えなら Suppressed」を守る。パースエラーは goyacc の verbose 出力を
  `tokenDisplay` で綴りに直している
- **VS Code 拡張**（2026-09-15〜）: `editors/vscode/`。TextMate 文法 + `fcc fmt` / `fcc check` を呼ぶだけの薄い拡張
  （LSP なし）。`npm install && npm run check-grammar`（文法をリポジトリの全 .fc でトークン化して検査）、
  `npm run package` で VSIX、`code --install-extension fc-lang-*.vsix`。文法を足したら `syntaxes/fc.tmLanguage.json` と
  `scripts/check-grammar.js` の期待値を更新する
- **ca65 / ld65 の探索**（2026-09-14〜）: `internal/driver/tools.go` の `ToolPath`。`FC_CC65_BIN` → fcc の実行ファイルと
  同じディレクトリ（と `cc65/`, `bin/`）→ PATH の順。リリース（`release.yml`）は Linux amd64（cc65 をソースから静的ビルド）と
  Windows（公式スナップショットの 32 ビット exe）に ca65 / ld65 を同梱する。`fcc version` が解決先を表示する
- **CI / リリース**（2026-09-14〜）: push ごとに `.github/workflows/ci.yml`（Linux + Windows、cc65 導入、`go vet` / `go test`）。
  タグ `v*` を push すると goreleaser がバイナリを GitHub Releases に置く。`fcc version` でバージョンとリビジョン
- **`fcc check`**（2026-09-14〜）: ファイルを作らずにコンパイルしてエラーと警告を出す。警告は build でも出る
  （`file:line:col: warning: ...`）。構文の検査は `internal/syntax/lint.go`、意味解析側は `Hlc.warn`
- **`fcc fmt`**（2026-09-12〜）: `fcc fmt -l <files>` で未整形のファイルを列挙、`-w` で上書き、`-d` で差分。
  正規形は [internal/syntax/printer.go](../internal/syntax/printer.go) 先頭のコメントと `TestFormatStyle` が定義。
  **リポジトリ内の .fc はまだ整形していない**（castle は製品コードなので一括整形はオーナー判断。整形しても asm は変わらない）
- golden の再生成（feature/v2 以降）: **`go test ./internal/driver -run 'TestGolden|TestExample' -update`**。
  Go 自身の出力で上書きする（形式は [golden_dump_format.md](golden_dump_format.md)）。**意図しない差分を `-update` で消さない**
  （`-update` の前に差分を読んで、意図した変化だけを受け入れる）
- golden は `.gitattributes` で `eol=lf` に固定してあり、`-update` 後に `git status` がクリーンなら
  出力が完全一致している
- **fc ソースのテスト（`test/test_*.fc` の assert 群）は TestGoldenStdout が
  コンパイル→実行して stdout・終了コードごと検証する**（assert 失敗 = exit 1 + ERROR 出力
  で必ず不一致になる）。テスト .fc を新規追加したら golden ディレクトリに空ファイルを置くか
  `-update` で生成する
- 性能退行の検知 (コンパイラ自身の速度): `go test ./internal/driver -run xxx -bench BenchmarkCastle -benchmem`
- **フレームの静的割付**（2026-09-16〜、[v2_frame_alloc.md](v2_frame_alloc.md) §6）: コンパイルの順序は
  sema（全モジュール）→ `codegen.PrepareProgram`（`frames.Analyze` で ABI を決める → 全関数の opt + regalloc →
  `frames.Place` で配置）→ `_frames.inc` を書く → 各モジュールの codegen → ca65。**codegen を直接呼ぶテストは
  `PrepareProgram` を先に呼ぶ**（golden の `newLlcForGolden`）。castle など base.asm を自前で持つプロジェクトは
  `FC_SZP` / `FC_SRAM` と `_SIZE` の export、スタックの空き先頭 `FC_SP` を足す（examples/castle/src/data.asm）
- **最適化のパイプライン**（2026-09-16〜）: sema（IR 生成）→ `internal/opt`（IR→IR。`ir.BuildCFG` / `ir.BuildUseDef` の上に
  書く小さなパスの列: ポインタ融合、コピー除去、ジャンプ整理、ループ回転…）→ `internal/regalloc` → `internal/codegen`
  （命令選択 + asm テキストのピープホール `peephole.go`）。パスを足したら `go test ./bench -v` で効果を見て、
  golden（asm / allocir）は差分を眺めてから `-update`。**ハマった点**: (1) 命令を融合したら regalloc の A 割付の
  対象リスト（`allocateA` の producer / consumer）と `DeleteUnuse` の対象にも足す（足さないと退行する）、
  (2) `p = p.next` のように読み先がポインタ自身のとき `(p),y` で直接読むと下位バイトを書いた後に上位を読んで壊れる、
  (3) A の値の追跡でグローバル変数を追うと `options(address:)` の I/O レジスタ（`$2002` は読むたびに変わる）まで
  消してしまい castle が止まる（内蔵 NES ランナーでは再現せず、`TestMesenPlayCastle` で発覚）。**castle を変える最適化は
  Mesen のテストまで通してからコミットする**、(4) 「x を先に書き換えてよい」型のパス（chainInPlace）は後続の命令の
  **全ての**入力が x を読まないことを確かめる（`x = (x << 1) ^ x` が壊れていた。golden / bench に出ない形だったので
  `internal/driver/ops_test.go` に小さな実行テストを足して守る）、(5) asm のピープホールでオペランドを文字列で
  分類するとき: `<S+n,x`（フレーム）は X が関数内で一定なので正確な場所として扱えるが **X を変える命令
  （ldx / inx / dex / tax / call）で状態を捨てる**こと、`sym+0,y`（グローバル配列）はゼロページの局所と重ならないので
  A / Y の追跡を壊さなくてよい（これを「どこを書いたか分からない」扱いにしていて Y の追跡がほぼ効いていなかった）、
  (6) A 割付は「定義の直後に使用」が条件なので、IR の並びを変えるパス / sema の変更（引数の先行評価で `push_result` が
  `sub t; push_arg t` の間に入った）で黙って外れる。ベンチで `fib` が +7% になって気づいた。並びを変えたら bench を見る、
  (7) 最適化の効果は **`FC_NO_RESIDENT=1` のように環境変数で切れるようにして A/B で測る**（ループ内の A 常駐は
  最初 crc16 / textprint / oam で退行していて、切って比べてコストモデルの間違いが 3 つ見つかった）、
  (8) bench のサイクル数は **コードの配置でも動く**（分岐のページまたぎ +1、`abs,y` のページまたぎ +1）。サイズが
  減ったのにサイクルが増えたら（`tay` 化で bgdecode +0.6%）、`.s` を diff して生成コードの差を確かめてから判断する、
  (9) レジスタに変数を置いたまま命令を実行する（friendly）判定は「その命令がレジスタを壊さない」ことまで含める
  （符号付きの `lt` は `cmp` でなく `sec; sbc` で A を壊す）。bench は出力（チェックサム）も照合するので、こういう
  壊れ方はそこで捕まる。**ベンチの表だけ grep して出力の行を落とさない**こと、
  (10) `allocateA`（一時変数を A に残す）は「定義の直後に **第 1 入力** として使う」形にしか効かない。IR を作る側
  （sema / opt）は可換な演算なら直前の一時変数を第 1 入力に置く（`opt.commuteTemp`。これだけで entities / oam が -4%）、
  (11) **コンパイル時間も見る**。常駐の候補の組み合わせ探索（A × Y × X で候補数の 3 乗）は、関数全体を領域にした
  途端に castle のコンパイルが 1.7 秒 → 55 秒になった（`TestExampleCastle` が 50 秒になっていたのに気づかなかった）。
  レジスタごとに単独の得で上位 4 つに絞ってから組み合わせる（`bestPair`）ことで 1.8 秒。目安: castle（440 関数、
  約 1.2 万行）の `fcc compile` は 2 秒以内、`go test ./internal/driver` の castle は 3 秒以内。
  **番をしているテスト**: `TestExampleCastle` はコンパイル時間をログに出し 10 秒を超えると fail、`go test ./bench` は
  12 本のビルドと実行が 30 秒を超えると fail（どちらも通常の 5 倍以上の余裕）
- **インライン展開**（`opt.InlineProgram`、2026-09-19）は `codegen.PrepareProgram` の最初（`frames.Analyze` の前）に
  プログラム全体で 1 回。IR の呼び出し列 `push_result; push_arg…; call` を、呼び先の変数・ラベルを付け替えた本体で
  置き換える（`return v` は `load 結果 = v; jump 終端`）。far call の `Far` フラグは sema が呼び先のモジュール基準で
  付けているので、**呼び出しを含む本体は別モジュールに写さない**（葉関数だけ）。他の呼び出しの引数の中も展開しない
  （fastcall の引数領域）。fclib の `math.abs` / `rand` / `sign` が `options(inline: true)`（castle で 110 か所）
- `fcc -O 0` は最適化パス・常駐・ピープホールを切る（`BuildOptions.OptimizeLevel` は 0 が「未指定 = 2」、-1 が -O 0）。
  レジスタ割付は -O 0 でも同じ `regalloc.AllocateRegister`（静的フレームの関数は固定番地に置く必要があるので、
  「全部フレーム」の簡易版は使えない）。`TestOptimizeLevel0` が番
- **生成コードのベンチマーク**（2026-09-15〜）: `go test ./bench`。[bench/](../bench/README.md) の 12 本の .fc を emu で走らせ、
  `stdio.bench_start` / `bench_end` で囲んだ区間のサイクル数（`r6502.Cpu.Cycles`。ページクロス・分岐成立込みで決定的）と
  モジュールのセグメントサイズを `bench/results.json` と比べる。出力（チェックサム）の違いはコンパイラのバグ、
  サイクル数・サイズの違いは最適化の効果か退行で、どちらも `-update` で受け入れる（golden と同じ運用）。
  ベンチを書いたときに 16 ビットの除算・剰余のバグが 3 つ見つかった（`__mod_16` が stub、符号付き 16 ビットが符号無し除算、
  2 のべき乗の符号付き除算の `cmp $80`）。**新しい種類のコードを書くときは Python などで同じ計算を再現して照合する**と
  コンパイラのバグがすぐ見つかる
- **castle のマクロベンチ**（2026-09-19）: `go test ./internal/nes -run CastleFrameCycles -v`。内蔵 NES ランナーが
  `_ppu_vsync_flag` を読む `lda; bne` の待ちループを idle と数え、局面ごとの 1 フレームの busy サイクルを
  `bench/castle_frames.json` と比べる（`-update` で更新。[bench/README.md](../bench/README.md)）。bench/ の 12 本と
  違って実ゲームの 1 フレームの重さが見える（フィールドで約 10,200 サイクル = 34%）。最適化の効果は両方で見る
- ca65 は既定で CPU 数だけ並列に走る。ca65 のエラー調査などで逐次にしたいときは `fc.Options.Jobs = 1`
  （CLI にはフラグ無し）
- examples と実プロジェクトの同期・差分確認: `tools/sync_examples.ps1`（詳細は
  [../examples/README.md](../examples/README.md)）
- 内蔵NESランナーのスクリーンショット: `FC_NES_SNAPSHOT_DIR=<dir> go test ./internal/nes`
- **注意: `go test ./...` はパッケージを並列実行する**。examples をビルドするテストを
  新設するときは、リポジトリ内の `examples/*/src/.fc-build` を共有しないこと
  （`internal/nes` の Mesen テストは一時ディレクトリに複製してからビルドしている。
  共有すると片方の RemoveAll でもう片方のビルドが壊れ、単独実行では再現しない
  フレーク不良になる）

## MesenCE の導入とハマりどころ

導入: [MesenCE releases](https://github.com/nesdev-org/MesenCE/releases) の
`Mesen_x.x.x_Windows.zip` を `C:\Applications\MesenCE` に展開
（別の場所なら環境変数 `FC_MESEN` に Mesen.exe のパスを設定）。

ヘッドレス実行の罠（TestMesenPlayCastle が自動処理するものも含む）:

1. **SmartScreen**: ダウンロードした exe は Mark of the Web 付きで、ヘッドレス起動すると
   見えないダイアログ待ちで無音のままハングする → `Unblock-File` で解除
2. **初回起動**: `settings.json` が exe と同じ場所に無いと初回ダイアログで停止する。
   さらに `Nes.Port1.Type` にコントローラを設定しないと **`emu.setInput` が無視される**
   （ポート未接続扱い。`emu.getInput` が空テーブルを返すのが症状）。
   テストは settings.json が無ければ最小構成を自動生成する
3. **testrunner の Lua では `io`/`os` が使えない**（設定でも解除不可）。
   テスト結果は `emu.stop(exitCode)` の終了コードで返す設計にする。
   `emu.log` の出力は stdout には出ない
4. Lua API 覚え書き: `emu.setInput(inputTable, port)`（inputPolled イベント内で呼ぶ。
   キーは a/b/select/start/up/down/left/right の bool）、
   `emu.read(addr, emu.memType.nesDebug, false)`（副作用なし読み取り）

シンボルアドレスは ld65 のマップファイル（`-m`）から取得する
（`tools/` 相当の処理は `internal/nes/mesen_test.go` の `parseLd65Map`）。

### Mesen でのソースレベルデバッグ（`fcc build -g`、2026-09-19）

`fcc build -t nes -g -o game.nes main.fc` は ROM の隣に **`game.dbg`**（ld65 の `--dbgfile`。fc のソース行を含む）と
**`game.mlb`**（Mesen 2 形式のラベル: `NesPrgRom:<offset>:_mod_func`、`NesInternalRam:<addr>:_mod_var`）を書く。
Mesen は ROM を開くとき同じ名前の `.dbg` / `.mlb` を自動で読むので、デバッガに関数名・変数名が付き、
`.fc` の行でブレークポイントとステップ実行ができる（cc65 の C ソース対応と同じ仕組み: codegen が命令ごとに
`.dbg line, "file", N` を出し、ca65 `-g` が `type=1` の行レコードにする）。`.dbg` のファイル名は ROM の隣からの
相対パスなので、ROM とソースの位置関係を変えたら作り直す。`.dbg` は `-g` 無しでも常に書く（`--size-report` /
`fcc size` / マクロベンチのプロファイルが使う）。自前で ld65 を呼ぶプロジェクト（castle）は `--dbgfile` を足し、
`.mlb` は `fcc size` と同じ `driver.DbgFile.WriteMlb` で作れる（CLI は未提供）。

### エディタ連携（`fcc watch`、`fcc check --json`、`tools/vscode-fc/`）

`fcc watch [-t nes] [-o FILE] [-g] [-c] main.fc` はソースディレクトリと fclib の下の `.fc` / `.asm` / `.inc` / `.chr` /
`.txt` / `.cfg` の更新時刻を 0.5 秒ごとに見て、変わったら再ビルドして結果を出す（外部ライブラリ無し）。
`fcc check --json` は診断を 1 行 1 JSON（`file` / `line` / `col` / `severity` / `message`）で出す。
`tools/vscode-fc/` はそれを使う VS Code 拡張（plain JS、ビルド不要。ハイライト + 保存時に Problems へ。
`fc.mainFile` にプログラムの起点を書けばどのファイルを保存しても全体を検査する）。言語サーバは持たない。

### コードサイズ（`fcc build --size-report` / `fcc size game.dbg`）

セグメントごとの合計と、関数（ラベル）ごとの大きさ（次のラベルまで。関数の後ろの定数表を含む）を大きい順に出す。

## 実プロジェクトのビルド構成（参考）

- **fc-miku**: `fcc build -t nes miku.fc` だけで完結（fc 標準ドライバ）
- **castle**: `fcc compile -t nes main.fc` → `ca65 data.asm` → 独自 `ld65.cfg` でリンク
  （+ NSD サウンドドライバ、生成物リソースは「出来合い許容」ポリシー。
  詳細は [../examples/README.md](../examples/README.md)）
