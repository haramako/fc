# 開発メモ（環境・運用・ハマりどころ）

Go移植後の fc を開発するときに知っておくべきこと。
計画・経緯は [go_port_plan.md](go_port_plan.md)（移植、完了）と
[go_evolution_plan.md](go_evolution_plan.md)（今後の進化計画）を参照。

## ブランチ運用

- 開発は **`agent/golang`** で行う。**安定するまで master へはマージしない**
  （一度マージしたが取り消し済み。master = 8358b15 のまま）
- **`feature/v2`**（2026-09-12、`agent/golang` 45c2d78 から分岐）: Go らしい実装への転換
  （R0〜R3）と、その後の文法 v2 / フォーマッタ / モジュール単位コンパイル。
  作業指示書は [v2_plan.md](v2_plan.md)。厳密クローンの基準点は `agent/golang` に残る
- タグ:
  - `ruby-frozen` — 移植前の Ruby 版オリジナル
  - `go-strict-clone` — 厳密クローン完了・castle 動作確認済みの基準点
- **push 注意**: origin は公開の github.com/haramako/fc。
  `examples/castle` は製品コード（ゲームテキスト・リソース含む）なので、
  **push する前に公開可否の判断が必要**

## 開発環境

| 項目 | 状態 |
|---|---|
| Go | 1.24.5 windows/amd64 |
| cc65 | `C:\Applications\cc65-snapshot-win32\bin\` (PATH 通過済み) |
| Ruby | 3.3.7 x64-mingw-ucrt（オラクル用。`ruby/` 以下に凍結） |
| MesenCE | 2.2.1 を `C:\Applications\MesenCE\Mesen.exe` に導入済み |

- **`bundle exec` は壊れている**（shim が ruby を見つけられない）。素の `ruby` / `rspec` を使う
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

- golden の再生成（feature/v2 以降）: **`go test ./internal/driver -run 'TestGolden|TestExample' -update`**。
  Go 自身の出力で上書きする（Ruby オラクルの `tools/gen_golden.rb` は凍結。形式は
  [go_port_dump_format.md](go_port_dump_format.md)）。**意図しない差分を `-update` で消さない**
  （運用ルールは [v2_plan.md](v2_plan.md) §0.1 G2）。ast golden は廃止済み
- golden は `.gitattributes` で `eol=lf` に固定してあり、`-update` 後に `git status` がクリーンなら
  出力が完全一致している
- **fc ソースのテスト（`test/test_*.fc` の assert 群）は TestGoldenStdout が
  コンパイル→実行して stdout・終了コードごと検証する**（assert 失敗 = exit 1 + ERROR 出力
  で必ず不一致になる）。テスト .fc を新規追加したら golden ディレクトリに空ファイルを置くか
  `-update` で生成する
- 性能退行の検知: `go test ./internal/driver -run xxx -bench BenchmarkCastle -benchmem`
  （基準値は v2_plan.md の作業ログ）
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
