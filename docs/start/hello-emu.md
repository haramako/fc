# 最初のプログラム

`fcc` は 6502 のエミュレータを中に持っていて、書いたプログラムをその場で動かし、出力を端末に出せます。
これを **emu ターゲット**と呼びます。NES の画面や割り込みを知らなくても、fc の書き方を試せます。

## Hello

`hello.fc` という名前で保存します。

```fc run
#fc 4
use console;

function main():void
{
	console.init();
	printf("Hello, fc!\n");
}
```

```bash
fcc run hello.fc
```

```text
Hello, fc!
```

- `#fc 4` は、このファイルを fc 4 の言語として読むという印です。ファイルの 1 行目に書きます
- `use console;` は標準ライブラリの `console` モジュールを使うという宣言です。`printf` はここに出力します
- `main` からプログラムが始まります。emu では `main` から戻るとプログラムが終わります
- `console.init()` は出力の準備です。emu では何もしませんが、同じプログラムを NES で動かすときに画面を用意します

`fcc run` はビルドしてすぐ動かします。`fcc build hello.fc` ならビルドだけして `a.bin` を作ります（`-o` で名前を変えられます）。

## 変数・ループ・書式

```fc run
#fc 4
use console;

var line:[16]u8;

// 1 から n までの和
function sum_to(n:u8):u16
{
	var sum:u16 = 0;
	for (var i:u8 = 1; i <= n; i += 1) {
		sum += i;
	}
	return sum;
}

function main():void
{
	console.init();
	for (var n:u8 = 10; n <= 40; n += 10) {
		printf("1..{:2} = {:4}\n", n, sum_to(n));
	}
	var hp:u8 = 7;
	var s = @format(line, "HP {:3}/{:03}", hp, 50);
	printf("[{}] {} bytes, {:x} {:b}\n", s, @len(s), 255, 5);
}
```

```text
1..10 =   55
1..20 =  210
1..30 =  465
1..40 =  820
[HP   7/050] 10 bytes, ff 101
```

- 整数の型は `u8`（0〜255）・`u16`（0〜65535）・`i8`・`i16` です。6502 は 8 ビットの CPU なので、`u8` で足りる所は `u8` にすると
  速く小さくなります
- `var 名前:型 = 値;` で変数を宣言します。関数は `function 名前(引数:型):戻り値の型` です
- 書式の `{}` に引数が順に入ります。`{:4}` は幅 4 で右に寄せ、`{:03}` は 0 で埋め、`{:x}` は 16 進、`{:b}` は 2 進です
- `@format(line, ...)` は配列 `line` に書式どおりに書き、書いた部分を **slice**（先頭と長さの組）で返します。`@len(s)` がその長さです。
  NES の画面に文字を出すときも、こうして作った slice を渡します

## エラー

fc 4 は、型に入らない定数を黙って切り詰めません。

```fc error
#fc 4
use console;

function main():void
{
	console.init();
	var x:u8 = 300;
	printf("{}\n", x);
}
```

```text
over.fc:7:2: error: 300 does not fit in u8 (fc 4 does not truncate a constant implicitly; write `300 as u8` to truncate)
```

エラーは「ファイル:行:桁」から始まります。`fcc check over.fc` なら、ファイルを作らずにエラーと警告だけを調べられます。

次は [NES で Hello](./hello-nes) を出します。
