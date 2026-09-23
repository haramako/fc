# 開発メモ（環境・運用・ハマりどころ）

fc を開発するときに知っておくべきこと。残っている仕事は [roadmap.md](roadmap.md)、
言語仕様は [language_reference.md](language_reference.md)。
（2026-09 以前の計画書・作業ログは [archive/](archive/)。fc はもともと Ruby で書かれていて 2026-08〜09 に Go に
移植した。Ruby 版はタグ `ruby-frozen` に残っている。Ruby 版との互換は以後考慮しない）

## ブランチ運用

- 開発は **`feature/v2`**（2026-09-12〜。文法 v2 / struct・soa / far call / ベンチ / 最適化）で行う。
  **安定するまで master へはマージしない**（一度マージしたが取り消し済み。master = 8358b15 のまま）。
  `agent/golang` は Go 移植のブランチで、feature/v2 の親。最適化（第 3 弾〜）は **`feature/static-frame`**
  （feature/v2 から分岐）で進めている。**実プロジェクト castle への反映と feature/v2 へのマージは SSA
  （第 4 弾の本命）が終わってからまとめて行う**（2026-09-19 決定）。それまで examples/castle が castle 側の
  変更（data.asm の FC_SZP / FC_SRAM / FC_SP、mmc3.fc の options、ppu.fc のスプライト消去）の置き場
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
| golden差分 | TestGolden*（internal/driver） | 数秒 | コンパイラ出力の全段階が基準と一致（stdout は `-O 0` でも同じ: TestGoldenStdoutO0） |
| ROMバイト一致 | TestExampleMiku / TestExampleCastle | 〜5秒 | 実プロジェクト2つのROMがスナップショットと一致 |
| 内蔵スモーク | TestSmoke* / TestPlayCastle（internal/nes） | 〜1秒 | 起動・NMI/IRQ・描画・自動プレイでの画面遷移 |
| 実機精度 | TestMesenPlayCastle | 〜10秒 | MesenCE 上での自動プレイ（エリア変数で判定） |
| 差分テスト | TestRandomPrograms（internal/driver） | 〜20秒 | ランダムな小プログラムを -O 0 / -O 2 で走らせて出力が一致 |

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
- 種 160000〜 で 2 件: splitWords が途中で 1 バイトに狭めた cast の連鎖 `((p0 as int) as int16)` を分けた変数の上位で
  読む（`plainWord`。`TestSplitNarrowWiden`）、sign_extension の入力が A にある（呼び出しの戻り値。call を A 割付の
  producer にしてから）とき N が A を反映しないまま `bpl` していた（`TestSignExtendCallResult`）
- 種 170000〜 で 1 件: A に常駐したグローバルを `return g0` で返すと、return が friendly（戻り値を A から書く）扱いで
  g0 の書き戻しが出なかった（グローバルの常駐は return を clobber に。`TestResidentGlobalReturn`）
- **2026-09-20 に生成器をさらに広げた**: far call（`far1.fc` = `options(bank: 1)` のモジュールに関数を 1〜2 個置いて
  main から `far1.ff0()`。main のグローバルは見えないので引数とローカルだけ）、密な switch（case 0〜11 で
  ジャンプテーブル = `switch` 命令になる形）、const の表（`const ct0:[16]T = [...]`）、関数ポインタ表
  （`const fp0:[N]fn(...):R = [t0, ...]` を `fp0[(e & (N-1))](args)` で呼ぶ。要素の関数は Entry になる）。
  初回の 30 本で 1 件: regalloc の live range の流れ（`CalcLiveRange` の `flow`）に `switch` 命令の飛び先が無く、
  飛び先で使う変数の生存区間が切れて直後の一時変数と番地を共有していた（`TestSwitchTableLiveRange`。
  ジャンプテーブルは 2026-09-19 からあり、castle では偶然重なっていなかった）
