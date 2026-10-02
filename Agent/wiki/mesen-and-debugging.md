# MesenCE・ソースレベルデバッグ・エディタ連携

MesenCE の導入とハマりどころ、`fcc build -g` でのデバッグ、`fcc watch` / `fcc check --json`、コードサイズの調べ方。配置指定とスタック表示は [placement-debugging.md](placement-debugging.md)。

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
   `emu.log` の出力は stdout には出ないが、**`print()` は stdout に出る**（途中経過はこちらで出す。
   `emu.log = function(s) print(s) end` と差し替えれば、生成した `.fclog.lua` の表示をそのまま取れる。`TestMesenLog`）
4. **セーブデータ（`.sav`）が残る**: バッテリーバックアップの SRAM は ROM のファイル名で `Saves/<名前>.sav` に
   書かれ、次に同じ名前の ROM を開くと読み込まれる。castle はセーブがあると「つづける」で最後のチェックポイントから
   始まるので、自動プレイの行き先が変わる（2026-09-26、エリア 66 で止まって TestMesenPlayCastle が落ちた）。テストは
   ROM を `fc_test_<名前>` に写し、その `.sav` を前後で消す（`mesenFreshRom`。手でプレイした `castle.sav` には触らない）
5. **`emu.getState()` は重い**（1 回 100 マイクロ秒ほど。PPU まで含む全状態の表を作る）。exec コールバックの中で
   毎回呼ぶと数倍〜10 倍遅くなるので、レジスタ・サイクル数が要るときだけ呼ぶ
6. **Mesen が起動中だと Mesen のテストは飛ぶ**（単一インスタンスなので testrunner が既存の窓に渡る。`ensureMesenSettings`）。
   手でプレイしている間は `go test ./...` が通っても Mesen の側は確かめていない。生成した Lua が Lua のエラーになると
   testrunner は何も出さずに止まらない（2026-09-28 の driver を分けたときの名前の置き換えが `.fclog.lua` の中の `site` を
   `Site` にしていて、TestMesenLog が 60 秒のタイムアウトで落ちていた。2026-09-29 に直し、`internal/fclog` の
   TestMesenLogScriptFields が Mesen なしで見張る）。Go の名前を一括で置き換えるときは、文字列に埋め込んだ Lua / asm を除く
7. Lua API 覚え書き: `emu.setInput(inputTable, port)`（inputPolled イベント内で呼ぶ。
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
`.mlb` は `fcc size` と同じ `driver.DbgFile.WriteMlb` で作れる（CLI は未提供。castle は `fcc build` に移ったので不要）。

### エディタ連携（`fcc watch`、`fcc check --json`、`tools/vscode-fc/`）

`fcc watch [-t nes] [-o FILE] [-g] [-c] main.fc` はソースディレクトリと fclib の下の `.fc` / `.asm` / `.inc` / `.chr` /
`.txt` / `.cfg` の更新時刻を 0.5 秒ごとに見て、変わったら再ビルドして結果を出す（外部ライブラリ無し）。
`fcc check --json` は診断を 1 行 1 JSON（`file` / `line` / `col` / `severity` / `message`）で出す。
`tools/vscode-fc/` はそれを使う VS Code 拡張（plain JS、ビルド不要。ハイライト + 保存時に Problems へ。
`fc.mainFile` にプログラムの起点を書けばどのファイルを保存しても全体を検査する）。言語サーバは持たない。

### コードサイズ（`fcc build --size-report` / `fcc size game.dbg`）

セグメントごとの合計と、関数（ラベル）ごとの大きさ（次のラベルまで。関数の後ろの定数表を含む）を大きい順に出す。
2026-10-02 から、リンカ設定の ROM の領域（`MEMORY` の `file` のあるもの。16 バイト以下のヘッダ・ベクタは除く）ごとの使用量・空き・
置いたセグメントの表（`cc65.DbgFile.BankReport`。`.dbg` の seg の大きさを、リンカ設定の `SEGMENTS` の `load` で領域に足し上げる。
自前の設定も読める: `fcc size -cfg`）と、`--size-report` ではモジュールの組ごとの呼び出しの数（`driver/sizereport.go`。最適化の後の
`OpCall` を数え、`Op.Far` を far として数える。ROM に出さない `Lambda.Unused` の関数は除く）も出す。バンクとモジュールの組み合わせを
考えるための表（ユーザーの依頼）。
