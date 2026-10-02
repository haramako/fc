# コードの構造（パッケージの地図と IR の約束）

コードの構造（2026-09-28 の整理） の内容。

パッケージの依存の向きは `internal/driver/deps_test.go` の `TestImportDirection` が固定している（足したら表に書く）。

- **パイプライン**: `syntax`（goyacc の文法 `parser.y` と `checkVersion`）→ `sema`（`hlc.go` は文脈の型と共通の補助だけ。文は
  `stmt.go`、宣言は `decl.go`、式は `expr.go`、定数の評価は `consteval.go`、型式は `typeexpr.go`）→ `ir` → `pipeline.Prepare`
  （インライン展開・直接化 → volatile → `frames.Analyze` → 関数ごとに `opt.Optimize` → 引数の Y 渡しの印 → `regalloc` →
  `frames.Place`。順序はここだけが持つ。codegen は `pipeline.Backend` として呼び出しの計画 (`MarkArgY` / `CheckStackPush`)
  を提供する）→ `codegen`（`Llc` はモジュール単位、`funcGen`（genops.go）は関数単位で命令ごとのメソッド `genXxx`。
  生成した asm の後処理（ピープホール・レジスタの検査・分岐の延長・@log の地点）は `asm.go` の解析した行 `asmLine` の上で
  書く。ニーモニックの性質（書くレジスタ・フラグ・サイクル数）は `internal/m6502` の表 1 つ（regalloc の見積もりと共有）、
  番地の同一性は `operand.key()`（記号 + ずれ + 添字。綴りは見ない））。
- **driver とその周り**: `driver` はビルドの手順（`compileFront` = sema → Prepare → フレーム超過のやり直し。build / check /
  golden が共有）、`.s` / `.inc` の出力、ld65.cfg / base.s の生成、ca65 / ld65 の実行と asm のキャッシュ。設定ファイルと
  バンクの配置は `project`、ca65 / ld65 の探索と dbgfile は `cc65`、@log の生成物は `fclog`、FC_HOME の解決は `fchome`、
  emu ターゲットの実行とホストとのやり取りの取り決め ($fff0〜$ffff) は `emu`。
- **版と migrate**（2026-09-28。[Agent/wiki/plans/v4-plan.md](plans/v4-plan.md) §0）: 版は `syntax.Version2〜4`（`LatestVersion` = 4）で、モジュール
  ごと（`ir.Module.Version`。sema は `h.version()` で規則を選ぶ）。`fcc migrate` は `driver.Compiler.Migrate`: fc 2 → 3 は
  構文の書き換え（`internal/migrate` の `Rules`。ファイルごと）、fc 3 → 4 は意味の書き換えで、各ファイルを入口に sema で
  コンパイルし（`sema.Program.CollectRewrites`。fc 2 → 3 の結果はメモリの上の `Program.Overlay` で渡す）、sema が fc 3 の
  モジュールで fc 4 の意味と違う所を `Rewrite`（ソースの位置への挿入・置き換え。`sema/rewrite.go` の `rewriteAs` など）として
  集め、`migrate.ToV4` が当てる。書き換えを作れない所は `RewriteErrors` で migrate をエラーにする。fc 4 の規則を足すときは、
  fc 4 の意味（`h.version() >= syntax.Version4`）と、fc 3 のモジュールでの書き換え（`h.rewriting()`）を対で入れ、
  `TestMigrateExamples`（castle / miku を fc 4 に移して ROM の golden と比べる）と `TestMigrateGoldenPrograms` /
  `TestMigrateBench`（fc 2 の test / bench を fc 4 まで移す）で確かめる。同じ位置への挿入は報告の順に当たる（`internal/migrate` の `apply` の
  安定ソート）ので、同じ式の外側に付く書き換えは後で報告する（F6 の `as` は A1 の `(式) as T` の後: `((x + vx) as i8) as u16`）。
  A1 で広げた後の型を見る検査（F2 の値が必ず 0 になるシフト）は、広げるのが文の中で終わるので文の終わりに見る
  （`compileStatementRecover` の `checkShifts`）
