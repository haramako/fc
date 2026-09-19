# examples — 実プロジェクト由来の回帰テスト用サンプル

このコンパイラの実利用プロジェクトである **fc-miku** と **castle** から、
**コンパイルに必要なものだけ**（.fc / .asm / .chr / .nespal / フォント表 / リンカ設定 /
サウンド .o / NSD.lib 等）を取り込んだもの。

目的: **この2プロジェクトがビルドでき、ROM がスナップショットとバイト一致すれば、
コンパイラの動作が維持できている**とみなす回帰テスト。
（現時点でこのコンパイラで動いているプロジェクトはこの2つだけなので、これで十分）

## 構成

| ディレクトリ | 由来 | ビルド方法 |
|---|---|---|
| `miku/` | `C:\Work\fc-miku` | `fcc build -t nes miku.fc`（fc標準ドライバのみでROM生成） |
| `castle/` | `C:\Work\castle` | `cd src && fcc build -t nes -o ../castle.nes main.fc`（main.fc の `options(base / linker_config / link)` で自前の data.asm・ld65.cfg・NSD を指定。実プロジェクトの Rakefile はまだ `fcc compile` → `ca65` → `ld65` の手順） |

castle は `textmap` によるテキスト変換（`_T`/`_M`、表は `tmp/font/*.chr.txt`）、独自リンカ設定、
NSD サウンドドライバを含む、コンパイラ機能をほぼ全部通るサンプルになっている。

**examples/ の .fc は 2026-09-14 に `fcc migrate` で文法 v2 に移行した**（ROM はバイト一致）。
実プロジェクト側はまだ v1 のことがあるので、`sync_examples.ps1 -Update` で取り込む前に実プロジェクトを
v2 に移行するか、取り込んだ後に `fcc migrate` を掛け直すこと（castle は
`cd src && fcc migrate -t nes --textmap _T=../tmp/font/text.chr.txt --textmap _M=../tmp/font/misc_text.chr.txt -w main.fc`。
`src/macro.rb` は不要になったので削除済み。`title.fc` の `VERSION_STR()` は固定文字列 `_M("VERSION 0.5.0")` に変更済み）。

## テスト

```bash
go test ./internal/fc -run "TestExampleMiku|TestExampleCastle"
```

ビルド結果の ROM を `testdata/golden/examples/{miku,castle}.nes` とバイト比較する。
スナップショットは現行の fcc のビルド結果（`-update` で更新する）。

コード生成を意図的に変更したときは、差分を確認したうえでスナップショットを
新しいビルド結果で置き換えること。

## NES エミュレータでの動作スモークテスト

ROM がバイト一致するだけでなく「大雑把に動く」ことも自動確認できる:

```bash
go test ./internal/nes -v
```

`internal/nes` はスモークテスト用のヘッドレスNESランナー
（r6502 CPU + NROM/MMC3 マッパー + PPUレジスタ近似 + NMI/MMC3スキャンラインIRQ近似）。
判定は「Nフレーム実行してクラッシュしない・NMIが回る・描画が有効化される・
VRAMに画面が作られる」というゆるい基準。

`FC_NES_SNAPSHOT_DIR` 環境変数を設定すると、背景画面のスクリーンショットPNGを
保存する（castle はタイトルロゴ、miku は背景が描画されることを目視確認できる。
スプライト・スクロールは描画しない）。

`TestPlayCastle` はさらに実際にプレイする: タイトルで A 決定 → フィールドで
右移動+ジャンプ → 画面切り替えをスクリーンショット差分で検出する。

## MesenCE による高精度テスト (任意)

実機精度の検証には [MesenCE](https://github.com/nesdev-org/MesenCE) を使う:

```bash
go test ./internal/nes -run TestMesenPlayCastle -v
```

`TestMesenPlayCastle` は examples/castle をビルドし、ld65 マップから
`_my_x` / `_bg_cur_area` のアドレスを取り、自動プレイの Lua スクリプトを生成して
`Mesen --testrunner` (ヘッドレス) で実行、**エリア変数の変化2回**を成功条件として
終了コードで判定する。

導入: MesenCE の Windows zip を `C:\Applications\MesenCE` に展開する
（別の場所なら環境変数 `FC_MESEN` に Mesen.exe のパスを設定）。見つからなければ Skip。
初回セットアップの注意（テストが自動処理するものも含む）:

- ダウンロードした exe は `Unblock-File` が必要（SmartScreen のダイアログ待ちで
  無音のままハングする）
- `settings.json` が無いと初回起動ダイアログで止まる。さらに **`Nes.Port1.Type` に
  コントローラを設定しないと入力注入が効かない**（テストが無ければ自動生成する）
- testrunner の Lua では `io`/`os` は使えない（結果は emu.stop の終了コードで返す）

internal/nes のスモークの既知の制限: タイミングは概算のため精密な検証には
向かない（そちらは MesenCE 側で行う）。起動と進行の高速な常時確認が目的。

## 生成物リソースの方針（fs_data.bin 等）

`castle/res/fs_data.bin`・`castle/tmp/*.bin`・`castle/tmp/font/*`（フォント表）は
実プロジェクトのツールチェーン（map.json / xlsx / テキスト → castle 側の変換ツール）が
生成するもの。再生成には castle 側のツール一式が必要なため、
**examples では出来合いの生成物をスナップショットとして許容する**。

- ソース内文字列（`_T(...)` 等）の変更には追従しない。文字列とフォント表・
  fs_data.bin の整合性は実プロジェクト側の責務
- 実プロジェクトで再生成されたら `sync_examples.ps1 -Update` で持ってくるだけ

## 実プロジェクトとの同期・差分確認

取り込んだファイルが実プロジェクトからずれていないか（またはどうずれているか）は
次で確認できる:

```powershell
.\tools\sync_examples.ps1              # 差分表示 (git diff --no-index)
.\tools\sync_examples.ps1 -Update      # 実プロジェクト → examples/ へ再取り込み
```

- 対象は「examples/ に存在するファイル」。実プロジェクト側の新規ファイルを
  取り込みたい場合は、まず examples/ にファイルを置いてから `-Update` する
- **サンプル側を意図的に変更した場合**（コンパイラ都合で実プロジェクトのソースを
  直したほうが良いケース）は、差分がこのスクリプトで見えるので、
  実プロジェクトへ還元するかどうかをそこで判断する

## 取り込み時の変更点

**現時点でどちらのプロジェクトも無修正**（実プロジェクトのファイルをそのままコピー）。
変更が発生したら、この節に「どのファイルを・なぜ・どう変えたか」を記録すること。
