# プロジェクト

プログラムが大きくなったら、ファイル（モジュール）に分け、設定を fc.toml に書き、関数ごとにテストを書きます。

## モジュール

1 つのファイルが 1 つのモジュールです。モジュールの名前はファイル名から `.fc` を除いたものです。
`score.fc` を作ります。

```fc file=score.fc
#fc 4

public const MAX = 9999;

var best:u16;

// 点数を足す (MAX で止める)
public function add(score:u16, n:u16):u16
{
	if (n > MAX - score) {
		return MAX;
	}
	return score + n;
}

// いちばん良い点数を覚えておく
public function record(score:u16):bool
{
	if (score <= best) {
		return false;
	}
	best = score;
	return true;
}
```

別のファイルから `use score;` で使い、`score.add(...)` のように名前の前にモジュール名を付けて呼びます。

```fc run file=main.fc
#fc 4
use console;
use score;

function main():void
{
	console.init();
	var s = score.add(0, 1200);
	s = score.add(s, 9000);
	printf("score {}\n", s);
	if (score.record(s)) {
		printf("new record!\n");
	}
}
```

```bash
fcc run main.fc
```

```text
score 9999
new record!
```

- 宣言は、何も付けなければそのモジュールの中だけで見えます。ほかのモジュールに見せるものには `public` を付けます。
  `score.best` のように public でないものを使うとエラーになります
- グローバル変数（関数の外の `var`）は 0 で始まります。初期値は書けないので、関数の中で代入するか `const` にします
- `use` はモジュールを「ソースのディレクトリ → fc.toml に書いたライブラリ → 標準ライブラリ」の順に探します。
  同じ名前のファイルを置けば、標準ライブラリのモジュールを置き換えられます
- `use score as sc;` で別の名前を付けたり、`use add, MAX from score;` で名前を取り込んでモジュール名を付けずに使ったりもできます

## テスト

関数に `@(test)` を付けると、テストの関数になります。普通のビルドでは ROM に入りません。
`score.fc` の最後に足します。

```fc test file=score.fc
function test_add():void @(test)
{
	@assert_eq(add(100, 20), 120);
	@assert_eq(add(9990, 20), MAX);
}

function test_record():void @(test)
{
	@assert(record(10));
	@assert(!record(5));
	@assert(record(11));
}
```

```bash
fcc test score.fc
```

```text
score.test_add: ok
score.test_record: ok
2 tests ok
```

- `@assert(式)` は式が偽なら、`@assert_eq(実際, 期待)` は値が違えば、ファイルと行と値を出して止まります
- `fcc test` は emu で走らせます。`fcc test -t nes` なら内蔵の NES のランナーで走らせます

## fc.toml

プロジェクトの設定は `fc.toml` に書きます。`fcc` はソースのディレクトリから親へ向かって最初に見つかった `fc.toml` を使います。

```toml
# NES の ROM の形
[target]
mapper = "MMC3"     # NROM / UxROM / MMC1 / MMC3
prg = "128K"        # PRG ROM の大きさ (既定 32K)
chr = "8K"          # CHR ROM の大きさ (既定 8K。0 なら CHR RAM)
mirroring = "vertical"   # vertical / horizontal / four
battery = false     # バッテリーでセーブする RAM

# モジュールの @(build) の定数を上書きする
[define.pad]
REPEAT_DELAY = 20

# ほかのフォルダや git のリポジトリのモジュールを使う
[lib.util]
path = "../nes_util"

[lib.sound]
git = "https://github.com/someone/fc-sound"
rev = "v1.2.0"
```

- `[target]` があれば、`-t` を省いても NES の ROM を作ります
- `[define.モジュール名]` は、そのモジュールの `@(build)` を付けた定数の値を変えます。コマンドラインなら
  `fcc build -D pad.REPEAT_DELAY=20`
- `[lib.名前]` のモジュールは `use` で使えます。git のものはビルドのときに取ってきて、使ったコミットを `fc.lock` に書きます。
  `fcc lib update` で新しいコミットに進め、`fcc lib list` で一覧を出します

次は[エディタ](./editor)の設定です。
