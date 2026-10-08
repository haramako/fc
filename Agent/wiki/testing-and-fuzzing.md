# テストと fuzz（層・回し方・失敗の調べ方）

Windows でパスを検査するときは `filepath.Join` / `filepath.FromSlash` などで期待値を合わせる。
埋め込み HTML は checkout の CRLF を保持するため、生成結果の検査で LF 固定の正規表現を使わない。
2026-10-03 に `TestMacroServers`・`TestStarlarkMacros`・`TestSizeHTML` の失敗を Windows で再現して確認した。
実行ログと再現コマンドは [Actions の失敗調査](../discussions/2026-10-03-actions-failures.md)。

`go test ./...` の中身、差分 fuzz / Go native fuzz / バンク切替 fuzz の回し方、失敗した種と実プロジェクトの退行の調べ方、ベンチ。fuzz で見つかったバグの時系列は [../discussions/2026-09-19-fuzz-findings-log.md](../discussions/2026-09-19-fuzz-findings-log.md)。

```bash
go test ./...                                    # 全部 (golden + examples + NESスモーク)
```

| 層 | テスト | 時間 | 何を保証するか |
|---|---|---|---|
| golden差分 | TestGolden*（internal/driver） | 数秒 | コンパイラ出力の全段階が基準と一致（stdout は `-O 0` でも同じ: TestGoldenStdoutO0） |
| ROMバイト一致 | TestExampleMiku / TestExampleCastle | 〜5秒 | 実プロジェクト2つのROMがスナップショットと一致 |
| 内蔵スモーク | TestSmoke* / TestPlayCastle（internal/nes） | 〜1秒 | 起動・NMI/IRQ・描画・自動プレイでの画面遷移 |
| NES の fclib | TestNes* / TestExampleHello（internal/nes） | 〜1秒 | frame / vram / pal / oam / pad の画面・OAM・パッド、乱数の put の並びと Go の模型、NMI が vblank に収まる（`Stats.LateVramWrites` / `MaxVblankUse`） |
| 実機精度 | TestMesenPlayCastle | 〜10秒 | MesenCE 上での自動プレイ（エリア変数で判定） |
| 差分テスト | TestRandomPrograms（internal/driver） | 〜20秒 | ランダムな小プログラムを -O 0 / -O 2 で走らせて出力が一致 |
| 畳み込みの差分 | TestRandomConstFold（internal/driver） | 〜5秒 | 同じ式を型付きの定数と変数の 2 通りに書いて結果が一致（sema の誤りを見る） |
| 通ってはいけない | TestMustError / TestMustWarn（internal/driver） | 〜1秒 | エラー・警告になるべきプログラムの表（検査が緩む退行） |

- **カバレッジ**（2026-09-28）: `go test ./... -coverpkg=./... -coverprofile=cover.out` で、どのテストが通したかを問わず
  全パッケージの通過を取る（パッケージごとの表示の % は「そのテストがモジュール全体の何 % を通ったか」なので、block ごとに
  合わせて集計し直す）。普段の `go test`（ランダムなプログラムは既定の本数）でコンパイラ本体は 90〜96%、全体 90.7%。
  通っていないのは主にエラーの文言（sema 約 100 文、driver 約 70 文）、起きないはずの panic、調査用のトレース、CLI
  （`fcc watch` / `fcc migrate`）。この調査で、どこからも呼ばれない関数（`attachScope` / `GetWord` / `NewAllocator` /
  `hasCalls` / `Summary`）と、通らない `loadA` の条件フラグの変換（Ruby 版の名残。今は内部エラーの panic）を消し、
  テストの無かった常駐レジスタの自己修正（`TestResidentSelfCorrection`）とスタックの規約の関数のローカル配列
  （`TestStackAbiLocalArray`）のテストを足した。`load` の配列 → ポインタの変換（配列は sema が PointeredArray に包むので
  通らない見込み）は、最適化の途中で作られる可能性を否定しきれないので残している