- **追加のライブラリの探索先**（2026-09-29）: `BuildOptions.LibPath`（pkg/fc の `Options.LibPath`）は use / `@include` と ca65 の `-I` の
  探索先に、ソースのディレクトリの後・fclib の前で足す（`Compiler.libPath`・`ca65Args`）。`fcc test` がテストするモジュールの
  ディレクトリを足すのに使い、fc.toml の `[lib.*]`（Agent/wiki/plans/v4-stdlib.md §9）もここに入れる
- **組み込みが読み込むモジュール**（2026-09-29）: `@format` / fc 4 の printf は fmt / console を `use` 無しで使う（`sema/format.go` の
  `builtinModule`: 読み込んでいなければ `useModule` で読み込み、今のモジュールの `AddUse` に足す。足さないと asm がそのモジュールの
  .inc を取り込まずシンボルが未定義になる）。関数の本体のコンパイル中に読み込んだモジュールも本体をコンパイルするよう、
  `CompileAllBodies` は伸びたモジュールの一覧も最後まで回す（始めた時点の一覧を回していて、読み込んだ fmt の関数が空のままだった）
- **slice の一時変数とフレーム**（2026-09-29）: slice の型は内部では struct（`types.Universe.Slice`）で、regalloc は struct のローカルを
  「ポインタで触られうる」として専用の場所に置いていたので、文ごとの slice の一時変数（文字列リテラルを slice にしたものなど）が
  フレームを共有せず、`console.write("ab");` を 100 回並べた関数のフレームが 400 バイトになっていた。`&` を取られていない slice は
  live range で詰める側に入れ、コンパイラの一時変数で最初に現れるのが部分の書き込みなら、そこを使用にしない（部分の書き込みは残りを
  保つので使用でもある、という規則のままだと関数の頭から生きていることになる）。名前のある変数はループの前の周の値を読むことが
  あるので今までどおり。castle はフレームの番地が 58 バイト変わり、動作とサイクル数は同じ
- **外部のツールの同時起動の数**（2026-09-29）: `driver.toolSlots` がプロセス全体で ca65 / ld65 の同時起動を `2×CPU` に制限する。
  テストで多くのビルドを並べると、ビルドごとに CPU の数だけ ca65 を起動して数百のプロセスになり、Windows が "Not enough memory
  resources" でプロセスを作れなかった（起動できなかった理由がエラーから落ちていたのも直した: `CommandError.Result` に入れる）
- **演算子の優先順位は C と同じ**: `spec & ZERO == 0` は `spec & (ZERO == 0)`。fmt で踏んだ（括弧を付ける）
- **asm から呼ばれる関数とフレーム**（2026-09-29。[Agent/wiki/plans/v4-plan.md](plans/v4-plan.md) §2）: 静的フレームの重ね方は呼び出しグラフの到達関係
  だけで決まるので、グラフに見えない呼び出し（include した asm のファイルからの `jsr`）の先は `frames.Graph.hidden` にして
  どのフレームとも重ねない（割り込みの木と同じ）。インラインアセンブラの参照は含む関数からの辺にする。asm のテキストから
  fc のシンボルを拾うのは `ir.AsmSymbols` だけ（語の境界つき。語の途中の `_main` を拾うと、main が Entry になったり、hidden で
  全部のフレームが重ならなくなって castle が入らなくなる）。abi "frame" の asm の関数どうしが同じモジュールの中で呼び合うのは
  検出できない（定義のラベルと呼び出しを区別できない）ので仕様で禁止している。フレームを置く順は、ゼロページが必須の asm の
  関数が先、残りは深い順（`Graph.depth`）。深さは閉路（再帰の連鎖）の内側の辺を数えずに求める（数えると閉路の先の関数の深さが
  0 のまま残り、祖先より後に置かれてゼロページが埋まっていた。`TestDepthThroughCycle`）
- **調査用の設定**は `ir.Config`（FC_DISABLE / FC_TRACE_* / FC_DUMP_IR / FC_VERIFY_REGS）。環境変数を読むのは
  `ir.ConfigFromEnv` だけで、`BuildOptions.Config` → `sema.Program.Config` → `ir.Module.Config` と渡り、各段は `lmd.Cfg()` で
  引く。テストはビルドごとに別の設定を渡せる（`TestRandomMetamorphic` は同じプロセスで段を切って比べる）。
