import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitepress'

// fc のコードの色付け: VS Code 拡張の TextMate の文法をそのまま Shiki に渡す（```fc のブロックに効く）
const fcGrammar = JSON.parse(
  readFileSync(
    fileURLToPath(new URL('../../tools/vscode-fc/syntaxes/fc.tmLanguage.json', import.meta.url)),
    'utf-8',
  ),
)

export default defineConfig({
  lang: 'ja-JP',
  title: 'fc',
  description: 'NES（ファミコン）のためのコンパイラ',
  base: '/fc/',
  cleanUrls: true,
  // エージェント向けの規約と、作り直す前の言語仕様（参考文献。サイトが揃ったら消す）はサイトに出さない
  srcExclude: ['AGENTS.md', 'CLAUDE.md', 'language_reference.md'],

  markdown: {
    languages: [{ ...fcGrammar, name: 'fc' }],
  },

  themeConfig: {
    nav: [
      { text: 'はじめに', link: '/start/install' },
      { text: 'サンプル', link: '/samples/' },
    ],
    sidebar: [
      {
        text: 'はじめに',
        items: [
          { text: 'インストール', link: '/start/install' },
          { text: '最初のプログラム', link: '/start/hello-emu' },
          { text: 'NES で Hello', link: '/start/hello-nes' },
          { text: '小さなゲームを作る', link: '/start/first-game' },
          { text: 'プロジェクト', link: '/start/project' },
          { text: 'エディタ', link: '/start/editor' },
        ],
      },
      { text: 'サンプル集', link: '/samples/' },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/haramako/fc' }],

    search: {
      provider: 'local',
      options: {
        miniSearch: {
          options: {
            // 日本語は空白で区切られないので、Intl.Segmenter で語に分ける。
            // この関数は文字列にしてブラウザへ渡される（検索の語にも使う）ので、外の変数を参照しないこと
            tokenize: (text: string) =>
              Array.from(new Intl.Segmenter('ja', { granularity: 'word' }).segment(text))
                .filter((s) => s.isWordLike)
                .map((s) => s.segment),
          },
        },
        translations: {
          button: { buttonText: '検索', buttonAriaLabel: '検索' },
          modal: {
            displayDetails: '詳しく表示',
            resetButtonTitle: '消す',
            backButtonTitle: '閉じる',
            noResultsText: '見つかりませんでした',
            footer: { selectText: '選ぶ', navigateText: '移動', closeText: '閉じる' },
          },
        },
      },
    },

    outline: { level: [2, 3], label: 'このページの内容' },
    docFooter: { prev: '前へ', next: '次へ' },
    darkModeSwitchLabel: '表示',
    lightModeSwitchTitle: 'ライトモードにする',
    darkModeSwitchTitle: 'ダークモードにする',
    sidebarMenuLabel: 'メニュー',
    returnToTopLabel: '先頭へ',
    langMenuLabel: '言語',
    notFound: {
      title: 'ページが見つかりません',
      quote: 'URL が変わったか、まだ書かれていないページです。',
      linkText: 'トップへ',
    },
  },
})
