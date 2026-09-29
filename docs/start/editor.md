# エディタ

VS Code の拡張があります。fc のコードの色付け、`fcc fmt` での整形、保存したときの `fcc check` でのエラーの表示ができます。

## 入れる

拡張はリポジトリの [tools/vscode-fc](https://github.com/haramako/fc/tree/main/tools/vscode-fc) にあります。ビルドは要りません。
このフォルダを VS Code の拡張のフォルダに `fc-lang` という名前でコピーし、VS Code を起動し直します。

```bash
git clone https://github.com/haramako/fc.git
cp -r fc/tools/vscode-fc ~/.vscode/extensions/fc-lang
```

Windows の拡張のフォルダは `%USERPROFILE%\.vscode\extensions` です。

`fcc` にパスが通っていなければ、設定の `fc.fccPath` に `fcc` の場所を書きます。

## 使う

- **色付け**: `.fc` のファイルを開くと色が付きます
- **整形**: 「ドキュメントのフォーマット」（Shift+Alt+F）で `fcc fmt` の書式にします。保存のたびに整形するには、設定に次を足します

  ```json
  "[fc]": { "editor.formatOnSave": true }
  ```

- **エラーの表示**: 保存すると `fcc check` を走らせ、エラーと警告を「問題」パネルとコードの下線に出します

## 設定

| 設定 | 既定 | 意味 |
|---|---|---|
| `fc.fccPath` | `fcc` | `fcc` の場所 |
| `fc.target` | `auto` | 検査のターゲット。`auto` なら fc.toml に `[target]` があれば nes、無ければ emu |
| `fc.checkOnSave` | `true` | 保存したときに検査する |
| `fc.mainFile` | （空） | 検査の起点にするファイル（ワークスペースからの相対）。空なら保存したファイル |

複数のモジュールに分けたプログラムでは、`fc.mainFile` に `main` のあるファイルを書いておくと、どのファイルを保存しても
プログラム全体が検査され、ほかのモジュールのエラーもそのファイルに出ます。