- **命令の性質の表** `ir/opinfo.go`（副作用が無い・終端・分岐・呼び出し・可換・グローバルを触る・C を受け取る・asm）。
  「この命令はどれか」の switch を書かずに `op.Code.IsPure()` などで引く。直線区間の障壁も `IsBlockBoundary()` に
  足す形で書く（sink は C を受け取る命令と asm、fieldindex は呼び出しと asm、indexoff は境界だけ）。
- **メモリアクセス**は `load_mem` / `store_mem` の 2 命令（`ir/mem.go`、[Agent/wiki/design/ir-memops.md](design/ir-memops.md)）。番地は
  `Base + Index * Scale + Disp`（store は `Width`）で、`op.Mem()` で引く（添字が無ければ `Index == nil`。Src[1] には番兵
  `ir.NoIndex` が入っている。DefUse の uses は Src の位置を保つので、SSA のように位置で引く側は NoIndex を変数と見ないこと）。
  `index`（`&a[i]` の番地の計算）はそのまま。fuse / fieldindex / indexoff / scale は Addr の畳み込みで、codegen の
  `genLoadMem` / `genStoreMem` は Base の種類（配列 / ポインタ）と添字の有無でアドレッシングを選ぶ。ポインタ経由の
  添字とずれは Y に足し込む（`(p),y` に変位は無い）ので、**添字 * scale + disp + 幅 ≤ 256 は作る側が保証する**
  （グローバルの配列は全体が 256 バイト以内、ポインタは配列の長さが型で分かる struct の配列フィールドだけ: fuseArrayField）。
- **幅と符号**（`ir/sign.go`）: 意味が入力の幅・符号で変わる命令は、それを命令に持つ。eq / lt の `Op.Width`（比較の幅）と、
  lt / div / mod / shift_right の `Op.Sign`（`ir.Unsigned` / `ir.Signed`。ほかの命令は `SignNone`）。sema は `Hlc.emit` で
  `ir.InferWidthSign` が入力の型から決め（比較は広い方の幅で、どちらかが符号付きなら符号付き。除算は Dst の符号。右シフトは
  入力の符号）、以後の段は `op.Width` / `op.IsSigned()` だけを見る。入力を差し替えても（cast を落とす、型の違うリテラルや
  常駐の値にする、バイトに分ける）意味は変わらない。opt が命令を作るときは元の命令から写すか `InferWidthSign` で決め、
  決め忘れ（比較の Width が 0、Sign が SignNone）は Verify が落とす。暗黙の変換（狭い入力のゼロ拡張、広い入力の下位の
  切り詰め。リテラルは値のバイト）は命令にしない（規則は codegen の `byte`・interp の `byteOf`・opt の `castBits` の 1 つ。
  符号拡張だけが `sign_extension`）。interp は差分テストの独立した判定役なので、sema の直後の IR を型から自分で解釈する。
- **常駐の印**は `Op.Res[ir.RegA / RegY / RegX]`（`Residency{V, In, Out}`）。codegen の状態も `res[reg]` / `resMem[reg]`。
  退避・復帰・入口 / 出口の写しはレジスタのループで書く（A / Y / X で 3 回書かない）。
- **常駐の形**（`regalloc/forms.go`）: 常駐のレジスタを使う・触らない特別な出し方（`Form`）を 1 回だけ書き、codegen は
  それで命令を出し（genLoad / genIf / genEq / genLt / genAddSub / genShift / genRotateCarry。`codegen/forms.go` の
  `placement` / `emitter`）、regalloc の `Classify`（`regalloc/classify.go`）は同じ形から friendly / free と得を決める。
  形が決まる条件のうち、どのレジスタに何があるかは `Placement`（codegen は割付の後の実際、regalloc は常駐させたときの
  見込み）、比較の結果がフラグに乗るかは引数（codegen は LocCond、regalloc は condPredicted）。得は形の命令列と
  「常駐させないときの命令列」（`base`）のサイクル差（m6502。ゼロページで数える）。形を足すときは forms.go に 1 つ
  足せば両方に効く。表の外（汎用の出力が A の値をそのまま扱う形・添字を Y / X のまま使う形・needsX / needsY）は
  classify.go の規則で、外れたら codegen が命令単位で出し直す（テストと fuzz ではコンパイルエラー）。
