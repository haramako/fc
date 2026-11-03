# AI エージェントで作る

AI のコーディングエージェント（Claude Code、pi など）に fc でゲームを作らせるときの準備です。エージェントが fc の
まちがえやすい所とデバッグの道具を知っていると、行き詰まりにくくなります。

## スキルを入れる

[スキル](https://agentskills.io/)（エージェントが必要なときに読む手順書）を用意しています。fc の細かい仕様は書かず、
`fcc doc` とこのサイトを引くように書いてあるので、fc を更新しても古くなりにくくなっています。プロジェクトのフォルダで
次を実行します。

```bash
# pi など Agent Skills に対応したエージェント
mkdir -p .agents/skills/fc-nes-game
curl -fsSL -o .agents/skills/fc-nes-game/SKILL.md https://raw.githubusercontent.com/haramako/fc/main/skills/fc-nes-game/SKILL.md

# Claude Code
mkdir -p .claude/skills/fc-nes-game
curl -fsSL -o .claude/skills/fc-nes-game/SKILL.md https://raw.githubusercontent.com/haramako/fc/main/skills/fc-nes-game/SKILL.md
```

すべてのプロジェクトで使うなら、ホームの `~/.agents/skills/`（Claude Code は `~/.claude/skills/`）に置きます。
pi は、プロジェクトの `.agents/skills/` を読む前にそのプロジェクトを信頼するかを尋ねます（信頼すると読み込みます）。
対話しない `pi -p` では `-a` を付けると読み込みます。使う道具を絞るときは、ファイルを読む道具（`read`）を残します
（読めないとスキルの一覧が出ません）。
エージェントが自分で読み込まないときは、`/skill:fc-nes-game`（pi）や `/fc-nes-game`（Claude Code）で読ませます。
中身は [skills/fc-nes-game/SKILL.md](https://github.com/haramako/fc/blob/main/skills/fc-nes-game/SKILL.md) で見られます。

## デバッグの道具を置く

ROM を画面なしで動かして、`@log`・画面の中身・変数を端末に出す道具です（使い方は[デバッグ](../reference/debug#get-tools)）。
スキルにも同じ手順があるので、エージェントが自分で取ってくることもできます。

## エージェントに読ませる目次

このサイトの [llms.txt](/llms.txt) は、エージェントが最初に読む目次です（ページの一覧と、スキル・道具の場所）。
スキルを使わないエージェントには、作業の初めにこの URL（https://haramako.github.io/fc/llms.txt）を読ませます。
