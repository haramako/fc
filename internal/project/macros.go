package project

// 外部コマンドの定数マクロの宣言 (fc.toml の [macro_server.<name>]。internal/extmacro、Agent/wiki/plans/external-macros.md):
//
//	[macro_server.tools]
//	command = ["go", "run", "./tools/fcmacros"]   # 作業ディレクトリは fc.toml のあるディレクトリ
//	macros = ["sin_table", "font_map"]            # 提供するマクロ (ソースでは @sin_table(...) と呼ぶ)
//	inputs = ["tools/fcmacros/*.go"]              # 省略可: コマンド自身のソース (書くとディスクにキャッシュする)

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/extmacro"
)

// MacroServers は fc.toml の [macro_server.*] (名前の順)。
func (cfg *ProjectConfig) MacroServers() ([]*extmacro.Server, error) {
	var names []string
	for _, sec := range cfg.Order {
		if strings.HasPrefix(sec, "macro_server.") {
			names = append(names, sec)
		}
	}
	sort.Strings(names)
	var r []*extmacro.Server
	for _, sec := range names {
		fail := func(msg string) error { return fmt.Errorf("%s: [%s]: %s", cfg.Path, sec, msg) }
		kv := cfg.Sections[sec]
		s := &extmacro.Server{Name: strings.TrimPrefix(sec, "macro_server."), Dir: filepath.Dir(cfg.Path)}
		var err error
		if s.Command, err = StringList(kv["command"]); err != nil || len(s.Command) == 0 {
			return nil, fail("command must be a non-empty array of strings (command = [\"go\", \"run\", \"./tools/fcmacros\"])")
		}
		if s.Macros, err = StringList(kv["macros"]); err != nil || len(s.Macros) == 0 {
			return nil, fail("macros must be a non-empty array of macro names (macros = [\"sin_table\"])")
		}
		for _, m := range s.Macros {
			if !isMacroName(m) {
				return nil, fail(fmt.Sprintf("bad macro name %q (write it without @: letters, digits and _)", m))
			}
		}
		if v, ok := kv["inputs"]; ok {
			if s.Inputs, err = StringList(v); err != nil {
				return nil, fail("inputs must be an array of glob patterns")
			}
		}
		r = append(r, s)
	}
	return r, nil
}

func isMacroName(s string) bool {
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for _, c := range s {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// StringList は 1 行の文字列の配列の値 `["a", "b"]` を読む。
func StringList(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, fmt.Errorf("expected an array [\"...\", ...]")
	}
	body := strings.TrimSpace(v[1 : len(v)-1])
	var r []string
	for body != "" {
		if body[0] != '"' {
			return nil, fmt.Errorf("expected a string in the array")
		}
		end := 1
		for end < len(body) && body[end] != '"' {
			if body[end] == '\\' {
				end++
			}
			end++
		}
		if end >= len(body) {
			return nil, fmt.Errorf("unterminated string in the array")
		}
		s, err := strconv.Unquote(body[:end+1])
		if err != nil {
			return nil, err
		}
		r = append(r, s)
		body = strings.TrimSpace(body[end+1:])
		if body == "" {
			break
		}
		if body[0] != ',' {
			return nil, fmt.Errorf("expected , between strings")
		}
		body = strings.TrimSpace(body[1:])
	}
	return r, nil
}