- 種 190000〜 で 1 件: 常駐レジスタへの差し替え（`makeResident` の replace）が cast を落としていて、`(x as int) >= 0` の x が
  X に常駐すると比較が符号付きになった（cast を残す。`TestResidentKeepsCast`）
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
- 2 かたまり目（種 372000〜、6 万本）で 2 件: ピープホールが番地を綴りで追跡していて `1+<F+5` と `0+<F+6`（同じ番地）
  を別物と見て、必要な `ldy` を消した（`canonAddr` で `k+<L+n` → `<L+(n+k)`、`+0` は落とす。`TestPeepholeAddressSpelling`。
  最初は `<F+0` の綴りが揃わず calls +3% になった: 正規化は**全部の綴り**に掛ける）。内側の領域から次の内側の領域へ
  移る辺で、外側の常駐の復帰 `ldx p1` が次の領域の入口の写し `ldx l3` の後ろに出て、X が p1 のままループに入った
  （復帰は前の領域の退避の後・次の領域の入口の写しの前: `isResSpill` で区別。`TestResidentEntryAfterRestore`）。
  失敗した種は `git worktree add /c/Work/fc_base <前のコミット>` で古い版と比べると、並行して入った他の変更の影響を
  切り分けられる
- **バンク切替の fuzz**（`internal/driver/bank_test.go`、2026-09-21）: emu にはバンクが無いので、切替バンクをまたぐ
  インライン化のバグ（表は元のバンクに残ってコードだけ移る）は既存の fuzz では見えなかった。nes ターゲット（MMC3、
  `bank_count: 8` でバンク 0 と 4 が同じ $8000 に来る）の小さなプログラムを生成して内蔵 NES ランナーで走らせ、生成器が
  計算した期待値と `-O 0` / `-O 2` を比べる（`TestRandomBankPrograms`。既定 8 本、`-randn 800` で 200 本 30 秒。
  トランポリンは `fclib/nes/farcall_mmc3.asm` を `include` し、`_mmc3_pbank_bak` を `symbol:` の var で持つ）。
  呼び出しの後で `pbank_bak` も見る（バンクの復帰）。ルールを切ると 8 本中 3 本が落ちることを確認済み
- 7 かたまり目（種 682000〜、6 万本）は失敗なし（長時間 fuzz で初めて）。8 かたまり目（種 742000〜781999 の 4 万本で止めた）で 1 件: 2 バイトの
  `g2++` は `inc lo; bne @s; inc hi` なので、下位が 0 に折り返すと Z は上位を映すのに、直後の `if ((g2 as sint))`
  （下位バイトの検査）が `flagsFromIncDec` でその Z を使っていた（2 バイトの inc の後は使わない。2 バイトの dec は最後が
  `dec lo` なので下位を映す。`TestIfLowByteAfterInc16`）
- 6 かたまり目（種 618000〜、6 万本）で 2 件、長時間 fuzz はここでいったん止めた（累計 37 万本で 14 件）:
  (1) stack 系（関数ポインタ経由）の呼び出しは push_result で `ldx FC_SP` してから `sta <S+k,x` で引数を積むが、X に
  常駐した変数の復帰 `ldx g1` が push_result の直後に出て X が戻り、引数が別の場所に書かれていた（push_result から
  call までは X の常駐をメモリ側にする `holdX`。入れ子の深さで数える。`TestStackCallHoldsX`）。(2) SSA の書き換えが
  `load l1 = ((l1 as int) as int16)` を「同じ場所・同じ型」だけ見て `load x = x` として消していた。内側で 1 バイトに
  狭めてからゼロ拡張するので切り詰めが消える（cast の各段が内側の幅に収まるときだけ消す `castFits`。
  `TestSSACastTruncateReload`）。**cast の連鎖の等価性は外側の型とオフセットの合計だけでは決まらない**（`castBits` と
  同じ穴）。ほかに `frame size over` が -O 2 だけで出る種（展開・インラインでフレームが 256 バイトを超える。runner は
  skip、roadmap に）
