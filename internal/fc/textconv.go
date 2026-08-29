package fc

// castle プロジェクトの src/macro.rb が使う NesTools::TextConverter
// (nes_tools/lib/nes_tools/text_converter.rb) のうち、マクロに必要な部分の移植。
// conv() の tr 対応表はソース15文字に対して宛先14文字でずれている
// (\→＊, *→＝, =→＠, @→＠) が、Ruby 版の挙動を忠実に再現している。

import "strings"

// convertChar は CONVERT_CHAR 相当 (濁点・半濁点の分解など)。
var convertChar = map[rune]string{
	' ': "　", '\n': "↓", '?': "？", '!': "！", '-': "ー", '−': "ー", '一': "ー",
	'ヘ': "へ", '口': "ロ",
	'が': "゛か", 'ぎ': "゛き", 'ぐ': "゛く", 'げ': "゛け", 'ご': "゛こ",
	'ざ': "゛さ", 'じ': "゛し", 'ず': "゛す", 'ぜ': "゛せ", 'ぞ': "゛そ",
	'だ': "゛た", 'ぢ': "゛ち", 'づ': "゛つ", 'で': "゛て", 'ど': "゛と",
	'ば': "゛は", 'び': "゛ひ", 'ぶ': "゛ふ", 'べ': "゛へ", 'ぼ': "゛ほ",
	'ぱ': "゜は", 'ぴ': "゜ひ", 'ぷ': "゜ふ", 'ぺ': "゜へ", 'ぽ': "゜ほ",
	'ガ': "゛カ", 'ギ': "゛キ", 'グ': "゛ク", 'ゲ': "゛ケ", 'ゴ': "゛コ",
	'ザ': "゛サ", 'ジ': "゛シ", 'ズ': "゛ス", 'ゼ': "゛セ", 'ゾ': "゛ソ",
	'ダ': "゛タ", 'ヂ': "゛チ", 'ヅ': "゛ツ", 'デ': "゛テ", 'ド': "゛ト",
	'バ': "゛ハ", 'ビ': "゛ヒ", 'ブ': "゛フ", 'ベ': "゛ヘ", 'ボ': "゛ホ",
	'パ': "゜ハ", 'ピ': "゜ヒ", 'プ': "゜フ", 'ペ': "゜ヘ", 'ポ': "゜ホ",
}

// trFullwidth は conv() の tr(...) 相当。
// 'A-Za-z0-9.\-[](),:/";\\*=@' → 'Ａ-Ｚａ-ｚ０-９．−［］（），：／”；＊＝＠'
// 記号部はソースが1文字多く、Ruby の tr は宛先を最終文字でパディングするため
// 末尾4文字の対応がずれる (忠実に再現)。
func trFullwidth(s string) string {
	symbolMap := map[rune]rune{
		'.': '．', '-': '−', '[': '［', ']': '］', '(': '（', ')': '）',
		',': '，', ':': '：', '/': '／', '"': '”', ';': '；',
		'\\': '＊', '*': '＝', '=': '＠', '@': '＠',
	}
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteRune('Ａ' + (c - 'A'))
		case c >= 'a' && c <= 'z':
			b.WriteRune('ａ' + (c - 'a'))
		case c >= '0' && c <= '9':
			b.WriteRune('０' + (c - '0'))
		default:
			if r, ok := symbolMap[c]; ok {
				b.WriteRune(r)
			} else {
				b.WriteRune(c)
			}
		}
	}
	return b.String()
}

type tcEntry struct {
	index, count int
}

// TextConverter は NesTools::TextConverter 相当 (using 文字列指定での構築のみ対応)。
type TextConverter struct {
	using map[rune]*tcEntry
}

// NewTextConverter は TextConverter.new(nil, using) 相当。
func NewTextConverter(using string) *TextConverter {
	tc := &TextConverter{using: map[rune]*tcEntry{}}
	for _, c := range using {
		tc.registerChar(c)
	}
	return tc
}

func (tc *TextConverter) registerChar(c rune) *tcEntry {
	if e, ok := tc.using[c]; ok {
		e.count++
		return e
	}
	var e *tcEntry
	if len(tc.using) >= 256 {
		// Ruby版と同じく、あふれた文字は 255 になる (エラーにしない)
		e = &tcEntry{index: 255, count: 1}
	} else {
		e = &tcEntry{index: len(tc.using), count: 1}
	}
	tc.using[c] = e
	return e
}

// Conv は conv(str) 相当。文字コード列を返す。
// 表にない文字は新規登録される (Ruby版と同じ破壊的挙動)。
func (tc *TextConverter) Conv(str string) []int {
	str = strings.ReplaceAll(str, "\r", "")
	str = trFullwidth(str)
	var expanded strings.Builder
	for _, c := range str {
		if rep, ok := convertChar[c]; ok {
			expanded.WriteString(rep)
		} else {
			expanded.WriteRune(c)
		}
	}
	var r []int
	for _, c := range expanded.String() {
		r = append(r, tc.registerChar(c).index)
	}
	return r
}
