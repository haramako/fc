package driver

// プロジェクトの設定ファイル fc.toml (doc/v3_plan.md §1 / §3)。ソースの基準ディレクトリから親へ向かって最初に見つかった
// ものを使う。TOML の必要な分だけを読む: `[section]` / `[a.b]` の見出し、`key = value` (値は true / false / 整数 /
// "文字列")、`#` から行末のコメント。今使う見出しは [define.<module>] (@(build) の const の上書き)。

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/sema"
)

// ConfigName はプロジェクトの設定ファイルの名前。
const ConfigName = "fc.toml"

// ProjectConfig は読み込んだ fc.toml。
type ProjectConfig struct {
	Path     string                       // 見つからなければ ""
	Sections map[string]map[string]string // 見出し → キー → 値の綴り (文字列は引用符を外したもの)
}

// findConfig は dir から親へ向かって fc.toml を探す (無ければ Path が "" の空の設定)。
func findConfig(dir string) (*ProjectConfig, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for d := abs; ; d = filepath.Dir(d) {
		p := filepath.Join(d, ConfigName)
		if data, err := os.ReadFile(p); err == nil {
			return parseConfig(p, data)
		}
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	return &ProjectConfig{Sections: map[string]map[string]string{}}, nil
}

// parseConfig は fc.toml の中身を読む。
func parseConfig(path string, data []byte) (*ProjectConfig, error) {
	cfg := &ProjectConfig{Path: path, Sections: map[string]map[string]string{}}
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(stripComment(sc.Text()))
		if line == "" {
			continue
		}
		fail := func(msg string) error {
			return &diag.Error{Msg: fmt.Sprintf("%s:%d: %s", path, n, msg)}
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fail("section header must end with ]")
			}
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "" {
				return nil, fail("empty section name")
			}
			if cfg.Sections[section] == nil {
				cfg.Sections[section] = map[string]string{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fail("expected `key = value`")
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			return nil, fail("expected `key = value`")
		}
		if strings.HasPrefix(v, "\"") {
			if len(v) < 2 || !strings.HasSuffix(v, "\"") {
				return nil, fail("unterminated string")
			}
			v = v[1 : len(v)-1]
		}
		if cfg.Sections[section] == nil {
			cfg.Sections[section] = map[string]string{}
		}
		if _, dup := cfg.Sections[section][k]; dup {
			return nil, fail(fmt.Sprintf("duplicate key %s", k))
		}
		cfg.Sections[section][k] = v
	}
	return cfg, sc.Err()
}

// stripComment は行の `#` から後ろを落とす (文字列の中の # は残す)。
func stripComment(line string) string {
	in := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			in = !in
		case '#':
			if !in {
				return line[:i]
			}
		}
	}
	return line
}

// defines は fc.toml の [define.<module>] と CLI の -D (`module.NAME=value`、後勝ち) から @(build) の const の上書きを作る。
func (cfg *ProjectConfig) defines(cli []string) (map[string]*sema.DefineUse, error) {
	m := map[string]*sema.DefineUse{}
	var sections []string
	for s := range cfg.Sections {
		sections = append(sections, s)
	}
	sort.Strings(sections)
	for _, s := range sections {
		mod, ok := strings.CutPrefix(s, "define.")
		if !ok {
			continue
		}
		for k, v := range cfg.Sections[s] {
			m[mod+"."+k] = &sema.DefineUse{Key: mod + "." + k, Value: v, Source: cfg.Path}
		}
	}
	for _, d := range cli {
		k, v, ok := strings.Cut(d, "=")
		if !ok || !strings.Contains(k, ".") || v == "" {
			return nil, &diag.Error{Msg: fmt.Sprintf("-D %s: expected module.NAME=value", d)}
		}
		m[k] = &sema.DefineUse{Key: k, Value: v, Source: "-D"}
	}
	return m, nil
}

// copyDefines は上書きの表の写し (Used はビルドごとに付け直す)。
func copyDefines(m map[string]*sema.DefineUse) map[string]*sema.DefineUse {
	r := make(map[string]*sema.DefineUse, len(m))
	for k, v := range m {
		c := *v
		c.Used = false
		r[k] = &c
	}
	return r
}

// moduleExists は libPath (dir 相対) のどこかに module.fc があるか。
func moduleExists(dir string, libPath []string, module string) bool {
	for _, p := range libPath {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		if _, err := os.Stat(filepath.Join(p, module+".fc")); err == nil {
			return true
		}
	}
	return false
}