- 5 かたまり目（種 556000〜、6 万本）で 1 件: 2 バイトの `g0 -= 1` は `lda lo; bne; dec hi; dec lo` で下位を見るので
  A を壊すのに、freeA が 1 バイトの dec と同じく「A を使わない」と見ていて、A に常駐した g1 が消えた（2 バイトの inc は
  `inc lo; bne; inc hi` で壊さない。`TestResidentDec16`）
- 4 かたまり目（種 496000〜、6 万本）で 1 件: 符号付きの `x < 0` は x の最上位バイトの N フラグを見るが、x が呼び出し
  （除算のランタイム）の戻り値で A にあるとき、call の後の常駐の復帰 `ldx` が N を壊していた（`if` の戻り値検査と同じ
  形。A にある値は `testA` で `cmp #0`。`TestSignedLtZeroAfterCall`）。**call の後に常駐の復帰が出るので、call の直後の
  命令が「直前の lda のフラグ」を当てにする形は全部この穴がある**（if、符号付き `< 0`。残りは cmp / sbc を自分で出す）
- 3 かたまり目（種 434000〜、6 万本）で 1 件（2 本）: `x++` の直後の `if (x)` で x が A に常駐していると
  `flagsFromIncDec` が `byte()` で綴りを比べようとして codegen が panic（`invalid location a`）。A にある値は if の
  codegen が `cmp #0` で検査するので、A にある値では false に（`TestIfAfterIncResident`）
- **`symbol:` / `address:` の整理**（2026-09-20）: `symbol: "name"` は関数・変数・配列定数に共通の「シンボル名」で、
  定義があればその名前で出力（`.export`）、無ければ asm 側の定義の参照（`.global`。`ir.DefExtern`、本体なし関数も
  `.export` から `.global` に）。`address:` は数値の固定番地だけ（文字列は `symbol:` へ誘導するエラー）。castle の
  sound.asm にあった `.global _nsd_bgm_BGM0 …` はこれで要らなくなった。参照のシンボルが同じモジュールで定義されている
  と `addDefModule` の重複除去で DefExtern が消える（= `.global` を出さない。それで正しい。`TestSymbolOption`）
- **ca65 の `.proc` の中のラベルは同じファイルの別の `.proc` から見えない**（2026-09-20）: レジスタ渡しの `sym__frame`
  を `.proc` の中に置いて `.export` したら、同じモジュール内の呼び出しで未定義になった（castle の text で発覚。
  fc のテストは他モジュールからの参照しか無かった）。関数の途中に入口を作るときは `.endproc` で閉じて別の `.proc` にする

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
  `-update` で生成する。`-O 2` は SSA の定数伝播で演算をほとんど畳んでしまう（test_op はほぼ消える）ので、
  同じプログラムを `-O 0` でも走らせる `TestGoldenStdoutO0` が codegen を検証する（golden は同じファイル）
- 性能退行の検知 (コンパイラ自身の速度): `go test ./internal/driver -run xxx -bench BenchmarkCastle -benchmem`
- **フレームの静的割付**（2026-09-16〜、[v2_frame_alloc.md](v2_frame_alloc.md) §6）: コンパイルの順序は
  sema（全モジュール）→ `codegen.PrepareProgram`（`frames.Analyze` で ABI を決める → 全関数の opt + regalloc →
  `frames.Place` で配置）→ `_frames.inc` を書く → 各モジュールの codegen → ca65。**codegen を直接呼ぶテストは
  `PrepareProgram` を先に呼ぶ**（golden の `newLlcForGolden`）。castle など base.asm を自前で持つプロジェクトは
  `FC_SZP` / `FC_SRAM` と `_SIZE` の export、スタックの空き先頭 `FC_SP` を足す（examples/castle/src/data.asm）
