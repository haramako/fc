package driver

// ca65 のオブジェクトの再利用 (差分ビルドの第 1 段。doc/v3_plan.md §4)。
//
// アセンブルするたびに、ca65 が読んだ全ファイル (`.s` 本体、.include した .inc / .asm、.incbin したファイル。ca65 の
// --create-dep で得る) の内容のハッシュと、ca65 自身 (パス・大きさ・更新時刻) と引数を `<obj>.stamp` に記録する。次の
// ビルドで全部が同じで `.o` も残っていれば ca65 を起動しない。Windows ではプロセスの起動と Defender の検査が重いので、
// 変わっていないモジュールの ca65 を飛ばす効果が大きい (Linux の castle では 0.1 秒前後)。
//
// 内容で比べるので、生成した .s / .inc を書き直しても (更新時刻が変わっても) 中身が同じなら再利用する。FC_HOME の
// 場所は記録では `$FCHOME` に置き換える (配布版の fcc は同梱の fclib / share を実行のたびに別の一時ディレクトリへ
// 展開するので、そのままでは引数も依存ファイルのパスも毎回変わる)。既知の穴:
// -I の探索で、前回見つかったファイルより前の探索先に同名のファイルを新しく置いても気づかない (前回のファイルが
// 変わっていなければ再利用する)。FC_NO_ASM_CACHE=1 で使わない。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// assembleCached は src をアセンブルする。前回のアセンブルから入力が変わっていなければ ca65 を起動しない。
func (c *Compiler) assembleCached(ctx context.Context, src string) error {
	args := c.ca65Args(src)
	obj := c.objPath(src)
	stamp := obj + ".stamp"
	useCache := os.Getenv("FC_NO_ASM_CACHE") == ""
	if useCache && c.stampValid(stamp, obj, args) {
		return nil
	}
	os.Remove(stamp) // 失敗したときに古い記録で .o を使わないように
	dep := obj + ".d"
	c.asmRuns.Add(1)
	if err := c.run(ctx, "ca65", append([]string{"--create-dep", dep}, args...)...); err != nil {
		return err
	}
	if !useCache {
		return nil
	}
	deps, err := readDepFile(dep)
	if err != nil {
		return nil // 記録できなければ次回もアセンブルするだけ
	}
	if s, err := c.makeStamp(args, deps); err == nil {
		writeIfChanged(stamp, []byte(s))
	}
	return nil
}

// objPath は src のオブジェクトファイルのパス (ca65Args の -o と同じ)。
func (c *Compiler) objPath(src string) string {
	base := filepath.Base(src)
	return filepath.Join(c.buildDir, strings.TrimSuffix(base, filepath.Ext(base))+".o")
}

// homeMark は記録の中で FC_HOME の場所を表す印。
const homeMark = "$FCHOME"

// portable は s の中の FC_HOME の場所を homeMark に置き換える (restore が逆)。
func (c *Compiler) portable(s string) string {
	if c.FCHome == "" {
		return s
	}
	return strings.ReplaceAll(s, c.FCHome, homeMark)
}

func (c *Compiler) restore(s string) string {
	if c.FCHome == "" {
		return s
	}
	return strings.ReplaceAll(s, homeMark, c.FCHome)
}

// makeStamp は ca65 と引数、読んだファイルの内容のハッシュの記録を作る。
func (c *Compiler) makeStamp(args, deps []string) (string, error) {
	var b strings.Builder
	tool, err := toolID()
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "tool %s\n", tool)
	fmt.Fprintf(&b, "args %q\n", c.portable(strings.Join(args, "\x00")))
	for _, d := range deps {
		abs, err := filepath.Abs(d)
		if err != nil {
			return "", err
		}
		h, err := fileHash(abs)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "file %s %q\n", h, c.portable(abs))
	}
	return b.String(), nil
}

// stampValid は stamp の記録が今の ca65・引数・ファイルの内容と一致し、obj が残っているか。
func (c *Compiler) stampValid(stamp, obj string, args []string) bool {
	if _, err := os.Stat(obj); err != nil {
		return false
	}
	data, err := os.ReadFile(stamp)
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) < 3 {
		return false
	}
	tool, err := toolID()
	if err != nil || lines[0] != "tool "+tool || lines[1] != fmt.Sprintf("args %q", c.portable(strings.Join(args, "\x00"))) {
		return false
	}
	for _, l := range lines[2:] {
		var h, q string
		if _, err := fmt.Sscanf(l, "file %s %q", &h, &q); err != nil {
			return false
		}
		got, err := fileHash(c.restore(q))
		if err != nil || got != h {
			return false
		}
	}
	return true
}

// toolID は ca65 の実体 (パス・大きさ・更新時刻)。ca65 を入れ替えたら再アセンブルする。
func toolID() (string, error) {
	p := ToolPath("ca65")
	if !filepath.IsAbs(p) {
		if lp, err := exec.LookPath(p); err == nil {
			p = lp
		}
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%q %d %d", p, st.Size(), st.ModTime().UnixNano()), nil
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:]), nil
}

// readDepFile は ca65 の --create-dep の出力 (`obj: dep dep ...`。空白は `\ `) から依存ファイルを読む。
func readDepFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	line := string(data)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	i := strings.Index(line, ":")
	for i >= 0 && i+1 < len(line) && line[i+1] != ' ' && line[i+1] != '\t' {
		// Windows のドライブ名 (C:\...) の ':' は飛ばす
		j := strings.Index(line[i+1:], ":")
		if j < 0 {
			i = -1
			break
		}
		i += 1 + j
	}
	if i < 0 {
		return nil, fmt.Errorf("%s: no target", path)
	}
	var deps []string
	var cur strings.Builder
	rest := line[i+1:]
	for k := 0; k < len(rest); k++ {
		ch := rest[k]
		switch {
		case ch == '\\' && k+1 < len(rest) && rest[k+1] == ' ':
			cur.WriteByte(' ')
			k++
		case ch == ' ' || ch == '\t':
			if cur.Len() > 0 {
				deps = append(deps, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		deps = append(deps, cur.String())
	}
	if len(deps) == 0 {
		return nil, fmt.Errorf("%s: no dependencies", path)
	}
	return deps, nil
}

// writeIfChanged は path の中身が data と違うときだけ書く (同じなら触らない: Windows の Defender の検査を減らす)。
func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(path, data, 0o666)
}
