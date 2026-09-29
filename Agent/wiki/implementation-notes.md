# 実装の勘所（機能別のメモ）

機能ごとの実装の要点とハマりどころ（常駐の正しさ、cast の正規形、symbol / address、far call、エラー報告、ツールの探索、CI、fmt / check、フレームの静的割付、最適化のパイプライン、インライン展開、ca65 の再利用、実プロジェクトのビルド構成）。パッケージの地図は [code-structure.md](code-structure.md)。

- **常駐レジスタの正しさを実際の命令列から決める**（2026-09-23）: regalloc の「どの命令が A / X / Y を使うか」
  （`freeA` / `needsX` / `needsY`）は codegen の出力を手で写した見積もりで、食い違いが fuzz で何度も出ていた
  （2 バイトの dec、cast を挟んだ if、Y 代用と融合、push_result の後の ldx …）。`CompileLambda` は命令の本体を出した後で
  実際に書いたレジスタを数え（`regsWritten`）、常駐を「触らない」（ResFree）とした命令が書いていたら、その命令だけ
  退避 / 復帰（ResClobber）にして関数ごとコンパイルし直す（ラベルの番号などは `saveState` で戻す）。見積もりは常駐の
  損得の計算にだけ使う（外れても遅くなるだけ）。直した数は `Llc.ResidentFixes`、中身は `FC_TRACE_RESIDENT=1`。
  castle / miku / golden では 0 回（出力は同じ）。`TestResidentDec16` の修正を戻しても 5 命令が退避に直って通る。
  普段のビルドでは見積もりが外れないので、この経路はテストで通らなかった（2026-09-28 のカバレッジで 0）。
  `TestResidentSelfCorrection` は `BuildOptions.MisclassifyResident`（テスト用。常駐の変数を触らない命令の ResClobber を
  ResFree にする）で見積もりをわざと外し、ランダムなプログラム 6 本で自己修正が働いて（約 5,600 命令を直す）実行結果が
  普段と同じになることを確かめる
  呼び出しの引数の保持（A の最後の引数、Y の引数、stack 系の X = FC_SP）の検査は codegen の中の約束事なので、
  `FC_VERIFY_REGS` のコンパイルエラーのまま。
  **2026-09-28 から**: 常駐の形（Y に常駐する変数の ldy / cpy / iny、A が塞がっているときの Y での代用、A を使わない
  メモリ上の inc / シフト / rol、フラグの分岐）は regalloc と codegen が同じ表（`regalloc/forms.go`）を引く。codegen は
  表で命令を出し、regalloc は同じ命令列から「置いたまま実行できるか」「A を触らないか」と得（m6502 のサイクル数）を
  計算する。関数ごとの作り直しは命令単位の出し直し（`compileOp` と `saveOp` / `restoreOp`）に置き換え、テストと fuzz
  （`FC_VERIFY_REGS`）では食い違いそのものをコンパイルエラーにした（外れうるのは表の外の予測: needsX / needsY と、
  汎用の出力が A の値をそのまま扱う形の規則）。`TestResidentSelfCorrection` の自己修正は命令単位で働く（約 7,400 命令）

- **cast の正規形**（2026-09-23、Linux に移ってから）: `ir.CastedValue` は常に 1 段で `{From, Type, Offset, Width}`
  （From の Offset バイト目から Width バイトを読み、上位はゼロ拡張）。入れ子の cast は `ir.NewCastedValue` が畳む。
  それまでは入れ子のまま持っていて、内側の切り詰めを各パスが読み落とすバグが fuzz で 8 件以上出ていた（`castBits` /
  `castFits` / `plainWord` / splitWords / 常駐の差し替え）。「元の変数の一部をそのまま読むか」は `ir.PlainOperand`、
  差し替えは `ir.RebaseCast`（元の幅を保つ）。ダンプは切り詰めたときだけ `{cast T off/width x}`。
  正規化のついでに 2 件見つかった: 常駐の `isStep` / `isMemShift` が `g0 = ((g0 as int) as int16) + 1` を
  「g0 のその場の inc」と見ていた（検査 `FC_VERIFY_REGS` がコンパイルエラーで捕まえる）、sema の定数の `as` が値を
  切り詰めずに型だけ貼り替えていて `(300 as int) as int16` が 300 だった（`TestCastNarrowThenWiden`）

