# サンプル集

リポジトリの [examples](https://github.com/haramako/fc/tree/main/examples) にある fc 4 のサンプルです。
どれも標準ライブラリだけで書いていて、リポジトリのテストが毎回ビルドして動きを確かめています。

## hello — 文字とスプライト

![hello の画面](/samples/hello.png)

文字を出して黒からフェードし、十字キーで四角を動かします。いちばん小さな NES のプログラムです。
[NES で Hello](../start/hello-nes) で 1 行ずつ説明しています。

```bash
fcc build -t nes -o hello.nes hello.fc
```

[examples/hello](https://github.com/haramako/fc/tree/main/examples/hello)

## jump — ジャンプアクション

![jump の画面](/samples/jump.png)

十字キーで歩き、A で跳び（早く離すと低く跳ぶ）、足場の上のコインを 3 つ集めます。1/16 ドットの速さと重力、足場への着地、当たり判定。
[小さなゲームを作る](../start/first-game) で説明しています。

```bash
fcc build -t nes -o jump.nes jump.fc
```

[examples/jump](https://github.com/haramako/fc/tree/main/examples/jump)

## life — ライフゲーム

32×20 の端がつながった盤で、グライダー・点滅・ヒキガエルを動かし、世代ごとに `#` と `.` で出力します。
`console` への出力だけで書いているので、emu でも NES でも動きます。

```bash
fcc run life.fc
```

```text
gen 0
.#..............................
..#.............................
###.............................
................................
....................###.........
```

[examples/life](https://github.com/haramako/fc/tree/main/examples/life)

## statusbar — 画面の分割

![statusbar の画面](/samples/statusbar.png)

上の 4 行（ステータスバー）は止めたまま、下だけを横にスクロールし続けます。マッパーの MMC3 の走査線の割り込み（IRQ）で、
ステータスバーの下からスクロールを書き換えます。割り込みの中はアセンブリ（`split.asm`）で、fc.toml でマッパーを MMC3 にしています。

```bash
fcc build -t nes -o statusbar.nes statusbar.fc
```

[examples/statusbar](https://github.com/haramako/fc/tree/main/examples/statusbar)

## wave — ゆらゆら

![wave の画面](/samples/wave.png)

MMC3 の走査線の割り込みを 8 ラインごとに入れ、帯ごとに横のスクロールを sin でずらして画面を波打たせます。
帯ごとの位置は次のフレームの分を先に計算しておき、vblank の間に写します。

```bash
fcc build -t nes -o wave.nes wave.fc
```

[examples/wave](https://github.com/haramako/fc/tree/main/examples/wave)

## miku4 — 縦スクロールのシューティング

![miku4 の画面](/samples/miku4.png)

実際に作られたシューティングゲーム（fc-miku）を、fc 4 の標準ライブラリで書き直したものです。
スクロール、自機と弾、敵の動きを 3 つのモジュール（`miku.fc`・`common.fc`・`en.fc`）に分けています。

```bash
fcc build -t nes -o miku4.nes miku.fc
```

[examples/miku4](https://github.com/haramako/fc/tree/main/examples/miku4)
