# far call のレジスタ渡し（2026-10-05）

ユーザーの指示で roadmap の「far call にもレジスタで渡す」を実装した。設計の詳細は
[Agent/wiki/design/farcall.md](../wiki/design/farcall.md) §3.7。ここは決めたことと理由。

## 決めたこと

1. **トランポリンの規約を変える**: A / Y は呼び先の引数としてそのまま通し、A の戻り値をそのまま返す。速い経路は X だけで
   比べる（`ldx` / `cpx`）。切替の経路は A の引数をハードウェアスタックに退避して、呼ぶ直前に `tsx; lda $0102,x` で戻す。
   呼び先へは X = FC_SP で入る（stack 系の呼び先の引数の底。前は X をそのまま通していた）。
2. **名前を `farcall` → `farcall_ay` に変える**: 前の規約のトランポリンはプロジェクトがコピーして持っている（castle の
   `src/mmc3.asm`）。名前が同じだと、新しい fcc で黙って引数と戻り値が壊れる。名前を変えればリンクで落ちる
   （`Unresolved external 'farcall_ay'`）。fc 4 は互換を考えない方針なので、両方の入口を持つことはしない。
3. **FC_FARCALL は X で置く**: roadmap の案は「FC_FARCALL の設定を引数の読み出しの前に出す」だったが、入れ子の呼び出しの
   引数で FC_FARCALL が上書きされるうえ、A の連鎖も切れる。X は呼び出しが壊す扱いで、call の命令の前に X の常駐は退避
   済みなので、call の命令の中で `ldx` / `stx` で置けば引数の A / Y を壊さず、置く場所も変えずに済む。
4. **MMC3 の切替の経路はスロットごとに書く**: 最初はスロットの番号を積んで `@select` を呼ぶ形にしたが、castle の field で
   トランポリンの時間が 610 → 838 サイクル / フレームに増えた（+2.6%）。castle の far call は slot 1 の切替の経路が大半で、
   その経路が重くなっていた。スロットごとに即値で書くと 543 サイクルになり、全体で −0.6〜−0.8% になった。
5. **割り込みから届く関数の far call を警告にする**: レジスタ渡しで危険は増えない（A の退避はスタックで、割り込みはその下に
   積む）が、もともと FC_FARCALL とバンクの写しが主の処理と共有で、主の far call の途中に割り込みが far call すると壊れる
   （MMC3 の BANK_SELECT / BANK_DATA の書き込みの間の NMI も同じ）。設計では「対象外」としていたが、何も言わずに通っていた
   ので、frames の irqTree の関数の far call を警告にした。castle には該当が無い。

## 確かめたこと

- `TestFarCallRegisters16K` / `TestFarCallRegistersMMC3`（fclib の uxrom / mmc1 / mmc3 のトランポリン）、
  `TestRandomBankPrograms`（引数 2 つと再帰の関数を足し、トランポリンで Y を壊すと 100 本中 20 本が落ちることも確かめた）、
  `TestFarCallInInterruptWarns`。
- castle: field 8626 → 8559、area8b 12099 → 12027 サイクル。ROM 244474 → 244454。castle の熱い far call は引数が無いか
  2 バイトなので、呼ぶ側の関数の時間はほとんど変わらず、効果の大半はトランポリンの書き直し。

## 残り

- 実プロジェクトの castle の `src/mmc3.asm` を `farcall_ay` に書き換える（roadmap の castle 側の反映の項目に足した）。
