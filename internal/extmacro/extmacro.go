// Package extmacro は外部コマンドの定数マクロ (Agent/wiki/plans/external-macros.md)。fc.toml の [macro_server.<名前>] に
// 書いたコマンドを、最初に使ったときに起動してビルドの間だけ生かしておき、改行区切りの JSON (1 行に 1 つ) で要求と応答を
// やり取りする。マクロは引数 (定数) を受け取って定数 (整数・整数の配列・バイト列・文字列) を返すだけ。
//
// プロトコル:
//
//	起動直後 (サーバー → fcc): {"macros": ["sin_table", ...]}
//	要求 (fcc → サーバー):     {"id": 1, "macro": "sin_table", "args": [256, 64]}
//	応答 (サーバー → fcc):     {"id": 1, "result": {"type": "i8", "data": [0, 1, ...]}, "deps": ["font.txt"]}
//	                           {"id": 1, "error": "..."}
//
// 引数は整数・文字列・バイト列 ({"bytes": "<base64>"})・整数の配列。結果は {"int": n} / {"type": "i8", "data": [...]} /
// {"bytes": "<base64>"} / {"string": "..."} のどれか (type は u8 / i8 / u16 / i16。int に付ければ型付きの定数)。deps は
// マクロが読んだファイル (作業ディレクトリ = fc.toml のあるディレクトリからの相対)。マクロのログは stderr (fcc がそのまま流す)。
//
// キャッシュ: 同じビルドの中の同じ要求はメモリで覚える。fc.toml に inputs (マクロのコマンド自身のソースの glob) を書いたときだけ、
// 「コマンド・inputs の中身・マクロ名・引数」→ 結果と deps の中身のハッシュを CacheDir に置き、次のビルドで deps が変わって
// いなければそれを使う (全部当たればプロセスを起動しない)。inputs が無いとコマンドが変わったことが分からないので置かない。
package extmacro

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Server は fc.toml の [macro_server.<Name>] の 1 つ。
type Server struct {
	Name    string
	Command []string // 起動するコマンドと引数
	Dir     string   // 作業ディレクトリ (fc.toml のあるディレクトリ)
	Macros  []string // 提供するマクロの名前 (`@` を除く。起動しなくても名前が分かるように fc.toml に書く)
	Inputs  []string // コマンド自身のソースの glob (Dir からの相対。ディスクのキャッシュの鍵に入れる)
}

// Result はマクロの結果。Kind で中身が決まる。
type Result struct {
	Kind  string // "int" / "data" / "bytes" / "string"
	Type  string // 整数の型 (u8 / i8 / u16 / i16)。int では "" なら型のない定数、data では必須
	Int   int
	Data  []int
	Bytes []byte
	Str   string
}

// StartTimeout / CallTimeout は起動のあいさつと 1 つの要求の応答を待つ時間 (`go run` は初回のビルドで時間がかかる)。
var (
	StartTimeout = 2 * time.Minute
	CallTimeout  = 2 * time.Minute
)

// Pool はビルドの間のサーバーの集まり。Close で全部止める。
type Pool struct {
	CacheDir string // ディスクのキャッシュを置くディレクトリ ("" なら置かない)
	servers  map[string]*Server
	byMacro  map[string]*Server
	procs    map[string]*proc
	memo     map[string]*Result
	mu       sync.Mutex
}

// NewPool は servers のマクロの表を作る (マクロ名の重複はエラー)。プロセスはまだ起動しない。
func NewPool(servers []*Server, cacheDir string) (*Pool, error) {
	p := &Pool{CacheDir: cacheDir, servers: map[string]*Server{}, byMacro: map[string]*Server{}, procs: map[string]*proc{}, memo: map[string]*Result{}}
	for _, s := range servers {
		p.servers[s.Name] = s
		for _, m := range s.Macros {
			if o := p.byMacro[m]; o != nil {
				return nil, fmt.Errorf("macro @%s is provided by both [macro_server.%s] and [macro_server.%s]", m, o.Name, s.Name)
			}
			p.byMacro[m] = s
		}
	}
	return p, nil
}

// Macros はマクロの名前 (`@` を除く) を名前の順に返す。
func (p *Pool) Macros() []string {
	var r []string
	for m := range p.byMacro {
		r = append(r, m)
	}
	sort.Strings(r)
	return r
}