- **opt の段**は `opt.Pass{Name, Requires, Grows, Run, Then, Repeat}` で宣言し、`Pass.Apply` が FC_DISABLE の判定・compact・
  @log の付け替え・トレース・IR の検証を一括で行う。段を足すときは変換だけを書き、`Passes()` に並べる（順序の依存は
  そこのコメントに）。fuzz の切り分け `rpLocate` も `Apply` で 1 段ずつ当てる。
- **常駐の割付の後の IR を変えるとき**（2026-09-30）: 常駐の一時変数（`i@Y` など。`Value.Home` が退避先）への書き込みは、
  IR の上で読まれていなくても消してはいけない。ループ・関数の出口の書き戻し（`sty home`）、退避と復帰は codegen が `Op.Res`
  の印から出すので、`ir.BuildUseDef` には使用として現れない（`regalloc.PropagateLiteralLoads` が castle の
  `my_process.anchor_throw` の `anchor_vx@Y` への書き込みを消し、Mesen の自動プレイだけが落ちた。内蔵のランナーの
  自動プレイと fuzz は通った）。変わらない（`Clean`）常駐の退避先も、復帰で読まれるので中身が要る
- **IR の検証器** `ir.Verify`（命令の種類とオペランドの数、Dst の有無、ラベルの一意性と飛び先、push の Type、メモリの番地の形、
  比較の幅と符号の決め忘れ）。FC_VERIFY_IR=1
  で opt の各段・常駐・割付の後に走り、テストと fuzz では常に有効（`verify_test.go` の init）。「一時変数の定義は 1 つ」は
  前提にしていない（`||` / `&&` の一時変数と常駐の一時変数が複数回書かれる）。
- **ループの解析**は `ir/loops.go`（`CFG.DomTree()`、`CFG.Loops()` は Parent / Depth 付きで CFG にキャッシュ、`Preheader`、
  `Loop.EveryIteration`）。ループを変換する段は opt/loopmatch.go の `loopHeader` / `loopDefs` / `singleStep` を使う。
- **テストの共通の手順**は `internal/driver/harness_test.go` の `testBuild(t, buildSpec)`。新しいテストは既存の包み関数
  （`runEmu` / `buildFiles` / `buildBothLevels` …）か `testBuild` を使い、`NewCompiler(...).Build(...)` を直接書かない。
  例外は `internal/doccheck`（テストだけのパッケージ。docs/ の例のビルドと文書への参照の検査。driver の外なので `testBuild` を使えない）
- **`internal/extmacro`**（2026-10-02）: 外部コマンドの定数マクロ（fc.toml の `[macro_server.*]`）のプロセスと改行区切りの JSON の
  プロトコル・キャッシュ。何にも依存しない葉。fc.toml を読むのは `project/macros.go`、`@名前` の登録と定数への変換は
  `sema/extmacro.go`、意味解析の間だけ起動して止めるのは driver（`Compiler.projectMacros`）。計画は `wiki/plans/external-macros.md`
- **`internal/starmacro`**（2026-10-02）: Starlark のスクリプトの定数マクロ（fc.toml の `[macro_script.*]`）。外の依存は go.starlark.net
  だけ（リポジトリで唯一の外部の Go モジュール。go 1.24 で使える版に固定）。結果の型は extmacro の `Result` を共有し、sema の
  `MacroSource` として外部コマンドと同じ口で登録する
- **`internal/sizehtml`**（2026-10-02）: `--size-html` / `fcc size -html` のページ（`page.html` を埋め込み、データの JSON を差し込む）。
  データは `cc65.DbgFile`・`cc65.LinkConfig`（リンカ設定を読む: `cc65/linkcfg.go`）と、driver の `moduleCalls`（モジュールの間の呼び出し）
- **`internal/fcdoc`**（2026-09-30）: `fcc doc`。構文木から public の宣言と直前のコメントを集め、端末の文字とサイトの Markdown にする
  （`syntax` だけに依存。`pkg/fc` の `StdDocs` / `DocFile` / `DocIndex` が包む）
