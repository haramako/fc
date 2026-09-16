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

- **文法 v2**（2026-09-14〜）: 先頭行 `#fc 2`。仕様は [language_reference.md](language_reference.md)、設計の経緯は
  [v2_grammar.md](v2_grammar.md)。**fclib は v2 に移行済み**、`test/*.fc` は v1 のまま残し（v1 パーサの回帰）、
  v2 版を `test/v2/` に置いて `TestGoldenV2` で同じ golden に一致させている。`test/v2/` は `test/*.fc` を
  変えたら `fcc migrate` で作り直す（`cd test/v2 && cp ../test_*.fc ../cycle_use.fc . && fcc migrate -w test_*.fc`）
- **struct / soa**（2026-09-14〜、v2 のみ）: 設計と実装メモは [v2_types_struct.md](v2_types_struct.md)、仕様は
  [language_reference.md](language_reference.md) §2.1 / §2.2。テストは `internal/driver/struct_test.go` / `soa_test.go`
  （小さなプログラムを emu で実行）と `test/v2/test_struct.fc` / `test_soa.fc`（unittest 形式。golden は無く、実行結果で判定）。
  `share/runtime.asm` の `__mul_16`（16 ビット乗算）は長らく `rts` だけの未実装で、このとき実装した（`j * 100` が 0 になっていた）
- **`fcc migrate`**: `fcc migrate [-t nes] [--lib DIR] [--textmap NAME=PATH] -w <main.fc>...`。作業ディレクトリと
  `--lib` の下のファイルだけを書き換え、再コンパイルして asm（正規化）が一致しなければ元に戻す（`--force` で受け入れ）。
  castle は `cd src && fcc migrate -t nes --textmap _T=../tmp/font/text.chr.txt --textmap _M=../tmp/font/misc_text.chr.txt -w main.fc`
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
  `FC_SZP` / `FC_SRAM` と `_SIZE` の export を足す（examples/castle/src/data.asm）
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
  最初 crc16 / textprint / oam で退行していて、切って比べてコストモデルの間違いが 3 つ見つかった）
- **生成コードのベンチマーク**（2026-09-15〜）: `go test ./bench`。[bench/](../bench/README.md) の 12 本の .fc を emu で走らせ、
  `stdio.bench_start` / `bench_end` で囲んだ区間のサイクル数（`r6502.Cpu.Cycles`。ページクロス・分岐成立込みで決定的）と
  モジュールのセグメントサイズを `bench/results.json` と比べる。出力（チェックサム）の違いはコンパイラのバグ、
  サイクル数・サイズの違いは最適化の効果か退行で、どちらも `-update` で受け入れる（golden と同じ運用）。
  ベンチを書いたときに 16 ビットの除算・剰余のバグが 3 つ見つかった（`__mod_16` が stub、符号付き 16 ビットが符号無し除算、
  2 のべき乗の符号付き除算の `cmp $80`）。**新しい種類のコードを書くときは Python などで同じ計算を再現して照合する**と
  コンパイラのバグがすぐ見つかる
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

## 実プロジェクトのビルド構成（参考）

- **fc-miku**: `fcc build -t nes miku.fc` だけで完結（fc 標準ドライバ）
- **castle**: `fcc compile -t nes main.fc` → `ca65 data.asm` → 独自 `ld65.cfg` でリンク
  （+ NSD サウンドドライバ、生成物リソースは「出来合い許容」ポリシー。
  詳細は [../examples/README.md](../examples/README.md)）
