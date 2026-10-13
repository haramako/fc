# @log の注釈とパス（最適化を書くときの規則）

@log の注釈とパス（最適化を書くときの規則、2026-09-25） の内容。

fc 3 の `@log` は命令を出さず、次の命令への注釈 `ir.Op.Logs` になる（`internal/ir/log.go`、[Agent/discussions/2026-09-20-v3-plan.md](../discussions/2026-09-20-v3-plan.md) §9）。
最適化の判断には注釈を使わないので、生成コードは `@log` の有無で変わらない。そのかわり、命令を消す・作り直すパスは
注釈を正しい地点へ運ぶ責任がある。IR のパス（`internal/opt`、regalloc）を書く・直すときは次の 3 つを守る。

1. **命令の削除・差し替えはヘルパーを通す。** `ops[i] = nil` / `ops[i] = &ir.Op{…}` を直接書かない。
   - 消す: `ir.DropOp`（注釈は次に実行される命令へ）。到達しないコード: `ir.DiscardOp`（注釈も捨てる）
   - 置き換える: `ir.ReplaceOp`。2 つを 1 つにまとめる（融合）: `ir.MergeDrop`
   - `TestNoDirectOpWrites`（`internal/opt/logrules_test.go`）が `internal/opt` / `regalloc` / `codegen` の直書きを落とす。
2. **命令列を作り直すパス**（`out` を組み立てて `lmd.Ops = out`。ywalk、split、ループの回転・展開、インライン展開）は、
   消える命令の `Logs` を代わりに出す最初の命令へ引き継ぐ（新しい `ir.Op` に `Logs: op.Logs`）。命令を写すときは
   `ir.CloneLogs`（値を付け替えるなら mapOperand も）。命令を別の場所へ移すなら注釈は元の場所に残す（sink）。
   取りこぼしはパスごとの `ir.KeepLogs` が「元の位置の前で残った命令の直後」へ付け替えるが、地点はずれる。
3. **変数の値の意味を変える変換は印を立てる。** ループの中で変数の更新を別の変数に置き換える（ywalk のポインタ `p` を
   `p` と `k` の組にする）なら `Value.LogNoValue`、死んだ代入を消すなら `Value.LogStale`（SSA の eliminateDead）。
   立てないと `@log` が古い値を「正しい値」として出す。いちばん気づきにくい。
   `LogStale` は「生きている地点の値だけ正しい」だが、写しの伝播で同じ変数が別の版の値の置き場所として生き続けると足りない
   （`p0 = l0; l0 = (l0 + p0) % 327; return p0` が `return l0` になり、消した代入の後の `@log` が代入前の値を出した）。
   SSA の eliminateDead は、消す版を読む `@log` の引数に `LogArg.Stale` を付ける（codegen は「?」。2026-10-13、`TestLogDeadDefCopy`）
4. **asm の後処理では制御の流れを変えない**（2026-09-29）。`@log` の地点のラベル（`__fclog_N`）は分岐の延長の後で置く
   （`placeLogLabels`）ので、asm の上で `jmp L` の飛び先を付け替えたり `jsr f; rts` を `jmp f`（末尾呼び出し）にしたりすると、
   飛び先のラベルの地点・呼び出しの直後の地点が消える。`-g` / `@log` のときだけやめると ROM が変わる（`TestLogZeroCost`）。
   飛び先が値の無い return だけの jump は IR で注釈を引き継いで行う（`opt/jumps.go` の `jumpsToReturn`）。末尾呼び出しは
   呼び出しの直後の地点を置く所が無いので行わない。

検査: `TestLogZeroCost`（`internal/driver/log_test.go`）は fuzz のプログラムの全部の文の前に変数を全部出す `@log` を置き、
ROM とプログラムの出力が変わらないこと（-O 0 / -O 2）を確かめ、`@log` の値を -O 0 と -O 2 で比べて、1 つでも食い違えば失敗。
ただし -O 2 で地点がずれうる `@log` は比べない（`logImprecise`）: 合流点の地点（地点の情報の `prevs` がある）と、地点の情報が
`approx` のもの（丸ごと展開したループの見出しと最後のブロックの注釈 `LogPoint.Unrolled`、ループの回転で付け替えた `Moved`）。
展開したループの写しの注釈は、ジャンプの整理で合流点や出口へ寄り、別の周の値を出す（命令を持たない注釈の限界。nop を置けば
避けられるが ROM が変わる）。以前は全部を比べて「食い違った種が 1% + 3 個まで」としていて、本物の不具合（上の `LogArg.Stale`）が
その中に埋もれていた（2026-10-13 に食い違った 29 件を分けると 28 件がずれ、1 件が本物）。パスを変えたら `-randn 20000` などで
広く回す。
注釈の動きの調査には `FC_TRACE_LOGS=1`（パスごとの注釈の位置）、`FC_TRACE_LOGS=<関数のシンボル>`（命令列ごと）、
`FC_TRACE_LOG_ID=<@log の ID>`（その注釈の引数の値）。
