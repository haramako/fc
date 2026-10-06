package main

// fcc fmt: ソースの整形 (gofmt 相当)。
//
//	fcc fmt [-l] [-w] [-d] <file.fc> ...
//	  (フラグなし)  整形結果を標準出力に書く
//	  -l           整形で変わるファイル名を列挙する
//	  -w           ファイルを上書きする
//	  -d           差分を表示する
//
// 入力が CRLF なら出力も CRLF にする (作業ツリーの改行を変えない)。

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/haramako/fc/pkg/fc"
)

// rewriteFlags は fmt / migrate の結果の出し方 (無ければ標準出力に書く)。
type rewriteFlags struct {
	List  bool `short:"l" help:"List files whose result differs."`
	Write bool `short:"w" help:"Write the result to the (source) file instead of stdout."`
	Diff  bool `short:"d" help:"Display diffs instead of rewriting files."`
}

type fmtCmd struct {
	rewriteFlags
	Files []string `arg:"" name:"file" help:"Source files."`
}

func (c *fmtCmd) run() int {
	rc := 0
	for _, path := range c.Files {
		if err := fmtFile(path, c.rewriteFlags); err != nil {
			var ce *fc.Error
			if errors.As(err, &ce) {
				fmt.Fprintf(os.Stderr, "%s: error: %s\n", ce.Pos, ce.Msg)
			} else {
				fmt.Fprintln(os.Stderr, err)
			}
			rc = 1
		}
	}
	return rc
}

func fmtFile(path string, f rewriteFlags) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := fc.Format(src, path)
	if err != nil {
		return err
	}
	return f.emit(path, src, out, "formatted")
}

// emit は path の元の内容 src と書き換えた内容 out (LF) を、フラグに従って出す (fmt と migrate で共通)。入力が CRLF なら出力も
// CRLF にする。label は差分の見出し (`+++ path (label)`)。
func (f rewriteFlags) emit(path string, src, out []byte, label string) error {
	if bytes.Contains(src, []byte("\r\n")) {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	changed := !bytes.Equal(src, out)
	switch {
	case f.List:
		if changed {
			fmt.Println(path)
		}
	case f.Diff:
		if changed {
			fmt.Printf("--- %s (original)\n+++ %s (%s)\n", path, path, label)
			fmt.Print(lineDiff(string(src), string(out)))
		}
	case f.Write:
		if changed {
			return os.WriteFile(path, out, 0o666)
		}
	default:
		os.Stdout.Write(out)
	}
	return nil
}

// lineDiff は行単位の差分を unified 風 (文脈なし) に出す。LCS による素朴な実装 (ソースは小さい)。
func lineDiff(a, b string) string {
	al := strings.SplitAfter(a, "\n")
	bl := strings.SplitAfter(b, "\n")
	n, m := len(al), len(bl)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var sb strings.Builder
	emit := func(sign byte, s string) {
		sb.WriteByte(sign)
		sb.WriteString(strings.TrimRight(s, "\r\n"))
		sb.WriteByte('\n')
	}
	i, j := 0, 0
	inHunk := false
	for i < n || j < m {
		if i < n && j < m && al[i] == bl[j] {
			i++
			j++
			inHunk = false
			continue
		}
		if !inHunk {
			fmt.Fprintf(&sb, "@@ -%d +%d @@\n", i+1, j+1)
			inHunk = true
		}
		if i < n && (j >= m || lcs[i+1][j] >= lcs[i][j+1]) {
			emit('-', al[i])
			i++
		} else {
			emit('+', bl[j])
			j++
		}
	}
	return sb.String()
}
