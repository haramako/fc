---
layout: home

hero:
  name: fc
  text: NES（ファミコン）のためのコンパイラ
  tagline: C に近い言語で書いたプログラムを、6502 のアセンブリと NES の ROM にします
  actions:
    - theme: brand
      text: GitHub
      link: https://github.com/haramako/fc
    - theme: alt
      text: ダウンロード
      link: https://github.com/haramako/fc/releases

features:
  - title: C に近い書き方
    details: 関数・構造体・ポインタに加えて、長さを持つ slice と書式つきの出力（@format / printf）があります。
  - title: 6502 に合わせたコード
    details: 関数の変数は静的なフレームに置き、ループの変数はレジスタに置きます。使わない関数とデータは ROM に入りません。
  - title: NES の標準ライブラリ
    details: 画面の更新・パレット・スプライト・パッド、内蔵のフォント、UxROM / MMC1 / MMC3 のバンク切り替えがそろっています。
  - title: すぐ試せる
    details: 内蔵のエミュレータで動かす fcc run、単体テストの fcc test、Mesen でのソースレベルデバッグ（-g）があります。
---

::: warning fc 4 は開発中です
言語と標準ライブラリはまだ変わります。このサイトも作っている途中です。
:::

## こんなコードです

```fc
#fc 4
use console;

const SCORES:[?]u16 = [120, 45, 300, 7];
var line:[16]u8;

// いちばん大きい値を返す (配列は slice で受け取る)
function max_of(xs:[]const u16):u16
{
	var m:u16 = 0;
	for (var x in xs) {
		m = @max(m, x);
	}
	return m;
}

function main():void
{
	console.init();
	printf("{}\n", @format(line, "HI-SCORE {:05}", max_of(SCORES)));
	console.exit(0);
}
```

`fcc run hiscore.fc` で、fcc の内蔵のエミュレータで動きます。

```text
HI-SCORE 00300
```