- **最適化のパイプライン**（2026-09-16〜）: sema（IR 生成）→ `internal/opt`（IR→IR。`ir.BuildCFG` / `ir.BuildUseDef` の上に
  書く小さなパスの列: SSA の定数 / コピー伝播と DCE（先頭。[v2_ssa.md](v2_ssa.md)）、ポインタ融合、コピー除去、
  ジャンプ整理、ループ回転…）→ `internal/regalloc` → `internal/codegen`
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
  （fastcall の引数領域）。fclib の `math.abs` / `rand` / `sign` が `options(inline: true)`（castle で 110 か所）。
  **ハマった点**: 最初は `push_result … call` の間を丸ごと本体で置き換えていて、引数の式の計算（`abs(x % 16 - 8)` の
  `mod` / `sub`）まで消していた。梯子・敵との当たり・セーブポイントが「たまに効かない」という形で実プロジェクトの
  プレイで発覚（単体テストは引数が変数か定数だけだった）。今は push_arg をその場で引数への代入に置き換え、間の命令は
  残す（`TestInlineFunction` の e / f が番）
- **Y での引数渡し**（2026-09-20、v2_frame_alloc.md §7.1）: 最後から 2 つ目の 1 バイト引数は Y。呼び出し側は
  `push_arg` から `call` までの間の命令が Y を使わないときだけ（`codegen.markArgY` → `ArgY` / `HoldY`）。**ハマった点**:
  (1) 最初は `push_arg` / `call` だけ見ていて、castle の hot な関数（`fastcall: true` = `push_fastcall_arg` / `fastcall`）が
  全部 `__a` の入口に落ちて +0.8% 退行した。(2) `push_arg` を間の命令の下に沈める案は A の連鎖（演算の結果を A のまま
  渡す）を壊して損。(3) 常駐の復帰（`ldy home` / `lda home`）が引数を置いた `push_arg` の直後に出ると引数が消える:
  保持中は退避も復帰もしない（A 渡しにも潜在していた）。効果の測り方は castle のフレーム（`go test ./internal/nes -run
  CastleFrame -v`）が一番敏感で、bench は calls 以外ほぼ動かない。`FC_CASTLE_DIR=C:\Work\castle` を付けると実プロジェクトをその場で
  ビルドして測る（自動プレイはマップが違うので field だけ。json との比較は当然 FAIL するが内訳は出る）
- **自動インライン**（2026-09-20）: 印が無くても小さい関数（12 命令以下、ループ・呼び出し・asm・`&f`・配列 / struct の
  ローカル無し）は同じ仕組みで展開する（6 命令以下は無条件、それより大きいものは呼び出し 2 か所まで。`opt.autoInlinable`）。
  **テストで「この関数が出力される」ことを見るときは `options(noinline: true)` を付ける**（`TestUnusedFunctions` /
  `TestDebugInfoAndSizeReport` / `TestFarCall` は小さな関数が消えて落ちた）。const の別名（`const D2 = f`）で参照される
  関数は呼び出しが全部展開されても出力が要る（`frames.Analyze` が DefEqu を根に足す）
- **実プロジェクトの退行の切り分け**（2026-09-19）: `FC_DISABLE=名前,名前,...` で最適化のパスを個別に切れる
  （`ir.Disabled`。名前は `internal/ir/disable.go`: ssa mul indexoff induction unroll devirt autoinline sink fuse coalesce chain narrow scale commute carry split rotate dup
  inline resident func-resident step shift8 fuse-index switch peephole）。`internal/nes/probe_test.go` は環境変数が
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
`.mlb` は `fcc size` と同じ `driver.DbgFile.WriteMlb` で作れる（CLI は未提供。castle は `fcc build` に移ったので不要）。

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
- **castle**: `fcc build -t nes -o ../castle.nes main.fc` だけ（2026-09-19〜。main.fc の `options(base: "data.asm")` /
  `options(linker_config: "../ld65.cfg")` / `options(link: "... NSD.lib")` で自前の土台・リンカ設定・NSD を指定する。
  以前は `fcc compile` → `ca65 data.asm` → `ld65` を Rakefile が並べていた。リンク順が Rakefile の glob 順から
  fc の順 (base, runtime, モジュール順, 追加) に変わったので ROM のバイト列は変わる。
  生成物リソースは「出来合い許容」ポリシー。詳細は [../examples/README.md](../examples/README.md)）
