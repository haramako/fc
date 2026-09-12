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
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/haramako/fc/pkg/fc"
)

const fmtUsage = `Usage: fcc fmt [-l] [-w] [-d] <file.fc> ...
    -l    list files whose formatting differs
    -w    write result to (source) file instead of stdout
    -d    display diffs instead of rewriting files
`

func runFmt(args []string) int {
	fs := flag.NewFlagSet("fcc fmt", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(fmtUsage) }
	list := fs.Bool("l", false, "list files whose formatting differs")
	write := fs.Bool("w", false, "write result to (source) file instead of stdout")
	diff := fs.Bool("d", false, "display diffs instead of rewriting files")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Print(fmtUsage)
		return 0
	}
	rc := 0
	for _, path := range fs.Args() {
		if err := fmtFile(path, *list, *write, *diff); err != nil {
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

func fmtFile(path string, list, write, diff bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := fc.Format(src, path)
	if err != nil {
		return err
	}
	if bytes.Contains(src, []byte("\r\n")) {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	changed := !bytes.Equal(src, out)
	switch {
	case list:
		if changed {
			fmt.Println(path)
		}
	case diff:
		if changed {
			fmt.Printf("--- %s (original)\n+++ %s (formatted)\n", path, path)
			fmt.Print(lineDiff(string(src), string(out)))
		}
	case write:
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
