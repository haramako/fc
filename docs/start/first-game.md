# 小さなゲームを作る

十字キーで歩き、A ボタンで跳び、足場の上のコインを 3 つ集めるゲームです。
[NES で Hello](./hello-nes) と同じ形（最初に画面を作り、毎フレーム入力を読んでスプライトを置く）に、動きと当たり判定を足します。

![jump の画面](/samples/jump.png)

ソースは [examples/jump/jump.fc](https://github.com/haramako/fc/blob/feature/v4/examples/jump/jump.fc)（135 行）です。
全体はページの最後に載せています。

```bash
fcc build -t nes -o jump.nes jump.fc
```

## 1/16 ドットで動かす

1 フレームに 1 ドットずつ動かすと、落ちる速さを滑らかに変えられません。そこで縦の位置と速さを 1/16 ドットの単位で持ちます。

```fc
const GRAVITY = 4; // 1/16 ピクセル / フレーム²
const JUMP = -80; // 跳んだときの速さ (1/16 ピクセル / フレーム。約 52 ピクセル上がる)
const FALL_MAX = 64;

var px:u8; // 自分の左上 (ピクセル)
var py:u16; // 自分の上の y (1/16 ピクセル)
var vy:i8; // 縦の速さ (1/16 ピクセル / フレーム。下が正)
var on_ground:bool;
```

- 画面の y は `py >> 4`（16 で割る）です。`py` は 240 × 16 まで入るように `u16` にします
- 速さ `vy` は上向きが負なので、符号付きの `i8` です
- `const` は型を書かなくても、値から型が決まります

## 跳ぶ・落ちる

毎フレーム、重力の分だけ速さを足し、速さの分だけ位置を動かします。

```fc
if (on_ground && (pad.p1.pressed & pad.A) != 0) {
	vy = JUMP;
	on_ground = false;
}
if (!on_ground) {
	if (vy < 0 && (held & pad.A) == 0) {
		vy /= 2; // 上がっている途中で A を離すと低く
	}
	vy = @min(vy + GRAVITY, FALL_MAX);
	py = @bitcast(u16, @bitcast(i16, py) + vy);
}
```

- `pad.p1.pressed` は押した瞬間のボタン、`pad.p1.held` は押しているボタンです。跳び始めは `pressed`、高さの調節は `held` で見ます
- `@min(a, b)` は小さい方です。落ちる速さに上限を付けます
- `@bitcast(型, 値)` は、ビットを変えずに型だけを読み替えます。ここでは `py`（`u16`）を符号付きの `i16` として読んで負になりうる `vy` を
  足し、`u16` に戻しています（符号付きと符号なしを混ぜた計算は符号付きの方に合わせるので、`py + vy` と書いても同じ値になります）

## 足場に乗る

足場はタイルの単位の (x, y, 幅) を並べた表です。最後の 1 つが床です。

```fc
const FLOORS = [4, 21, 6, 14, 17, 6, 24, 13, 6, 8, 10, 5, 0, 26, 32];

// 足の下 (y = feet) に足場があるか (x は自分の左端)
function floor_at(x:u8, feet:u8):bool
{
	var left = x >> 3;
	var right = (x + 7) >> 3;
	for (var i:u8 = 0; i < @len(FLOORS); i += 3) {
		var tx = FLOORS[i];
		if (feet == FLOORS[i + 1] * 8 && right >= tx && left < tx + FLOORS[i + 2]) {
			return true;
		}
	}
	return false;
}
```

落ちている間は、このフレームで足が通った y を 1 つずつ調べます。速く落ちても、足場の上の線を飛び越えて抜けません。

```fc
var before = ((py >> 4) as u8) + 8;
// (速さを足して py を動かす)
var after = ((py >> 4) as u8) + 8;
if (vy > 0) {
	for (var feet = before + 1; feet <= after; feet += 1) {
		if (floor_at(px, feet)) {
			py = ((feet - 8) as u16) << 4;
			vy = 0;
			on_ground = true;
			break;
		}
	}
}
```

- `x as u8` は型の変換です。`u16` から `u8` のように小さくするときは、`as` を書かないとエラーになります
- 足場の上にいるときに足の下に足場が無くなったら（端から歩いて出たら）、`on_ground` を `false` にして落とします

## コインを取る

```fc
function coins():void
{
	var y = (py >> 4) as u8;
	for (var i:u8 = 0; i < 3; i += 1) {
		if (got[i]) {
			continue;
		}
		var cx = COINS[i * 2];
		var cy = COINS[i * 2 + 1];
		if (hit.box(px, y, 8, 8, cx, cy, 8, 8)) {
			got[i] = true;
			score += 1;
			vram.put(vram.addr(0, 2, 2), @format(line, "COIN {}/3", score));
			if (score == 3) {
				vram.put(vram.addr(0, 12, 6), "CLEAR!");
			}
		} else {
			oam.spr(cx, cy - 1, COIN, 1);
		}
	}
}
```

- `hit.box(x1, y1, 幅1, 高さ1, x2, y2, 幅2, 高さ2)` は 2 つの四角が重なっているかです（標準ライブラリの `hit`）
- 取ったコインは `got` に印を付けて描かなくなり、取っていないコインはスプライトで描きます（`COIN` は `'$'` の文字のタイル）
- 点数は `@format` で文字にして `vram.put` で画面に書きます。描画中なので、次の `frame.wait` で送られます

## メインループ

```fc
function main():void
{
	frame.init();
	for (var i:u8 = 0; i < @len(FLOORS); i += 3) {
		vram.fill_now(vram.addr(0, FLOORS[i], FLOORS[i + 1]), BLOCK, FLOORS[i + 2]);
	}
	vram.fill_now(0x23c0 + 8, 0x55, 56); // 足場を緑に
	vram.write_now(vram.addr(0, 2, 2), "COIN 0/3");
	vram.write_now(vram.addr(0, 18, 2), "A:JUMP");
	pal.set_all(PALETTE);
	px = 16;
	py = (26 * 8 - 8) << 4;
	on_ground = true;
	frame.render_on();
	while (true) {
		pad.poll();
		move();
		oam.begin();
		oam.spr(px, ((py >> 4) as u8) - 1, BLOCK, 0);
		coins();
		oam.end();
		frame.wait();
	}
}
```

描画を止めている間に足場と文字を `fill_now` / `write_now` で書き、描画を出してから、入力 → 動き → スプライト → 待つ、を繰り返します。
自分のスプライトの y を 1 引いているのは、NES のスプライトが指定より 1 ライン下に出るためです。

## ソースの全体

<<< @/../examples/jump/jump.fc

次は、ファイルを分けてテストを書く[プロジェクト](./project)の作り方です。
