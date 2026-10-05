# バンクと far call

マッパーで ROM のバンクを切り替えるプログラムは、fc.toml の `[target]` と `[bank.*]` でバンクを決め、モジュールを `@(bank: "名前")` で置く
（[fc.toml](./fc-toml#bank-名前)）。`main` のモジュールに `@(farcall);` を書くと、別の切り替えるバンクにあるモジュールの関数の呼び出しを、
コンパイラがバンクを切り替えて呼んで戻す形（far call）にする。呼ぶ側の書き方は普通の呼び出しと同じ。

- 固定の所・同じモジュール・`@(near)` の関数への呼び出しは普通の `jsr`。
- `farfn` の関数ポインタはバンクの番号を持ち、呼ぶときに切り替える。`fn` のポインタのバンクは呼ぶ側が決める。
- 別のバンクのデータを読むときのバンクは、呼ぶ側が決める。
- `fcc build -d` で far call になった所を並べる。

バンク切り替えの API は [uxrom](./std/nes/uxrom)・[mmc1](./std/nes/mmc1)・[mmc3](./std/nes/mmc3) を参照。
アセンブリの関数を呼ぶ場合は[呼び出し規約](./assembly#calling-conventions)も確認する。

## 割り込みと far call {#interrupts}

割り込み（NMI・IRQ）の中から far call しない（コンパイラが警告する）。far call の途中の状態（呼び先の番地・今のバンク）は
1 組だけなので、主の処理の far call の途中に割り込みが far call すると、主の呼び出しが別の関数へ飛ぶかバンクがずれる。
割り込みからは固定の所の関数だけを呼ぶ。

## トランポリン {#trampoline}

far call は、呼び先の番地とバンクを `FC_FARCALL`（3 バイト: 番地の下位・上位・バンク）に置いて、マッパーごとのアセンブリの
関数 `farcall_ay`（トランポリン）を `jsr` する。[uxrom](./std/nes/uxrom)・[mmc1](./std/nes/mmc1)・[mmc3](./std/nes/mmc3) の
モジュールはトランポリンを持つ。バンクを切り替えないターゲット（emu・マッパーの無い nes）にはコンパイラが用意する。

自前のマッパーのモジュールで far call を使うときは、固定の所に `farcall_ay` を書く（例:
[farcall_mmc3.asm](https://github.com/haramako/fc/blob/main/fclib/nes/farcall_mmc3.asm)）。決まりは次のとおり。

- A と Y は呼び先の引数なので、そのまま呼び先へ渡す。X は使ってよいが、呼び先へは X = `FC_SP` で入る。
- 呼び先から戻ったら、A（戻り値）をそのまま返す。X と Y は壊してよい。`FC_FASTCALL_REG` は触らない。
- 呼び先のスロットに既に同じバンクが入っていれば、切り替えずに `jmp (FC_FARCALL)` で飛ぶ。違えば今のバンクを
  ハードウェアスタックに退避して切り替え、`jsr` で呼んで、戻ったら元のバンクに戻す（入れ子でも動く）。