// Call はマクロ name を args で呼ぶ。args の要素は int / string / []byte / []int。
func (p *Pool) Call(name string, args []any) (*Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.byMacro[name]
	if s == nil {
		return nil, fmt.Errorf("unknown external macro @%s", name)
	}
	jargs, err := encodeArgs(args)
	if err != nil {
		return nil, err
	}
	argText, _ := json.Marshal(jargs)
	key := name + "\x00" + string(argText)
	if r := p.memo[key]; r != nil {
		return r, nil
	}
	diskKey := ""
	if p.CacheDir != "" && len(s.Inputs) > 0 {
		if h, err := s.inputsHash(); err == nil {
			sum := sha256.Sum256([]byte(strings.Join(s.Command, "\x00") + "\x00" + h + "\x00" + key))
			diskKey = hex.EncodeToString(sum[:])
			if r := p.readCache(s, diskKey); r != nil {
				p.memo[key] = r
				return r, nil
			}
		}
	}
	pr, err := p.start(s)
	if err != nil {
		return nil, err
	}
	resp, err := pr.call(name, jargs)
	if err != nil {
		return nil, fmt.Errorf("[macro_server.%s] @%s: %w", s.Name, name, err)
	}
	r, err := decodeResult(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("[macro_server.%s] @%s: %w", s.Name, name, err)
	}
	p.memo[key] = r
	if diskKey != "" {
		p.writeCache(s, diskKey, resp)
	}
	return r, nil
}

// Close は起動したサーバーを止める (標準入力を閉じて終わるのを少し待ち、終わらなければ止める)。
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pr := range p.procs {
		pr.stop()
	}
	p.procs = map[string]*proc{}
}

// ---------------------------------------------------------------
// プロセス
// ---------------------------------------------------------------

type proc struct {
	srv    *Server
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	done   chan struct{} // 標準出力が閉じた
	quit   chan struct{} // stop が閉じる (読む側が止まっても読み取りの goroutine が残らないように)
	nextID int
	dead   error
}

type response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
	Deps   []string        `json:"deps"`
}

func (p *Pool) start(s *Server) (*proc, error) {
	if pr := p.procs[s.Name]; pr != nil {
		if pr.dead != nil {
			return nil, pr.dead
		}
		return pr, nil
	}
	if len(s.Command) == 0 {
		return nil, fmt.Errorf("[macro_server.%s]: command is empty", s.Name)
	}
	cmd := exec.Command(s.Command[0], s.Command[1:]...)
	cmd.Dir = s.Dir
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("[macro_server.%s]: cannot start %q: %w", s.Name, strings.Join(s.Command, " "), err)
	}
	pr := &proc{srv: s, cmd: cmd, stdin: stdin, lines: make(chan string, 16), done: make(chan struct{}), quit: make(chan struct{})}
	p.procs[s.Name] = pr
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 64<<20) // 大きな表 (1 行の JSON)
		defer close(pr.done)
		for sc.Scan() {
			select {
			case pr.lines <- sc.Text():
			case <-pr.quit:
				return
			}
		}
	}()
	line, err := pr.read(StartTimeout)
	if err != nil {
		pr.dead = fmt.Errorf("[macro_server.%s]: no greeting (`{\"macros\": [...]}`) from %q: %w", s.Name, strings.Join(s.Command, " "), err)
		pr.stop()
		return nil, pr.dead
	}
	var hello struct {
		Macros []string `json:"macros"`
	}
	if err := json.Unmarshal([]byte(line), &hello); err != nil || hello.Macros == nil {
		pr.dead = fmt.Errorf("[macro_server.%s]: the first line must be `{\"macros\": [...]}` (got %q)", s.Name, clip(line))
		pr.stop()
		return nil, pr.dead
	}
	have := map[string]bool{}
	for _, m := range hello.Macros {
		have[m] = true
	}
	for _, m := range s.Macros {
		if !have[m] {
			pr.dead = fmt.Errorf("[macro_server.%s]: fc.toml lists @%s but the server provides %v", s.Name, m, hello.Macros)
			pr.stop()
			return nil, pr.dead
		}
	}
	return pr, nil
}

// read は 1 行を待つ (プロセスが終わった・時間切れならエラー)。
func (pr *proc) read(timeout time.Duration) (string, error) {
	select {
	case l := <-pr.lines:
		return l, nil
	case <-pr.done:
		select {
		case l := <-pr.lines:
			return l, nil
		default:
		}
		return "", fmt.Errorf("the process exited")
	case <-time.After(timeout):
		return "", fmt.Errorf("timed out after %s", timeout)
	}
}

func (pr *proc) call(name string, args []any) (*response, error) {
	if pr.dead != nil {
		return nil, pr.dead
	}
	pr.nextID++
	id := pr.nextID
	req, _ := json.Marshal(map[string]any{"id": id, "macro": name, "args": args})
	if _, err := pr.stdin.Write(append(req, '\n')); err != nil {
		pr.dead = fmt.Errorf("cannot write the request: %w", err)
		return nil, pr.dead
	}
	for {
		line, err := pr.read(CallTimeout)
		if err != nil {
			pr.dead = err
			return nil, err
		}
		var r response
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			pr.dead = fmt.Errorf("bad response %q: %v", clip(line), err)
			return nil, pr.dead
		}
		if r.ID != id {
			continue // 前の要求の応答 (今は 1 つずつ送るので来ない)
		}
		if r.Error != "" {
			return nil, fmt.Errorf("%s", r.Error)
		}
		return &r, nil
	}
}

