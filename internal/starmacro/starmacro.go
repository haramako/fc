// Package starmacro は Starlark で書く定数マクロ (fc.toml の [macro_script.<名前>]。Agent/wiki/plans/external-macros.md)。
// fcc の中で動くので、fcc のほかに何も要らない (外部コマンドの [macro_server.*] は Go などで何でも書ける逃げ道)。
//
// スクリプトの一番上の関数 (`_` で始まらないもの) がそのまま `@名前` のマクロになる。Starlark は外界に触れない (ファイル・時刻・
// 乱数・OS が無い) ので、使えるのは fcc が渡す組み込みだけ:
//
//	read(path)            ファイルの中身 (bytes)。fc.toml のあるディレクトリの中だけ (外へ出るパス・リンクはエラー)
//	glob(pattern)         ファイルの一覧 (名前の順。同じく fc.toml のあるディレクトリの中)
//	math                  Starlark の math モジュール (sin・floor など)
//	fc.array(type, list)  型付きの整数の配列 (type は "u8" / "i8" / "u16" / "i16")
//	fc.int(type, n)       型付きの整数
//	print(...)            標準エラーに出す (デバッグ用)
//	load("x.star", "f")   同じディレクトリ (の下) の別のスクリプト
//
// マクロの戻り値: 整数 (型のない定数)・文字列・bytes ([N]u8)・整数のリスト (全部 0..255 なら [N]u8。それ以外は fc.array で
// 型を書く)・fc.array・fc.int。同じ入力からはいつも同じ結果になるので、結果はビルドの間だけ覚える (ディスクのキャッシュは要らない)。
// 実行の手数は MaxSteps で打ち切る (無限ループ)。
package starmacro

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/haramako/fc/internal/extmacro"
	"go.starlark.net/lib/math"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// MaxSteps は 1 回の読み込み・マクロの呼び出しで実行する Starlark の手数の上限。
var MaxSteps uint64 = 200_000_000

// Script は fc.toml の [macro_script.<Name>] の 1 つ。
type Script struct {
	Name string
	File string // スクリプトのパス (絶対パス)
	Root string // read / glob / load で読めるディレクトリ (fc.toml のあるディレクトリ)
}

// Set は読み込んだスクリプトのマクロの集まり。
type Set struct {
	mu     sync.Mutex
	macros map[string]*macro
	memo   map[string]*extmacro.Result
}

type macro struct {
	script *Script
	fn     starlark.Callable
}

// fileOptions は Starlark の方言 (while・再帰・一番上の if / for を許す。止まらないものは MaxSteps で打ち切る)。
var fileOptions = &syntax.FileOptions{Set: true, While: true, TopLevelControl: true, GlobalReassign: true, Recursion: true}

// Load は scripts を実行して、一番上の関数をマクロにする (同じ名前の関数が 2 つのスクリプトにあればエラー)。
func Load(scripts []*Script) (*Set, error) {
	s := &Set{macros: map[string]*macro{}, memo: map[string]*extmacro.Result{}}
	for _, sc := range scripts {
		globals, err := sc.exec()
		if err != nil {
			return nil, err
		}
		for name, v := range globals {
			fn, ok := v.(starlark.Callable)
			if !ok || strings.HasPrefix(name, "_") {
				continue
			}
			if _, isFunc := v.(*starlark.Function); !isFunc {
				continue // load した組み込みなど
			}
			if o := s.macros[name]; o != nil {
				return nil, fmt.Errorf("macro @%s is defined in both [macro_script.%s] and [macro_script.%s]", name, o.script.Name, sc.Name)
			}
			s.macros[name] = &macro{script: sc, fn: fn}
		}
	}
	return s, nil
}

// Macros はマクロの名前 (`@` を除く) を名前の順に返す。
func (s *Set) Macros() []string {
	var r []string
	for m := range s.macros {
		r = append(r, m)
	}
	sort.Strings(r)
	return r
}