- **fuzz の効果の測定**（2026-09-28）: 生成器の機能と判定を足すだけでなく、効き目を測って「普段の `go test` に入れる / 夜間だけ /
  消す」を決めるための仕組み。
  - 機能の名前（internal/driver/randfeat_test.go の `rpFeatures`）: 生成器は機能を選ぶ所で `g.want(名前, 確率)` を呼ぶ。
    `-fuzzoff a,b` / `-fuzzonly a,b` で入れ切りでき、乱数は切っても同じだけ使う（全部入りなら種の番号は同じプログラム）。
    使った機能は失敗の報告（「使った機能:」）と `FUZZ_STATS=ファイル`（1 本 1 行の JSON。夜間の CI が artifact に残す）に出る
  - bugzoo（testdata/bugzoo/*.patch）: 直したバグをわざと戻すパッチ。先頭の `#` の行が説明。`go run ./tools/fuzzmeasure zoo`
    が 1 つずつ HEAD の一時的な作業ツリー（git worktree。コミットしていない変更は入らない）に当ててテストを回し、テストごとに
    失敗した数と、失敗した種で使っていた機能を表にする（`none` はパッチ無しで、誤検出が 0 のはず）。`-off 機能` で機能を切って
    回すと、その機能が無いと見つからないバグが分かる。夜間の CI（fuzz.yml）が日曜に回して job の summary に出す。
    **バグを直したら、そのバグを戻すパッチを足す**（`git show -R <commit> -- <直したファイル> > testdata/bugzoo/名前.patch`、
    先頭に `# 説明` の行。`git apply --check` で当たることを確かめる）
  - `go run ./tools/fuzzmeasure cover`: 全部の機能と、機能を 1 つずつ切ったときのコンパイラ（internal/...）の通過を比べ、
    その機能を切ると通らなくなる block の数を表にする（その機能だけが通す経路）
  - 判断の目安: bugzoo で 1 つも見つけず、独自の block もほとんど無い → 消す候補。効くが重い → 夜間だけ。
  - **2026-09-30 の点検で分かったこと**: (1) コードの書き換えで bugzoo の 22 個中 16 個が当たらなくなっていて、zoo はそのバグを
    測っていなかった（作り直し、`TestBugzooPatchesApply` で当たることを見張る）。(2) `TestRandomConstFoldV4` が @printf への改名の後
    何も比べずに通っていた（変数の形で全部の式が捨てられた。捨てた式の割合の見張りを足した）。**生成したソースの書き方が言語の
    変更に追いつかないと、差分テストは黙って空になる**ので、fc 4 の変更の後は「何本を比べたか」も見る。(3) テストごとのカバレッジは、
    TestRandom* 以外のテストで 91.7 %、TestRandom* を全部足しても +0.6 % で、ランダムテストの効き目は新しい行でなく組み合わせ。
    最適化の規則は同じ項の繰り返し・定数の連鎖で初めて動くので、項を毎回ランダムに選ぶ生成器では出ない（ssa-stale-useat は 250 万本に
    1 回）。bugzoo のうちランダムテストで見つからない実行結果のバグから、生成器の機能を足した: `reuse`（打ち消し合う同じ項の連鎖）、
    `bigarr`（要素 2 バイトの 200 要素の配列）、regpress の内側のループ、v3m の自己検査の enum の呼び出しの回数、randfold の型の
    ない大きな定数との比較。どれも 0 本 → 数 % で見つかる。sema の誤り（-O 0 / -O 2 / インタプリタが同じように間違える）は差分では
    見えないので、自己検査か randfold の形にする。エラーにすべきものを通すバグ（case-fit など）は差分テストの守備範囲外で、
    TestMustError などの表が受け持つ。fc 4 の文字列は `TestRandomStringsV4`（randstr_test.go。fc 4 のソースを直接作り、期待値を生成器が
    計算する。綴りを毎回選ぶのでエスケープも見る）で、作った直後に 3 件（途中の 0 のポインタ・大きさ 0 どうしの ==・インタプリタの
    配列の ==）と、作る前の確認で 1 件（128 以上のバイトで [N]i8）見つかった
    -O 0 / -O 2 / インタプリタの判定、最小化、定数の畳み込みの差分テスト、自己検査の仕組みは必須なので測らない
  - **機能・最適化を足すときの決まり**: 機能を足すのと同じ作業で生成器に形を足し（名前を付けて rpFeatures に）、コミットの前に
    `-randn 1000` で回す。直したバグは bugzoo に足す。新しい機能と既存の最適化の組み合わせのバグは足した直後に出やすい
    （2026-09-27 の struct の配列の最適化 5b89873 と v3 の生成器の形の組み合わせで、翌日に 3cba778 が出た）
  - 足した判定（2026-09-28）: `TestRandomMetamorphic`（fcc の実行ファイルで、普通 / インライン展開を全部切る / 最適化の段を
    1〜3 個切る、の出力を比べる。切る段は BuildOptions.Config (ir.Config) でビルドごとに渡す。既定 4 本、`-metan`。
    段を切った版がサイクルの上限に掛かったら上限を 50 倍にして走らせ直す: ssa を切ると 40 倍遅くなるプログラムがある）、
    `TestRandomMutate`（生成した正しいプログラムを行・名前・型・数・キャストの単位で壊して Check に通し、panic だけを失敗にする。
    既定 10 本 × 20 通り、`-mutn`）、`TestTypeRuleMatrix`（型の種類 × 使う場所の「通る / エラー」の表を
    testdata/golden/typerules.txt と比べる。検査が緩む・厳しくなる変化が差分で見える）、生成器の `regpress`（常駐レジスタに
    負荷をかけるループ: 1 バイトのカウンタを 2 バイトの要素に書く、2 バイトの減算の隣の 1 バイトの常駐、呼び出しの直後の
    符号付きの比較、常駐の添字と融合）
  - 生成器に 5 つ足した（2026-09-29。main に入った言語の変更で生成器が出していなかったもの）: `shift16`（2 バイトの値の変数の
    回数のシフト。量は `& 15`、左は `as` で 2 バイトに固定）、`arrlit`（実行時の値を要素に持つ配列リテラルで初期化するローカル
    配列。要素は `as` で包む: 定数だけの式は型のない定数になり、要素の型が値から決まって宣言と合わない）、`structeq`（S と T の
    値の `==` / `!=`）、`defargs`（末尾の整数の引数に既定値、直接の呼び出しでときどき省く）、`shadow`（素のブロックで外の変数と
    同じ名前・型の変数）。足す前の確認で、インタプリタが配列の値の load（初期値つきのローカル配列）を番地のコピーにしていたのを
    直した（94ab120）。スキップ（ROM に入らない等）は 200 種で 13.5% → 16%（`arrlit` が一番効くので確率を低めに）
  - fuzz が N100 機（16GB）のメモリを使い切って OOM killer がセッションごと落とした原因は、TestRandomMutate が壊して作った
    `g0 << 99999999`（1 バイトの値の、型の幅を大きく超える定数の回数のシフト）: codegen の genShift と regalloc の
    ShiftMemForm が 1 ビットずつ n 回の命令を並べていた（100 万で 200MB、1 億でメモリを食い尽くす）。回数を型の幅で
    頭打ちに（`TestShiftCountBeyondWidth`）。fuzz の実行は差分 fuzz・FuzzCheck ともメモリの上限つき（systemd-run --scope）に
  - 常駐レジスタの自己修正（`ResidentFixes`）を「A のまま使う」(ResFriendly) 命令にも広げる案は、その命令が常駐の値を A に
    読み直すのも「A を書いた」と数えて収束しなかったので入れていない（2026-09-28）
- **デバッグ用のゲーム**（2026-10-08、`games/`・`internal/nes/games_test.go`）: 11 本のゲームを `AUTO` で遊ばせ、進み具合（得点・
  面・倒した敵）を変数で確かめる。`TestGamesSameAtTick` はメインループの手数（`end_frame` に `ticks == k` で入った所。
  `nes.Machine.Stop` で命令ごとに止める）で -O 0 と -O 2 の main の変数（.map の `_main.o` の BSS）を比べる。AUTO の考える時間で
  進むフレーム数が違っても、ゲームの論理は手数ごとに決まるので一致するはず。bugzoo の最適化のバグ 5 つは見つけなかった（fuzz の
  作る特別な形）。panic で止まると `Machine.Output` の文言で落ちる。内蔵のランナーに足したもの: スプライト 0 の当たりの近似
  （BG との重なりは見ない）、CHR RAM。見つかったことは [../discussions/2026-10-08-debug-games-findings.md](../discussions/2026-10-08-debug-games-findings.md)

- **fc 4 の機能の生成器**（2026-10-08）: 期待値を生成器が Go で計算する形（sema の誤りも見える）で、-O 0 / -O 2 / インタプリタと比べる。
  `TestRandomMethodsV4`（randmethod_test.go。struct の値・`*P`・`*const P`・soa のハンドルのメソッド、P を返すメソッド、受け取り手は
  変数・配列の要素・ローカル変数・ポインタ・soa の要素・関数の戻り値・`P.m(x, …)`。soa / 普通の interface、2 つのモジュールの実装、
  手動・自動の ID、既定の本体、`@set_id` と init・.none・実装の無い ID、`@id_of`、`@bitcast` の実装のハンドル、別の要素のメソッドと
  深さを限った再帰。式は符号なしの環の演算だけにして、計算の幅の規則によらず期待値が決まるようにした。`-randn` の 1/4 本）、
  `TestRandomFormatV4`（randformat_test.go。@printf / @format / @try_format の書式と、整数・bool・enum・文字列の引数。同じ値を定数
  （sema の formatConst）と実行時の値（fclib/fmt.fc）の両方で書く。1/4 本）、`TestRandomFarIfaceNES`（randfariface_test.go。
  nes ターゲットの UxROM で、同じ番地に違う表を持つ 2 つのバンクに実装を置き、`@(far)` の振り分けと、near の `@bank_of_id` での
  切り替えを -O 0 / -O 2 で。1/2 本）。足した直後に `@bank_of_id` のバグを 1 件（最初のメソッドを書かない実装が固定の所のバンクに
  なる。bugzoo の bank-of-id-default）。わざと戻したバグで効くことを確かめた: 自動の ID を 2 から振る → TestRandomMethodsV4 が 100 本中
  24 本、0 埋めの符号の位置（formatConst）→ TestRandomFormatV4 が 14 本、fmt.fc の小文字の 16 進 → 78 本、`@clamp` の畳み込みを
  `max(min(x, hi), lo)` に → TestRandomConstFold が落ちる。同じ日に、TestRandomConstFold に `@clamp`、rpCondRewrite（fc 4 の差分）に
  `while (C) { S }` → `loop { if (!(C)) { break; } S }` を足した（100 本で 63 か所）。3 つとも fuzzmeasure zoo の既定の `-run` に入れた
- **2 次元の配列とループの生成器**（2026-10-08）: `TestRandomGridV4`（randgrid_test.go。期待値は Go で計算。`-randn` の 1/4 本）。
  opt の 2 次元の添字（fieldindex）・licm・strength を狙う（[design/ssa.md](design/ssa.md) §12）。256 バイト以内 / 超える u8 と
  u16 の要素の配列、増える（歩幅 1〜3）・減る・while のループの 1〜3 重、添字は ループの変数 ± 1・定数・ループの前の変数・手で
  書いた `y * C + x`、continue / break、本体の途中でループの変数を進める（誘導変数でなくなる）。わざと壊した 3 つの段を 80〜100 本
  見つけた。fuzzmeasure zoo の既定の `-run` に入れた

- **差分テストの判定の弱点と、足した検査**（2026-09-27）: TestRandomPrograms / TestRandomV3Programs の判定は -O 0・-O 2・
  最適化前の IR のインタプリタの 3 つで、どれも sema の作った同じ IR を実行するので **sema の誤りは 3 つとも同じように
  間違えて見えない**（型付きの定数の畳み込み、i8 の初期値の符号拡張、ポインタの負のずれの符号拡張、for-each の回数など。
  2 回目の調査で人手で見つかったもの）。そこで次を足した:
  - `TestRandomConstFold`（randfold_test.go）: 同じ式を型付きの定数 `(200 as u8)` と noinline の関数 `id_u8(200)` で書いて
    比べる（畳み込みと実行時の計算の食い違い。最初の実行で畳み込みの型の食い違いが 4 種類見つかった）。狭い型の値で広い型の
    変数を暗黙に初期化する形（出どころは式・大域変数・配列の要素・struct のフィールド）も比べる。既定 10 本、`-foldn 500`
  - v3m の自己検査（randv3check_test.go）: 同じことを 2 通りに計算して違えば `stdio.exit(77)`（rpCheck が `selfcheck` にする）
  - 生成器: 初期値なしの変数とループ・if / else での代入、暗黙の拡張、200 要素の u16 の配列（k - 128 の重なりも読む）、
    ポインタの負のずれ、fc 3 の for-each・範囲・case の範囲・`..=`、struct の配列のフィールド、const の表の無名関数
  - インタプリタ: `@null_fn` と asm の `mem.copy` / `set` / `zero` を実行する（v3 のプログラムの約半分で判定を飛ばしていた）
  足した形が実際にバグを捕まえるかは、直したバグを一時的に戻して確かめた（SSA の φ: 200 本中 2 本、i8 の初期値: 30 本で 39 か所、
  2 バイトの要素の添字: 60 本中 1 本、負のずれ: 60 本中 5 本、文字列リテラル: 60 本中 6 本）。
  夜間の CI（.github/workflows/fuzz.yml）で種を日替わりにして 1500 本 + 畳み込み 500 本、go の fuzz（FuzzParse / FuzzFormat /
  FuzzCheck。fc 3 の新しい文法の種を足した）を回す。`rpRun` は ca65 / ld65 が何も出さずに失敗したとき（並列で重いときの
  Windows の一時的な失敗）だけやり直す

- **ランダムプログラムの差分テスト**（`internal/driver/randprog_test.go`、2026-09-19〜）: Csmith と同じ考え方で、
  4 つの整数型・配列・関数（fastcall / inline）・if / for / while / switch・全演算子を混ぜた小さなプログラムを生成し、
  `-O 0` と `-O 2` の emu の出力を比べる（片方だけ panic / 止まらないのも検出）。既定は種 1〜30 で毎回同じ。
  数を増やすには `go test ./internal/driver -run TestRandomPrograms -randn 1000 -randseed 5000 -timeout 60m`（1000 本で
  数分。2000 本は `go test` の既定の 10 分を超えるので `-timeout` が要る）。生成物は `t.TempDir()` (= `%TMP%`) に
  1 本ごとに作って消すので、Windows Defender が重いときは `$env:TMP` を除外済みの専用フォルダ (`C:\Work\gotmp` など) に
  して走らせる。サイクル数の上限は 20M で、`-O 0` だけが掛かったときは 10 倍で走らせ直す（far call と除算の入れ子で
  `-O 0` が `-O 2` の 4 倍 (21M 対 5.7M) になった種があり、「片方だけ止まらない」の擬陽性で最小化に 4〜7 分かかっていた）。
  食い違いは文の木を消して最小化してログに出す。式の中まで縮めるには `tools/reduce_fc.py prog.fc fcc.exe [panic]` の
  「括弧の部分式を定数に置き換える」雑な delta debugging と、`FC_DISABLE` でのパスの切り分けを併用する。
  **初日に 7 件見つかった**（`TestFuzzFound1` / `TestFuzzFound2` に固定）: cast を挟んだ `!` のコンディション、
  融合した index_pset の書く幅、splitWords の cast の上位バイト、dead store と live range（無限ループ）、
  `l ^ l` の A 割付（-O 0）、入れ子の cast の codegen の byte、関数全体の常駐の退避と内側のループの写しの順序。
  単機能のテストは全部通っていたので、**組み合わせのバグはこれで探す**。未定義動作は生成しない（0 除算・範囲外の添字・
  2 バイト値の変数シフト・無限ループ・ループ変数への代入）。判定は emu なのでフレームの位相のような擬陽性は無い。
  生成器の制限で「型エラー」になる形が出たら生成器の問題（`ビルド失敗 (生成器の問題)`）として直す。
  **2026-09-20 に生成器を広げた**: 16 要素のローカル配列、配列へのポインタ（`&a[e & 7]` から始めて `*p` / `p[e & 7]`、
  ポインタの引数）、ポインタをずらすループ（`for` で `p += 1`、`while (k < n)` で `q += s; k += s`。誘導変数の統合と
  展開の形。添字が範囲を出ないように本体の前で進める）、減らす `for`、struct（グローバル `s0`、配列 `sa[4]`、
  ポインタ `ps` 経由のフィールド）。初回の 30 本で 3 件: SSA の simplify が消した `load x = x` を参照して panic、
  要素 2 バイトのポインタ参照の `iny` で Y に常駐する添字がずれる、フレームがゼロページでない関数でポインタを reg に
  写すときに A の添字を壊す（`TestPointerIndexY`）。次の 1000 本で 2 件: fusePointer が struct 配列の先頭フィールドへの
  書き込みを index_pset（書く幅 = 要素）にして隣のフィールドを壊す、splitWords が 1 バイトのフィールドを広げた cast の
  下位を struct の 0 バイト目として読む（`TestStructFieldWidths`）。さらに次の 1000 本で 2 件: splitWords が 2 バイトの
  フィールド `<int16+1>s0` の k バイト目を作るとき cast のオフセットを二重に足す、符号付き 1 バイトの変数シフトが
  左でも `cmp #128; rol` で符号を回し込む（`-109 << 1` が 39。`TestShiftVarSigned`。-O 0 でも同じ結果なので差分では
  出ず、SSA の定数畳み込みと食い違って発覚）。その次の 1000 本で 2 件: regalloc が cast を挟んだ一時変数の `if`
  （`if ((!x) as int16)`）を「コンディションの一時変数」と見て A を壊さない扱いにしていた（`TestResidentIfCast`）、
  SSA の定数の読み出しが cast の連鎖の内側の切り詰めを無視していた（`((l0 as int) as sint16)`。`castBits`）。
  次の 1000 本で 1 件: `if (A || 定数)` の片側が畳まれて残った `if_true c goto next`（飛び先 = 落ちる先 = ループの入口）で、
  常駐の入口の写しが落ちる辺にしか付かなかった（simplifyJumps が直後への条件分岐を消し、`onEdge` は両方の辺に置く。
  `TestResidentEntryBothEdges`）

- **長時間 fuzz（2026-09-20〜、種 310000〜）**: 2000 本 × 30 のかたまりで回す（1 かたまり約 2 時間。`scratchpad/fuzz/`
  にログ）。最初の 6 万本で 20 件失敗、実バグ 6 件: `cpx tab+0,y` / `cpy S+k,x`（無いアドレッシングモード。
  `TestCompareOperandModes`）、A 常駐があるときの Y 代用と融合の衝突、2 バイトの一時変数の if を freeA が
  コンディション扱い（`TestResidentIf16`）、内側ループの出口の辺で退避と復帰の順序が逆（`TestResidentExitEdgeOrder`）、
  call の戻り値 (A) の if の前の常駐復帰 `ldy` がフラグを壊す（`TestIfCallResultFlags`）。残りは**プログラム側**:
  (1) ソフトウェアスタックのあふれ（表経由で再帰する関数がローカル配列 32 バイトを持つと -O 0 でフレームが重なり
  ゼロページを壊す。pc=$ffff / 定数表の中で invalid opcode）→ emu ターゲットは zp,X / zp,Y のページ越えを検出して
  panic（`r6502.Cpu.TrapZpWrap`）、runner は「ソフトウェアスタックがあふれた」で飛ばす、生成器は表の関数にローカル配列を
  持たせない（`RP_TABLE_ARRAYS=1` で以前どおり）。(2) 最小化がローカル配列の初期化文を消して未初期化の読み出しを作る
  擬陽性 → 最小化と `tools/reduce_fc.py` は `laN[k] = …` を残す（**最小化したプログラムで値が違っても、初期化が消えて
  いないか先に見る**）。(3) -O 0 が 200M サイクルでも足りない重い種（再試行を 50 倍に）。**調べ方**: 失敗した種は
  `go test -run TestRandomPrograms -randn 1 -randseed N`（旧生成器なら `RP_TABLE_ARRAYS=1`）で最小化し直し、
  ログから `t.fc` / `far1.fc` を切り出して `fcc run` / `fcc run -O 0`、`FC_DISABLE=パス` で切り分け、
  `FC_DUMP_IR=1` で IR、invalid opcode なら `FC_TRACE_PC=1` で直前の PC（`.dbg` の `sym … val=` で関数に当てる）

- **バンク切替の fuzz**（`internal/driver/bank_test.go`、2026-09-21）: emu にはバンクが無いので、切替バンクをまたぐ
  インライン化のバグ（表は元のバンクに残ってコードだけ移る）は既存の fuzz では見えなかった。nes ターゲット（MMC3、
  `bank_count: 8` でバンク 0 と 4 が同じ $8000 に来る）の小さなプログラムを生成して内蔵 NES ランナーで走らせ、生成器が
  計算した期待値と `-O 0` / `-O 2` を比べる（`TestRandomBankPrograms`。既定 8 本、`-randn 800` で 200 本 30 秒。
  トランポリンは `fclib/nes/farcall_mmc3.asm` を `include` し、`_mmc3_pbank_bak` を `symbol:` の var で持つ）。
  呼び出しの後で `pbank_bak` も見る（バンクの復帰）。ルールを切ると 8 本中 3 本が落ちることを確認済み。2026-10-05: 引数
  2 つの関数（far call のレジスタ渡し: Y と A）と再帰する関数（stack の呼び先: X = FC_SP で入る）を足した（トランポリンの
  slot 0 の切替の経路で Y を壊すと 100 本中 20 本が落ちる）

- **IR インタプリタを 3 つ目の判定に**（2026-09-23、`internal/interp`）: -O 0 と -O 2 の差分だけでは、両方のレベルで同じように
  間違える codegen のバグ（符号付きの変数シフトなど）が見えない。sema の直後の IR を 6502 を介さずに実行し、emu の出力と
  比べる（`rpCheck` の失敗の種類 `interp`。`-randinterp=false` で切る）。各命令の意味は codegen の出すコードに合わせる
  （オペランドの k バイト目は codegen の byte と同じ規則、加減算は Dst の幅、比較は入力の大きい方の幅、乗除算は Dst の
  幅と符号の床除算）。stdio は FC のまま実行し、emu と同じ $FFFE / $FFFF への書き込みで出力・終了する。遅くならない
  （1000 本で 20 秒のまま）。**失敗の切り分け**（`rpLocate`）: 最小化した後、sema の IR にインライン展開 → opt の各段
  （`opt.Passes`。全関数に 1 段ずつ）を当てては実行し直し、最初に出力が変わった段をログに出す（最後まで変わらなければ
  レジスタ割付か codegen）。`FC_DISABLE` での手の切り分けが要らなくなる。わざと commute を壊すと影響した 96 本すべてで
  「opt の commute」と出ることを確認済み

- **Go native fuzz（コンパイラが落ちないこと）の導入**（2026-09-25）: 差分 fuzz（上）は正しいプログラムの実行結果を
  比べるもので、壊れた入力での panic は見ない。`internal/syntax/fuzz_test.go`（FuzzParse / FuzzFormat: 整形結果が
  再パースできて冪等）と `internal/driver/fuzz_test.go`（FuzzCheck: 構文〜コード生成、ファイルは書かない）を追加。
  `go test ./internal/driver -fuzz FuzzCheck -fuzztime 60s` のように回す。見つけた入力は `testdata/fuzz/` に保存され、
  以後は通常の `go test` で回帰テストになる。最初の数分で sema の nil 参照 2 件: 空の配列リテラル（`const A=[];`）で
  要素型が nil のまま `ArrayOf` に渡る（cArray で要素が無ければエラーに）、トップレベルの裸のブロックの中の実行文が
  現在の関数（`h.lmd`）無しで `emit` に届く（emit でモジュールレベルならエラーに）。
  速度の目安: N100（4 コア）で FuzzParse 約 2 万実行/秒、TestRandomPrograms 約 5 本/秒（200 本 41 秒。
  16 コア機の記録 6 万本 ≈ 2 時間 ≈ 8 本/秒の 6 割ほど）
  続けて FuzzCheck 3 ワーカー + FuzzFormat 1 ワーカーで回して 5 件（どれも数分以内）: トップレベルのブロックの中の
  `return`（emit より前に `h.lmd` を読む。`requireFunction` を切り出して return でも呼ぶ）、呼び出しの結果への代入
  （`f() = 0` / `f()++` が void なら nil 参照、値を返す関数なら一時変数への代入として**黙って通っていた**。代入の左辺が
  呼び出しならエラーに）、宣言がエラーの soa の添字（`soaIndex` が `soa.Base` を直に読んで nil。`soaElement` を通す）、
  void の呼び出しの `f().x` / `*f()` / `&f()`（値が要る lval は `lvalValue` で nil を弾く）、`//\r` の整形が冪等でない
  （行コメント末尾の単独の CR が出力の改行と CRLF になる。行コメントの末尾の CR を落とす）。
  続けて定数同士の 0 除算（`1/0`、`[0%0]`）が畳み込み（`foldIntOp`）で Go の panic に（"div by 0" のエラーに）。
  エラーの上限（30 件）ちょうどが関数本体の外側の回復点（`recoverTo`）で記録されると、`report` の投げる
  "too many errors" が defer の中から抜けて落ちた（`record` を分けて、最外の回復点では投げずに記録だけ）。
  FuzzFormat が 1 時間 20 分でハング: 後置の型（v1 の `A****`、`int[1][1]`）の入れ子で `PointerType.Pos` /
  `ArrayType.Pos` が `IsPrefix` と自分とで `Elem.Pos()` を 2 回ずつ呼び、段数の指数時間（`*` 25 個で 2.5 秒）。
  続けて FuzzFormat が 2 時間 34 分で文字列の中の `\r\r\n` の整形が冪等でないのを発見（Format の CRLF → LF が 1 回の
  置き換えで `\r\n` が残る。改行の直前の CR をまとめて落とす `normalizeNewlines` に）。
  続けて 5 時間は失敗なし（FuzzCheck 6240 万回、FuzzFormat 1.6 億回）。その次に FuzzFormat が 3 時間 12 分で `a. .B`
  （`.` の右辺が型名を省いた enum）が `a..B` に整形されて `..` になるのを発見（printer の `mergesWithPrev` に `..` が
  抜けていた。字句解析の 2 文字の記号と揃える）。
  FuzzCheck（2 ワーカーに減らした。4 ワーカーでメモリ不足で止められたことがある）が 1 時間 22 分で、宣言がエラーの soa を
  既定値つきの引数の型にすると落ちるのを発見（`fitArrayLiteral` が Kind == Array の soa の `Base`（nil）を読んだ。soa を除く）。
  続けて 4 時間 24 分で `options(symbol: "")` の外部 const を読むと codegen が落ちるのを発見（シンボル名を検査して
  いなかった。空白入り・数字始まりも壊れた asm になる。`symbolOption` で識別子か検査する）。
  続けて 1 時間 38 分で、宣言より前に評価される式（`function A():t {}` から引かれた `{ var t = Es[0]; }`）が登録前の soa に
  届いて落ちるのを発見（`soaElement` は要素の型だけ先に決める。`soaOf` で宣言をその場で解決する）。
  注意: `go test -fuzz` を kill してもテストバイナリ（コーディネータとワーカー）が残って回り続ける。止めるときは
  `setsid` で起動してプロセスグループごと kill する

- golden の再生成（feature/v2 以降）: **`go test ./internal/driver -run 'TestGolden|TestExample' -update`**。
  Go 自身の出力で上書きする（形式は [Agent/wiki/golden-dump-format.md](golden-dump-format.md)）。**意図しない差分を `-update` で消さない**
  （`-update` の前に差分を読んで、意図した変化だけを受け入れる）
- golden は `.gitattributes` で `eol=lf` に固定してあり、`-update` 後に `git status` がクリーンなら
  出力が完全一致している
- **fc ソースのテスト（`test/test_*.fc` の assert 群）は TestGoldenStdout が
  コンパイル→実行して stdout・終了コードごと検証する**（assert 失敗 = exit 1 + ERROR 出力
  で必ず不一致になる）。テスト .fc を新規追加したら golden ディレクトリに空ファイルを置くか
  `-update` で生成する。`-O 2` は SSA の定数伝播で演算をほとんど畳んでしまう（test_op はほぼ消える）ので、
  同じプログラムを `-O 0` でも走らせる `TestGoldenStdoutO0` が codegen を検証する（golden は同じファイル）
- 性能退行の検知 (コンパイラ自身の速度): `go test ./internal/driver -run xxx -bench BenchmarkCastle -benchmem`

- **実プロジェクトの退行の切り分け**（2026-09-19）: `FC_DISABLE=名前,名前,...` で最適化のパスを個別に切れる
  （`ir.Config.Disabled`。名前は `internal/ir/config.go`: ssa mul indexoff induction unroll devirt autoinline sink fuse coalesce chain narrow scale commute carry split rotate dup
  inline resident func-resident step shift8 fuse-index fnptr-reg fieldindex ywalk switch peephole。ビルドごとに `BuildOptions.Config` でも渡せる）。`internal/nes/probe_test.go` は環境変数が
  無ければ Skip する調査用テストで、`TestProbeDiff` が 2 つの ROM（`FC_PROBE_ROM_A` / `_B`、`FC_PROBE_DBG` / `_B` の
  dbgfile で名前→番地）を同じ入力で並走させ、両方が vsync 待ちに入ったフレームだけゲームの状態（`FC_PROBE_PREFIX=_my_,_en_,...`
  の変数）を比べて最初に食い違う番地を出す。配置が同じ（同じソースでパスだけ切った）ROM 同士なら食い違いを A に合わせて続けられる。
  **注意**: (1) 旧コンパイラの ROM と比べると PPU の転送バッファや `_pad_*` はフレームの位相（ロード中の 1 フレームのずれ）で
  正当に違うので、ゲームの状態の変数に絞る、(2) 配置が違う ROM 間では RAM を写せない（ポインタの値が違う）、
  (3) 内蔵ランナーの idle 検出（`lda; bne` の形）は旧コンパイラの待ちループには効かないので、`Machine.FrameWaited`
  （そのフレームでフラグが非 0 のまま読まれた）で「待ちに入った」を見る。今回の梯子バグはこの並走では見つからず
  （パスを切っても同じバグが残る）、生成 asm を症状の行（`my_process.fc:93`）で読んで見つけた。**症状の行の `.s` を読む**のが早い。
  2 件目（踏むスイッチが効かない）も同じ手順: 症状の関数 `en1.switch_process` の `.s` を読むと `cmp; ldx; bne` で、
  比較の直後に挟まった常駐レジスタの復帰 `ldx` が Z を消していた（`on_idx == i` は i == 0 のときだけ動く）。
  **比較の結果がフラグ（N / Z）のときの復帰は `php` / `plp` で挟む**（`ir.CondRestoreNeedsFlags`、C はロードで変わらないので
  符号なしの `<` は挟まない。regalloc の gainOf も 7 サイクル引く）。可換な `==` は第 2 入力がレジスタでも `cpx` / `cpy` / `cmp`
  1 命令にして復帰自体を無くした。`TestResidentCondRestore` が番。ついでに r6502 の `plp` が S を戻していなかった（`php` /
  `plp` を出すコードが今まで無かった）。任意のエリアから始めるには `internal/nes/probe_test.go` の `TestProbeSwitch` のように
  チェックポイント 0 の `[area, x, y]` を ROM 上で書き換える（fs の敵データは `res/fs_data.bin`。`ENEMY_BASE + area` の
  ファイルの 7 バイト目から `[type, x, y, p1, p2, p3, slot]` × n）
- `fcc -O 0` は最適化パス・常駐・ピープホールを切る（`BuildOptions.OptimizeLevel` は 0 が「未指定 = 2」、-1 が -O 0）。
  レジスタ割付は -O 0 でも同じ `regalloc.AllocateRegister`（静的フレームの関数は固定番地に置く必要があるので、
  「全部フレーム」の簡易版は使えない）。`TestOptimizeLevel0` が番
- **生成コードのベンチマーク**（2026-09-15〜）: `go test ./bench`。[bench/](../../bench/README.md) の 12 本の .fc を emu で走らせ、
  `stdio.bench_start` / `bench_end` で囲んだ区間のサイクル数（`r6502.Cpu.Cycles`。ページクロス・分岐成立込みで決定的）と
  モジュールのセグメントサイズを `bench/results.json` と比べる。出力（チェックサム）の違いはコンパイラのバグ、
  サイクル数・サイズの違いは最適化の効果か退行で、どちらも `-update` で受け入れる（golden と同じ運用）。
  ベンチを書いたときに 16 ビットの除算・剰余のバグが 3 つ見つかった（`__mod_16` が stub、符号付き 16 ビットが符号無し除算、
  2 のべき乗の符号付き除算の `cmp $80`）。**新しい種類のコードを書くときは Python などで同じ計算を再現して照合する**と
  コンパイラのバグがすぐ見つかる
- **-O 2 が -O 0 より遅い形**（2026-10-02）: fuzz で見つけた、-O 2 のほうが遅かったプログラムを `internal/driver/testdata/perf/<名前>/`
  に置き、`TestPerfNotSlowerThanO0` が両レベルの出力が同じで -O 2 のサイクル数が -O 0 以下であることを見る。unroll-slower は
  展開した関数が静的フレームのゼロページを取ったのが原因だった（`frames.frameRefs` が展開の写しを数えないようにして直した。
  2015 万 → 1625 万サイクル、-O 0 は 1890 万。Agent/wiki/design/frame-alloc.md）
- **castle のマクロベンチ**（2026-09-19）: `go test ./internal/nes -run CastleFrameCycles -v`。内蔵 NES ランナーが
  `_ppu_vsync_flag` を読む `lda; bne` の待ちループを idle と数え、局面ごとの 1 フレームの busy サイクルを
  `bench/castle_frames.json` と比べる（`-update` で更新。[bench/README.md](../../bench/README.md)）。bench/ の 12 本と
  違って実ゲームの 1 フレームの重さが見える（フィールドで約 10,200 サイクル = 34%）。最適化の効果は両方で見る
- ca65 は既定で CPU 数だけ並列に走る。ca65 のエラー調査などで逐次にしたいときは `fc.Options.Jobs = 1`
  （CLI にはフラグ無し）
- examples と実プロジェクトの同期・差分確認: `tools/sync_examples.ps1`（詳細は
  [../examples/README.md](../../examples/README.md)）
- 内蔵NESランナーのスクリーンショット: `FC_NES_SNAPSHOT_DIR=<dir> go test ./internal/nes`（examples/hello は `FC_HELLO_PNG=<file>`）
- **QuickNES で画面を確かめる**（2026-09-29）: `internal/quicknes` は libretro の QuickNES のコア（`FC_QUICKNES` か
  `C:\Applications\libretro\quicknes_libretro.dll`）を cgo なしで（syscall で DLL を読み、コールバックは `syscall.NewCallback`）
  動かし、`Open(rom)` → `SetButtons` / `RunFrames` → `Image()`（256×240。上下左右の切り落としはコアの設定で切る）/ `RAM()`。
  内蔵のランナーは描画の途中の変化（スクロールの分割・ラスター効果）を描かないので、見た目はこちらで確かめる（MMC3 の走査線の
  IRQ も動く）。コアはプロセスに 1 つだけなので Open から Close まで大域の錠を持つ（並列のテストは待つ）。無ければテストは Skip。
  サンプルのテスト（`internal/nes/samples_test.go`）は `FC_SAMPLE_PNG_DIR=<dir>` で画面を PNG に書く。
  **ROM が変わるコンパイラの変更は castle を前と後で比べる**: `FC_QN_OLD=前.nes FC_QN_NEW=後.nes go test ./internal/quicknes
  -run ComparePlay -v`（`TestComparePlay`: タイトル → 開始 → 右へ歩いて跳ぶ 3000 フレームを同じ入力で動かし、30 フレームごとの
  画面 100 枚を比べる。前の ROM は `testdata/golden/examples/castle.nes`、後は `fcc build -t nes main.fc` を examples/castle/src の写しで）
- 内蔵NESランナー（internal/nes）の fc 向けの口（2026-09-29）: $4018 に書いた値を `Machine.Output` へ、$4019 に書いた値を終了コードに
  （`RunUntilExit`。NES の console が書く。`fcc test -t nes` が使う）、2P のパッド `SetButtons2`、描画中に vblank（NMI から
  `VblankCycles` = 2273 サイクル）の外で PPUDATA / OAM DMA に触った回数 `Stats.LateVramWrites` と、vblank の中で PPU に最後に
  触った時刻の最大 `Stats.MaxVblankUse`。ランナーの vblank は 21 走査線なので、はみ出しはこの値で数える
- **注意: `go test ./...` はパッケージを並列実行する**。examples をビルドするテストを
  新設するときは、リポジトリ内の `examples/*/src/.fc-build` を共有しないこと
  （`internal/nes` の Mesen テストは一時ディレクトリに複製してからビルドしている。
  共有すると片方の RemoveAll でもう片方のビルドが壊れ、単独実行では再現しない
  フレーク不良になる）