func (pr *proc) stop() {
	select {
	case <-pr.quit:
		return // 止めた
	default:
	}
	close(pr.quit)
	_ = pr.stdin.Close()
	exited := make(chan struct{})
	go func() { _ = pr.cmd.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		_ = pr.cmd.Process.Kill()
		<-exited
	}
}

func clip(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// ---------------------------------------------------------------
// 引数と結果
// ---------------------------------------------------------------

func encodeArgs(args []any) ([]any, error) {
	r := make([]any, len(args))
	for i, a := range args {
		switch x := a.(type) {
		case int, string, []int:
			r[i] = x
		case []byte:
			r[i] = map[string]string{"bytes": base64.StdEncoding.EncodeToString(x)}
		default:
			return nil, fmt.Errorf("argument %d: unsupported value %T", i+1, a)
		}
	}
	return r, nil
}

// intRange は結果の整数の型の範囲。
var intRange = map[string][2]int{"u8": {0, 255}, "i8": {-128, 127}, "u16": {0, 65535}, "i16": {-32768, 32767}}

func decodeResult(raw json.RawMessage) (*Result, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("the response has no result")
	}
	var x struct {
		Type   string  `json:"type"`
		Int    *int    `json:"int"`
		Data   []int   `json:"data"`
		Bytes  *string `json:"bytes"`
		String *string `json:"string"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&x); err != nil {
		return nil, fmt.Errorf("bad result %s: %v", clip(string(raw)), err)
	}
	n := 0
	for _, set := range []bool{x.Int != nil, x.Data != nil, x.Bytes != nil, x.String != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return nil, fmt.Errorf("the result must have exactly one of int / data / bytes / string (got %s)", clip(string(raw)))
	}
	rg, typed := intRange[x.Type]
	if x.Type != "" && !typed {
		return nil, fmt.Errorf("unknown result type %q (u8 / i8 / u16 / i16)", x.Type)
	}
	switch {
	case x.Int != nil:
		if typed && (*x.Int < rg[0] || *x.Int > rg[1]) {
			return nil, fmt.Errorf("the result %d does not fit in %s", *x.Int, x.Type)
		}
		return &Result{Kind: "int", Type: x.Type, Int: *x.Int}, nil
	case x.Data != nil:
		if !typed {
			return nil, fmt.Errorf("an integer array result needs \"type\" (u8 / i8 / u16 / i16)")
		}
		for i, v := range x.Data {
			if v < rg[0] || v > rg[1] {
				return nil, fmt.Errorf("element %d of the result (%d) does not fit in %s", i, v, x.Type)
			}
		}
		return &Result{Kind: "data", Type: x.Type, Data: x.Data}, nil
	case x.Bytes != nil:
		b, err := base64.StdEncoding.DecodeString(*x.Bytes)
		if err != nil {
			return nil, fmt.Errorf("bad base64 in the result: %v", err)
		}
		return &Result{Kind: "bytes", Bytes: b}, nil
	}
	return &Result{Kind: "string", Str: *x.String}, nil
}

// ---------------------------------------------------------------
// ディスクのキャッシュ
// ---------------------------------------------------------------

type cacheEntry struct {
	Result json.RawMessage   `json:"result"`
	Deps   map[string]string `json:"deps"` // ファイル (Dir からの相対) → 中身のハッシュ
}

func (s *Server) inputsHash() (string, error) {
	var files []string
	for _, g := range s.Inputs {
		m, err := filepath.Glob(filepath.Join(s.Dir, g))
		if err != nil {
			return "", err
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(f), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// path は Dir からの相対のパス f (絶対パスならそのまま)。
func (s *Server) path(f string) string {
	if filepath.IsAbs(f) {
		return f
	}
	return filepath.Join(s.Dir, f)
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (p *Pool) cachePath(key string) string { return filepath.Join(p.CacheDir, "macro-"+key+".json") }

func (p *Pool) readCache(s *Server, key string) *Result {
	b, err := os.ReadFile(p.cachePath(key))
	if err != nil {
		return nil
	}
	var e cacheEntry
	if json.Unmarshal(b, &e) != nil {
		return nil
	}
	for f, want := range e.Deps {
		if got, err := fileHash(s.path(f)); err != nil || got != want {
			return nil
		}
	}
	r, err := decodeResult(e.Result)
	if err != nil {
		return nil
	}
	return r
}

func (p *Pool) writeCache(s *Server, key string, resp *response) {
	e := cacheEntry{Result: resp.Result, Deps: map[string]string{}}
	for _, f := range resp.Deps {
		h, err := fileHash(s.path(f))
		if err != nil {
			return // 読めない deps は確かめられないので置かない
		}
		e.Deps[f] = h
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	if os.MkdirAll(p.CacheDir, 0o777) == nil {
		_ = os.WriteFile(p.cachePath(key), b, 0o666)
	}
}
