# NES で Hello

画面に文字を出し、十字キーでスプライト（四角）を動かす NES の ROM を作ります。

![hello の画面](/samples/hello.png)

## ソース

リポジトリの [examples/hello/hello.fc](https://github.com/haramako/fc/blob/feature/v4/examples/hello/hello.fc) です。

<<< @/../examples/hello/hello.fc

## ビルドして動かす

```bash
fcc build -t nes -o hello.nes hello.fc
```

`-t nes` で NES の ROM（iNES 形式の `.nes`）を作ります。できた `hello.nes` をエミュレータで開いてください。
`-t` を省くと、fc.toml に `[target]` があれば NES、無ければ emu になります。

## 何をしているか

NES の画面は、PPU（画面を描く回路）が持つメモリ（VRAM）の中身で決まります。
**PPU が画面を描いている間は VRAM に書けません**。書けるのは、1 画面を描き終えてから次を描き始めるまでの短い間（vblank）だけです。
fc の標準ライブラリは、この決まりを次の 2 つのやり方で扱います。

- **描画を止めている間**は、`vram.write_now` / `vram.fill_now` でその場で書けます。最初の画面はこうして作ります
- **描画を出している間**は、`vram.put` がキューに積み、`frame.wait` が次の vblank でまとめて送ります

### 準備と最初の画面

- `frame.init()` は、画面を 0 で埋めて vblank ごとの割り込み（NMI）を有効にします。描画は止めたままです。ほかの関数より先に呼びます
- `vram.addr(0, 9, 10)` は、画面（ネームテーブル 0）の 8×8 ドットのタイルの (x, y) = (9, 10) の番地です。画面は横 32・縦 30 タイルです
- `@include("font.chr")` で、fc の内蔵のフォントを CHR（タイルの絵）として ROM に入れます。タイルの番号が ASCII の文字コードと同じなので、
  文字列をそのまま書けば文字が出ます
- 色は `pal.set_all(PALETTE)` で決めます。背景に 4 つ・スプライトに 4 つのパレットがあり、それぞれ 4 色です
- `vram.fill_now(vram.attr_addr(...), 0x55, 4)` は属性（背景のどのパレットを使うか）を書きます。属性の 1 バイトは 4×4 タイル分です
- `pal.bright(0)` で真っ暗にしてから `frame.render_on()` で描画を出し、`pal.fade(4, 4)` で 4 フレームごとに 1 段ずつ元の明るさに戻します

### 毎フレームの処理

`while (true)` の中が 1 フレーム（60 分の 1 秒）の処理です。

- `pad.poll()` でコントローラを読み、`pad.p1.held` の押しているボタンのビット（`pad.LEFT` など）を見ます
- スプライトは毎フレーム置き直します。`oam.begin()` の後に `oam.spr(x, y, タイル, 属性)` で置き、`oam.end()` で締めます
  （属性はパレットの番号と上下左右の反転。0 ならパレット 0 で反転なし）
- `vram.put(番地, slice)` で文字を書きます。`@format(line, "X {:3}  Y {:3}", x, y)` が作った slice をそのまま渡せます
- `frame.wait()` で次の vblank まで待ちます。その間に、スプライトと `vram.put` で積んだものが PPU に送られます

次は、この形の上に[小さなゲームを作ります](./first-game)。
