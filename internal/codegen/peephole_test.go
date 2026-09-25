package codegen

import (
	"strings"
	"testing"
)

// TestPeepholeFlagTests は検査のためだけのロード (testMark) と cpx #0 の削除が、実際の命令列でフラグが
// その値を映しているときだけ効くことを確かめる。
func TestPeepholeFlagTests(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "dec の直後の検査は消す",
			in:   []string{"dec _g", "lda _g" + testMark, "bne L"},
			want: []string{"dec _g", "bne L"},
		},
		{
			name: "別の場所は消さない",
			in:   []string{"dec _g", "lda _h" + testMark, "bne L"},
			want: []string{"dec _g", "lda _h", "bne L"},
		},
		{
			name: "間の常駐の復帰 (ldx) でフラグが変わる",
			in:   []string{"dec _g", "ldx <L+0", "lda _g" + testMark, "bne L"},
			want: []string{"dec _g", "ldx <L+0", "lda _g", "bne L"},
		},
		{
			name: "2 バイトの inc の中の分岐 (ラベル) の後は消さない",
			in:   []string{"inc _g", "bne @s", "inc 1+_g", "@s:", "lda _g" + testMark, "beq L"},
			want: []string{"inc _g", "bne @s", "inc 1+_g", "@s:", "lda _g", "beq L"},
		},
		{
			name: "2 バイトの dec は最後が dec lo なので消す",
			in:   []string{"lda _g", "bne @s", "dec 1+_g", "@s:", "dec _g", "lda _g" + testMark, "beq L"},
			want: []string{"lda _g", "bne @s", "dec 1+_g", "@s:", "dec _g", "beq L"},
		},
		{
			name: "書いた場所への sta の後は消さない",
			in:   []string{"dec _g", "sta _g", "lda _g" + testMark, "bne L"},
			want: []string{"dec _g", "sta _g", "lda _g", "bne L"},
		},
		{
			name: "直後が C を見る分岐なら消さない",
			in:   []string{"dec _g", "lda _g" + testMark, "bcc L"},
			want: []string{"dec _g", "lda _g", "bcc L"},
		},
		{
			name: "ldy の検査も消す",
			in:   []string{"dec <L+1", "ldy <L+1" + testMark, "bne L"},
			want: []string{"dec <L+1", "bne L"},
		},
		{
			name: "dex / ldx の直後の cpx #0 は消す",
			in:   []string{"dex", "cpx #0", "bne L", "ldx <L+2", "cpx #0", "beq M"},
			want: []string{"dex", "bne L", "ldx <L+2", "beq M"},
		},
		{
			name: "iny / ldy の直後の cpy #0 は消す (以前は最初の switch で flagsFromY を落としていて効かなかった)",
			in:   []string{"iny", "cpy #0", "bne L", "ldy <L+2", "cpy #0", "beq M"},
			want: []string{"iny", "bne L", "ldy <L+2", "beq M"},
		},
		{
			name: "Y 以外でフラグが変わった後、C を見る分岐の前の cpy #0 は残す",
			in:   []string{"iny", "inx", "cpy #0", "bne L", "iny", "cpy #0", "bcs M"},
			want: []string{"iny", "inx", "cpy #0", "bne L", "iny", "cpy #0", "bcs M"},
		},
		{
			name: "X 以外でフラグが変わった後の cpx #0 は残す",
			in:   []string{"dex", "iny", "cpx #0", "bne L"},
			want: []string{"dex", "iny", "cpx #0", "bne L"},
		},
	}
	for _, c := range cases {
		got := stripTestMarks(peepholeA(c.in))
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}