- **`symbol:` / `address:` の整理**（2026-09-20）: `symbol: "name"` は関数・変数・配列定数に共通の「シンボル名」で、
  定義があればその名前で出力（`.export`）、無ければ asm 側の定義の参照（`.global`。`ir.DefExtern`、本体なし関数も
  `.export` から `.global` に）。`address:` は数値の固定番地だけ（文字列は `symbol:` へ誘導するエラー）。castle の
  sound.asm にあった `.global _nsd_bgm_BGM0 …` はこれで要らなくなった。参照のシンボルが同じモジュールで定義されている
  と `addDefModule` の重複除去で DefExtern が消える（= `.global` を出さない。それで正しい。`TestSymbolOption`）
- **ca65 の `.proc` の中のラベルは同じファイルの別の `.proc` から見えない**（2026-09-20）: レジスタ渡しの `sym__frame`
  を `.proc` の中に置いて `.export` したら、同じモジュール内の呼び出しで未定義になった（castle の text で発覚。
  fc のテストは他モジュールからの参照しか無かった）。関数の途中に入口を作るときは `.endproc` で閉じて別の `.proc` にする

- **文法 v2**（2026-09-14〜）: 先頭行 `#fc 2`（任意）。仕様は [language_reference.md](../../docs/language_reference.md)、設計の経緯は
  [Agent/discussions/2026-09-13-v2-grammar.md](../discussions/2026-09-13-v2-grammar.md)。**v1 は 2026-09-19 に削除**（`fcc migrate`、`internal/migrate`、`.rb` マクロの互換、
  `test/*.fc` の v1 版）。`test/*.fc` は v2 だけ。`syntax.File` / `ir.Module` にバージョンは無く、v1 だけの構文は
  `syntax.checkVersion` が「v2 ではこう書く」のエラーにする
- **struct / soa**（2026-09-14〜、v2 のみ）: 設計と実装メモは [Agent/wiki/design/types-struct.md](design/types-struct.md)、仕様は
  [language_reference.md](../../docs/language_reference.md) §2.1 / §2.2。テストは `internal/driver/struct_test.go` / `soa_test.go`
  （小さなプログラムを emu で実行）と `test/test_struct.fc` / `test_soa.fc`（unittest 形式。golden は無く、実行結果で判定）。
  `share/runtime.asm` の `__mul_16`（16 ビット乗算）は長らく `rts` だけの未実装で、このとき実装した（`j * 100` が 0 になっていた）
- **far call**（2026-09-15〜）: `options(farcall: true)` で有効。判定は `sema.Hlc.isFarCall`（呼び先モジュールの
  `ir.Module.Switchable()` = `bank` ≥ 0 かつ `near` 無し）、`ir.Op.Far`、codegen の `farCallSetup`（`.bank()` を使うので
  ld65.cfg の MEMORY に `bank = N` が要る。fc 生成の cfg は自動）。トランポリンは `fclib/<target>/farcall.asm`
  （emu / MMC0 は fc が用意）、MMC3 は `fclib/nes/farcall_mmc3.asm` を参考にプロジェクトが用意。テストは
  `internal/driver/farcall_test.go`。設計は [Agent/wiki/design/farcall.md](design/farcall.md)
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
  正規形は [internal/syntax/printer.go](../../internal/syntax/printer.go) 先頭のコメントと `TestFormatStyle` が定義。
  **リポジトリ内の .fc はまだ整形していない**（castle は製品コードなので一括整形はオーナー判断。整形しても asm は変わらない）

- **フレームの静的割付**（2026-09-16〜、[Agent/wiki/design/frame-alloc.md](design/frame-alloc.md) §6）: コンパイルの順序は
  sema（全モジュール）→ `pipeline.Prepare`（`frames.Analyze` で ABI を決める → 全関数の opt + regalloc →
  `frames.Place` で配置）→ `_frames.inc` を書く → 各モジュールの codegen → ca65。**codegen を直接呼ぶテストは
  `pipeline.Prepare` を先に呼ぶ**（golden の `newLlcForGolden`）。castle など base.asm を自前で持つプロジェクトは
  `FC_SZP` / `FC_SRAM` と `_SIZE` の export、スタックの空き先頭 `FC_SP` を足す（examples/castle/src/data.asm）