// Call はマクロ name を args (int / string / []byte / []int) で呼ぶ。
func (s *Set) Call(name string, args []any) (*extmacro.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.macros[name]
	if m == nil {
		return nil, fmt.Errorf("unknown macro @%s", name)
	}
	key := name + fmt.Sprintf("\x00%#v", args)
	if r := s.memo[key]; r != nil {
		return r, nil
	}
	sargs := make(starlark.Tuple, len(args))
	for i, a := range args {
		switch x := a.(type) {
		case int:
			sargs[i] = starlark.MakeInt(x)
		case string:
			sargs[i] = starlark.String(x)
		case []byte:
			sargs[i] = starlark.Bytes(x)
		case []int:
			l := make([]starlark.Value, len(x))
			for j, n := range x {
				l[j] = starlark.MakeInt(n)
			}
			sargs[i] = starlark.NewList(l)
		default:
			return nil, fmt.Errorf("argument %d: unsupported value %T", i+1, a)
		}
	}
	th := m.script.thread()
	v, err := starlark.Call(th, m.fn, sargs, nil)
	if err != nil {
		return nil, fmt.Errorf("@%s: %s", name, describe(err))
	}
	r, err := toResult(v)
	if err != nil {
		return nil, fmt.Errorf("@%s: %v", name, err)
	}
	s.memo[key] = r
	return r, nil
}

// describe は Starlark のエラー (スクリプトの中ならその位置の呼び出し履歴つき)。
func describe(err error) string {
	if e, ok := err.(*starlark.EvalError); ok {
		return e.Backtrace()
	}
	return err.Error()
}

// ---------------------------------------------------------------
// 実行
// ---------------------------------------------------------------

func (sc *Script) thread() *starlark.Thread {
	th := &starlark.Thread{
		Name:  sc.Name,
		Print: func(_ *starlark.Thread, msg string) { fmt.Fprintf(os.Stderr, "[macro_script.%s] %s\n", sc.Name, msg) },
	}
	th.SetMaxExecutionSteps(MaxSteps)
	loaded := map[string]starlark.StringDict{}
	th.Load = func(th *starlark.Thread, module string) (starlark.StringDict, error) {
		path, err := sc.resolve(module, filepath.Dir(sc.File))
		if err != nil {
			return nil, err
		}
		if g, ok := loaded[path]; ok {
			if g == nil {
				return nil, fmt.Errorf("load cycle at %s", module)
			}
			return g, nil
		}
		loaded[path] = nil
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		g, err := starlark.ExecFileOptions(fileOptions, th, path, src, sc.predeclared())
		if err != nil {
			return nil, err
		}
		loaded[path] = g
		return g, nil
	}
	return th
}

func (sc *Script) exec() (starlark.StringDict, error) {
	src, err := os.ReadFile(sc.File)
	if err != nil {
		return nil, fmt.Errorf("[macro_script.%s]: %v", sc.Name, err)
	}
	g, err := starlark.ExecFileOptions(fileOptions, sc.thread(), sc.File, src, sc.predeclared())
	if err != nil {
		return nil, fmt.Errorf("[macro_script.%s]: %s", sc.Name, describe(err))
	}
	return g, nil
}

// predeclared はスクリプトに渡す組み込み。
func (sc *Script) predeclared() starlark.StringDict {
	return starlark.StringDict{
		"math": math.Module,
		"read": starlark.NewBuiltin("read", sc.read),
		"glob": starlark.NewBuiltin("glob", sc.glob),
		"fc": &starlarkstruct.Module{Name: "fc", Members: starlark.StringDict{
			"array": starlark.NewBuiltin("fc.array", fcArray),
			"int":   starlark.NewBuiltin("fc.int", fcInt),
		}},
	}
}

// resolve は Root からの相対のパス p (load は from からの相対) を、Root の中の実際のパスにする (外へ出るならエラー)。
func (sc *Script) resolve(p, from string) (string, error) {
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%q: use a path relative to the project (fc.toml's directory)", p)
	}
	full := filepath.Join(from, filepath.FromSlash(p))
	if !sc.inside(full) {
		return "", fmt.Errorf("%q is outside the project (fc.toml's directory)", p)
	}
	if real, err := filepath.EvalSymlinks(full); err == nil && !sc.inside(real) {
		return "", fmt.Errorf("%q links outside the project", p)
	}
	return full, nil
}

func (sc *Script) inside(p string) bool {
	root := sc.Root
	if r, err := filepath.EvalSymlinks(root); err == nil && !strings.HasPrefix(p, root) {
		root = r
	}
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (sc *Script) read(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &p); err != nil {
		return nil, err
	}
	full, err := sc.resolve(p, sc.Root)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}
	return starlark.Bytes(data), nil
}

