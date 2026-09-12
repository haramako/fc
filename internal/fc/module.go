package fc

// モジュール・関数・定義 (lib/fc/base.rb の Module / Lambda 由来)。

import (
	"fmt"
	"strconv"

	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// Def はモジュール / 関数に属するアセンブラレベルの定義。Kind ごとに使うフィールドが決まる。
type Def struct {
	Sym  string // アセンブラシンボル名
	Kind DefKind
	Type *types.Type

	Equ     *Value    // DefEqu: 整数または関数シンボルのリテラル
	Segment string    // DefBss: 配置セグメント ("" なら既定の BSS)
	Elems   []Operand // DefBlock: 配列の要素
	Lambda  *Lambda   // DefCode
}

// OptionKind は OptionValue の種類。
type OptionKind uint8

const (
	OptInt OptionKind = iota + 1
	OptStr
	OptIdent
)

// OptionValue は options(...) の 1 つの値。整数 / 文字列リテラル / 識別子のいずれか。
type OptionValue struct {
	Kind OptionKind
	Int  int
	Str  string // OptStr の文字列、または OptIdent の識別子名
}

// Text は識別子/文字列としての表記 (整数なら 10 進)。旧実装の to_s 相当。
func (o OptionValue) Text() string {
	if o.Kind == OptInt {
		return strconv.Itoa(o.Int)
	}
	return o.Str
}

// Option は options(...) の 1 エントリ。
type Option struct {
	Key   string
	Value OptionValue
}

// Options は options(...) の順序付きリスト。重複キーは後勝ち (位置は最初のもの)。
type Options []Option

// Set はキーの値を設定する。既存キーは位置を保って上書きする。
func (o *Options) Set(key string, v OptionValue) {
	for i := range *o {
		if (*o)[i].Key == key {
			(*o)[i].Value = v
			return
		}
	}
	*o = append(*o, Option{Key: key, Value: v})
}

// Get はキーの値を返す。
func (o Options) Get(key string) (OptionValue, bool) {
	for _, e := range o {
		if e.Key == key {
			return e.Value, true
		}
	}
	return OptionValue{}, false
}

// Int はキーの値が整数ならそれを返す。
func (o Options) Int(key string) (int, bool) {
	v, ok := o.Get(key)
	if !ok || v.Kind != OptInt {
		return 0, false
	}
	return v.Int, true
}

// Has はキーが存在するかを返す。
func (o Options) Has(key string) bool {
	_, ok := o.Get(key)
	return ok
}

// Module は 1 ソースファイルに対応するコンパイル単位。
type Module struct {
	Id             string
	Path           string
	Vars           []*Value
	Lambdas        []*Lambda
	Options        Options // options(...) 文で設定されたモジュール属性 (bank, org, ...)。値は定数評価済み
	IncludeChrs    []string
	IncludeAsms    []string
	IncludeHeaders []string
	Modules        *ModuleList // use したモジュール
	Scope          *Scope
	CurrentPublic  bool // public: / private: ラベルの現在値
	FromFcm        bool
	Depends        []string
	Defs           []*Def
}

func NewModule(id, path string, globalScope *Scope) *Module {
	return &Module{
		Id:            id,
		Path:          path,
		Modules:       NewModuleList(),
		Scope:         NewScope(globalScope),
		CurrentPublic: true,
	}
}

// ModuleList は登録順を保つモジュールの集合 (id で検索できる)。
type ModuleList struct {
	list []*Module
	byId map[string]*Module
}

func NewModuleList() *ModuleList {
	return &ModuleList{byId: map[string]*Module{}}
}

// Add はモジュールを登録する (同じ id が既にあれば何もしない)。
func (l *ModuleList) Add(m *Module) {
	if _, ok := l.byId[m.Id]; ok {
		return
	}
	l.byId[m.Id] = m
	l.list = append(l.list, m)
}

// Get は id のモジュールを返す。
func (l *ModuleList) Get(id string) (*Module, bool) {
	m, ok := l.byId[id]
	return m, ok
}

// List は登録順のモジュール一覧。
func (l *ModuleList) List() []*Module { return l.list }

// Param は関数の仮引数 (名前と型)。
type Param struct {
	Name string
	Type *types.Type
}

// Lambda は関数 (宣言された関数、または関数リテラル)。
type Lambda struct {
	Id        string   // アセンブラシンボル名 (_<module>_<name>、_main、symbol オプション、または $N)
	Name      string   // 宣言名 (関数リテラルでは "")
	Params    []Param  // 仮引数の宣言
	Args      []*Value // 仮引数の変数 (compileLambda で Params から作られる)
	Type      *types.Type
	Options   Options       // options(...) の生の値 (segment, fastcall, symbol, ...)
	Extern    bool          // 本体を持たない (宣言のみ)
	Body      *syntax.Block // 関数本体。nil なら extern
	Ops       []*Op         // nil 要素は最適化で削除された命令
	Vars      []*Value
	Bank      int
	Result    *Value
	Defs      []*Def
	Asm       []string
	FrameSize int
}

// Segment は配置セグメント (options(segment:...))。"" なら既定。
func (l *Lambda) Segment() string {
	if v, ok := l.Options.Get("segment"); ok {
		return v.Text()
	}
	return ""
}

// String は "<Lambda:id type>" (エラーメッセージで使用)。
func (l *Lambda) String() string {
	return fmt.Sprintf("<Lambda:%s %s>", l.Id, l.Type)
}