- **最適化のパイプライン**（2026-09-16〜）: sema（IR 生成）→ `internal/opt`（IR→IR。`ir.BuildCFG` / `ir.BuildUseDef` の上に
  書く小さなパスの列: SSA の定数 / コピー伝播と DCE（先頭。[Agent/wiki/design/ssa.md](design/ssa.md)）、ポインタ融合、コピー除去、
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
- **インライン展開**（`opt.InlineProgram`、2026-09-19）は `pipeline.Prepare` の最初（`frames.Analyze` の前）に
  プログラム全体で 1 回。IR の呼び出し列 `push_result; push_arg…; call` を、呼び先の変数・ラベルを付け替えた本体で
  置き換える（`return v` は `load 結果 = v; jump 終端`）。far call の `Far` フラグは sema が呼び先のモジュール基準で
  付けているので、**呼び出しを含む本体は別モジュールに写さない**（葉関数だけ）。他の呼び出しの引数の中も展開しない
  （fastcall の引数領域）。fclib の `math.abs` / `rand` / `sign` が `options(inline: true)`（castle で 110 か所）。
  **ハマった点**: 最初は `push_result … call` の間を丸ごと本体で置き換えていて、引数の式の計算（`abs(x % 16 - 8)` の
  `mod` / `sub`）まで消していた。梯子・敵との当たり・セーブポイントが「たまに効かない」という形で実プロジェクトの
  プレイで発覚（単体テストは引数が変数か定数だけだった）。今は push_arg をその場で引数への代入に置き換え、間の命令は
  残す（`TestInlineFunction` の e / f が番）
- **Y での引数渡し**（2026-09-20、Agent/wiki/design/frame-alloc.md §7.1）: 最後から 2 つ目の 1 バイト引数は Y。呼び出し側は
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
- **ca65 のオブジェクトの再利用**（2026-09-25、`internal/driver/asmcache.go`）: ビルドディレクトリの `<obj>.stamp` に
  ca65 が読んだ全ファイルの内容のハッシュを記録し、同じなら ca65 を起動しない。アセンブルの結果が怪しいときは
  `FC_NO_ASM_CACHE=1` で切るか、ビルドディレクトリ（`.fc-build`）を消す
- **castle のビルドの時間**（2026-09-28、何も変えない再ビルド、Windows）: fcc が 4.1 → 2.2 秒。内訳は sema 0.15・最適化と
  割付 0.6・コード生成 0.3・ld65 0.9（`--dbgfile` 無しなら 0.37）・残り 0.1 秒。直したのは 3 つ:
  (1) ld65 の dbgfile（13MB、21 万行の大半は line / span）の全行を正規表現で読み、しかも `@(address:)` の検査と -g のラベルで
  2 回読んでいた（1.4 → 0.05 秒。`ParseDbgFile` は seg / sym だけ読む、読むのは 1 回）、(2) ca65 の依存の出力は `.include` の
  たびに同じファイルを並べ、変えていないビルドでも 6,716 回ハッシュしていた（記録は 1 回ずつ、ハッシュはビルドの中で使い回す）、
  (3) 配布版の fcc が同梱の fclib を実行ごとに別の一時ディレクトリへ展開し、-g の `.dbg file` のパスが毎回変わって 45 個中 18 個の
  モジュールを毎回アセンブルしていた（ユーザーのキャッシュ `FC_CACHE_DIR`、無ければ `os.UserCacheDir()/fc` の
  `home-<中身のハッシュ>` に展開して使い回す。`internal/driver/home.go`）。(1)〜(3) で ROM は変わらない。
  段ごとの時間を測るときは、`BuildContext` の段の間に時刻を出す一時的な変更を入れて測った（常設の仕組みは無い）

## 実プロジェクトのビルド構成（参考）

- **fc-miku**: `fcc build -t nes miku.fc` だけで完結（fc 標準ドライバ）
- **castle**: `fcc build -t nes -o ../castle.nes main.fc` だけ（2026-09-19〜。main.fc の `options(base: "data.asm")` /
  `options(linker_config: "../ld65.cfg")` / `options(link: "... NSD.lib")` で自前の土台・リンカ設定・NSD を指定する。
  以前は `fcc compile` → `ca65 data.asm` → `ld65` を Rakefile が並べていた。リンク順が Rakefile の glob 順から
  fc の順 (base, runtime, モジュール順, 追加) に変わったので ROM のバイト列は変わる。
  生成物リソースは「出来合い許容」ポリシー。詳細は [../examples/README.md](../../examples/README.md)）
