# IR の命令の同一性を `*Op` に（2026-10-05）

roadmap.md「構造の整理」の「IR の命令の同一性を `*Op` に」を進めた記録（決定は [2026-10-05-structure-cleanup.md](2026-10-05-structure-cleanup.md)）。

## 決めたこと

- **位置は命令に持たせた手がかりで引く**（`Lambda.IndexOf`）。`Op.pos` を確かめ（`ops[pos] == op`）、合わなければ関数の命令を
  数え直す。消した命令は「数え直したときの命令列（スライスの先頭と長さ）と回数」を記して、同じ命令列のまま何度引いても数え
  直さない。命令を挿す・動かす・消す・詰める側は何も知らせなくてよい（opt のパスは `lmd.Ops = out` で作り直すものが多く、
  編集の API に全部を通すのは現実的でないため、引く側で確かめる形にした）。消した `*Op` をスライスへ直接書き戻すと見つからない
  ので、戻すのは `ir.ReplaceOp`（手がかりを付け直す）か新しいスライスで。
- **UseDef は差分で保つ**: 変数 → 命令（`*Op`）の一覧。消えた命令は引くときに除き（登録も外す）、動いた命令は位置の順に並べ
  直して返す。足した命令は `Add`、書き換えた命令は `Update`（以前の添字の UseDef は、書き換えた後も古いまま使われていた所が
  あった: coalesce・chain・carry）。
- **隣接は `ir.NextOp` / `ir.PrevOp`**: 段の中では消した命令の穴（nil）が残る。`ops[i+1] == nil` で諦めていた所（opt の carry・
  narrow・coalesce ×2・fuse・devirt・commute、regalloc の litprop・fuseload ×2・常駐の condPredicted、codegen の fusableIndex）を
  穴を越えて見るようにした。
- **BuildCFG は終端の後の穴だけの空のブロックを作らない**。jumps の「次のブロック」を見る判定（分岐の反転など）が、穴をはさむと
  黙って効かなかった。これが実際に効いていなかったのは fclib/nes/frame の空の待ちループ `while (ready != 0) {}`: 分岐の反転
  （`if !c goto L1; jump L2; L1:` → `if c goto L2`）ができず、ループの回転が条件を写していた。直して `lda; bne` の 1 つのループに
  なった（hello / jump / miku4 / statusbar / wave の ROM が変わる。ほかの examples・bench・test の出力はバイト一致）。
- **CFG の使い回し**: 前に作った CFG は、命令列の長さと制御の命令（ラベル・分岐・終端・switch・return）の位置・命令・ラベルが
  同じで、終端の後の穴に命令が戻っていなければ返す（支配木・ループのキャッシュも）。ほかの命令の置き換え・削除では作り直さない。
- **繰り返しの上限**: 解析を作り直して変化が無くなるまで繰り返す段（ssa 16・induction 8・unroll 8・ywalk 8・jumps 20、
  regalloc の常駐 32）は `untilFixed` に。命令の数に比例する回数（64 + 4 × 命令数）を超えたら、FC_VERIFY_IR（テストと fuzz）では
  内部エラー、そうでなければ止める。carry（Repeat 19）・devirt（32）・sink（無限の作り直し）は、UseDef を保ったまま関数を 1 回で
  見る形にした（carry は置き換えと削除だけで分岐の形を変えない、devirt は足した命令を Add、sink は同じスライスの中で動かす
  `ir.MoveOpBefore`）。frames の 16 回と volatileOnce の 8 段は探索の打ち切りなのでそのまま。
- **compact は `Pass.Apply` だけ**（段の中の 9 か所をやめ、`Pass.Repeat` を消した）。regalloc の litprop は opt の外なので自分で詰める。
- **Liveness も `*Op` が鍵**（`LiveInOp` / `LiveOutOp`。添字の `LiveIn` は今の命令列の i 番目の命令を引く）。

## 残したもの

- SSA（`opt/ssa.go` の useAt / defAt / blockOf）は添字のまま。1 回の書き換えの間は命令を挿さない（置き換えと削除だけ）ので
  困らない。induction・unroll・ywalk は 1 つ変換するごとに SSA と CFG を作り直す（挿した命令の SSA と CFG を差分で直すのは
  大きいので見送り）。

## 確かめたこと（2026-10-05）

- 各段で examples 8 本の ROM、bench 13 本と test 18 本の `-O 0` / `-O 2` のバイナリを前の fcc と比べた（上の frame の待ちループ
  以外は一致）。`go test ./...`、fuzz（TestRandom* を 2 回: -randn 300 と fold 40 / mutate 30 / metamorphic 20、FC_VERIFY_IR は
  テストで常に有効）で失敗なし。`untilFixed` の上限に達した関数は無かった。
- castle のビルド時間は前後で誤差の範囲（負荷の高い機械で 5 回ずつ交互に測り、1.7〜2.9 秒で重なる）。