func (sc *Script) glob(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &p); err != nil {
		return nil, err
	}
	if _, err := sc.resolve(p, sc.Root); err != nil {
		return nil, fmt.Errorf("glob: %v", err)
	}
	ms, err := filepath.Glob(filepath.Join(sc.Root, filepath.FromSlash(p)))
	if err != nil {
		return nil, fmt.Errorf("glob: %v", err)
	}
	sort.Strings(ms)
	var r []starlark.Value
	for _, m := range ms {
		if rel, err := filepath.Rel(sc.Root, m); err == nil && sc.inside(m) {
			r = append(r, starlark.String(filepath.ToSlash(rel)))
		}
	}
	return starlark.NewList(r), nil
}

// ---------------------------------------------------------------
// 結果
// ---------------------------------------------------------------

// typedValue は fc.array / fc.int の値。
type typedValue struct {
	typ  string
	data []int
	n    int
	arr  bool
}

func (t *typedValue) String() string {
	if t.arr {
		return fmt.Sprintf("fc.array(%q, %v)", t.typ, t.data)
	}
	return fmt.Sprintf("fc.int(%q, %d)", t.typ, t.n)
}
func (t *typedValue) Type() string          { return "fc.typed" }
func (t *typedValue) Freeze()               {}
func (t *typedValue) Truth() starlark.Bool  { return true }
func (t *typedValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", t.Type()) }

func fcArray(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var typ string
	var list starlark.Iterable
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 2, &typ, &list); err != nil {
		return nil, err
	}
	data, err := intList(list)
	if err != nil {
		return nil, fmt.Errorf("fc.array: %v", err)
	}
	r := &typedValue{typ: typ, data: data, arr: true}
	if err := (&extmacro.Result{Kind: "data", Type: typ, Data: data}).Validate(); err != nil {
		return nil, fmt.Errorf("fc.array: %v", err)
	}
	return r, nil
}

func fcInt(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var typ string
	var n int
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 2, &typ, &n); err != nil {
		return nil, err
	}
	if typ == "" {
		return nil, fmt.Errorf("fc.int: the type is empty")
	}
	if err := (&extmacro.Result{Kind: "int", Type: typ, Int: n}).Validate(); err != nil {
		return nil, fmt.Errorf("fc.int: %v", err)
	}
	return &typedValue{typ: typ, n: n}, nil
}

func intList(it starlark.Iterable) ([]int, error) {
	var r []int
	iter := it.Iterate()
	defer iter.Done()
	var x starlark.Value
	for iter.Next(&x) {
		n, err := starlark.AsInt32(x)
		if err != nil {
			return nil, fmt.Errorf("element %d: %v", len(r), err)
		}
		r = append(r, n)
	}
	return r, nil
}

// toResult はマクロの戻り値を結果にする。
func toResult(v starlark.Value) (*extmacro.Result, error) {
	var r *extmacro.Result
	switch x := v.(type) {
	case *typedValue:
		if x.arr {
			r = &extmacro.Result{Kind: "data", Type: x.typ, Data: x.data}
		} else {
			r = &extmacro.Result{Kind: "int", Type: x.typ, Int: x.n}
		}
	case starlark.Int:
		n, err := starlark.AsInt32(x)
		if err != nil {
			return nil, fmt.Errorf("the result %s is too large", x)
		}
		r = &extmacro.Result{Kind: "int", Int: n}
	case starlark.String:
		r = &extmacro.Result{Kind: "string", Str: string(x)}
	case starlark.Bytes:
		r = &extmacro.Result{Kind: "bytes", Bytes: []byte(x)}
	case *starlark.List, starlark.Tuple:
		data, err := intList(x.(starlark.Iterable))
		if err != nil {
			return nil, err
		}
		for _, n := range data {
			if n < 0 || n > 255 {
				return nil, fmt.Errorf("a list result with %d is not a byte array: write fc.array(\"i8\" / \"u16\" / \"i16\", list)", n)
			}
		}
		r = &extmacro.Result{Kind: "data", Type: "u8", Data: data}
	default:
		return nil, fmt.Errorf("the result must be an int, string, bytes, list of ints, fc.array or fc.int (got %s)", v.Type())
	}
	return r, r.Validate()
}
