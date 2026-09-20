package ir

// モジュール・関数・定義 (lib/fc/base.rb の Module / Lambda 由来)。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
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
	AsmSymbols     []string // include したアセンブラファイルが参照するシンボル (volatile と Entry の判定用)
	IncludeHeaders []string
	Uses           []*ModuleInterface // use したモジュール (出現順、重複なし)
	Scope          *Scope
	Seq            int // コンパイラ生成名 (一時変数 $N、ラベル @x_N、無名関数) の連番。モジュール内で閉じる (C5)
	FromFcm        bool
	Depends        []string
	Defs           []*Def
}

func NewModule(id, path string, globalScope *Scope) *Module {
	scope := NewScope(globalScope)
	scope.Owner = id
	return &Module{
		Id:    id,
		Path:  path,
		Scope: scope,
	}
}

// AddUse は use したモジュールを記録する (同じモジュールは 1 回だけ)。
func (m *Module) AddUse(mi *ModuleInterface) {
	for _, u := range m.Uses {
		if u.Id == mi.Id {
			return
		}
	}
	m.Uses = append(m.Uses, mi)
}

// ModuleInterface は importer から見えるモジュールの外面 (doc/archive/v2_plan.md C4)。
// 宣言の検索と識別だけを提供し、Lambda 本体や IR には触れさせない。
// F-mod (分割コンパイル) ではこれをシリアライズしたものが `use` の入力になる。
//
// 可視性は宣言側モジュールの文法バージョンで決まる (doc/v2_grammar.md §3.2, §4.2):
//   - `use * from mod;` (LookupPublic) はどちらのバージョンでも public だけを取り込む
//   - `mod.name` のドット参照 (Lookup) は v1 モジュールでは private にも届き、v2 では public のみ (規則 S7)
type ModuleInterface struct {
	Id    string
	scope *Scope
}

// Interface はこのモジュールの外面を返す。
func (m *Module) Interface() *ModuleInterface {
	return &ModuleInterface{Id: m.Id, scope: m.Scope}
}

// Lookup はドット参照 `mod.name` 用に公開宣言を探す (無ければ nil)。
func (mi *ModuleInterface) Lookup(name string) *Value {
	return mi.scope.Find(name, false)
}

// LookupPublic は `use * from` 用に公開宣言だけを探す (無ければ nil)。
func (mi *ModuleInterface) LookupPublic(name string) *Value {
	return mi.scope.Find(name, false)
}

// LookupInternal はバージョンに関係なく private も含めて探す (コンパイラ組み込み機能の内部参照用。
// 利用者コードの名前解決には使わない)。
func (mi *ModuleInterface) LookupInternal(name string) *Value {
	// 利用者コードの参照ではないので名前解決の観測 (migrate の参照解析) には乗せない
	return mi.scope.withoutTrace(func() *Value { return mi.scope.Find(name, true) })
}

// LookupMust は Lookup と同じだが、見つからなければ diag.Error。
// v2 モジュールの private を指していたら、その旨を伝える。
func (mi *ModuleInterface) LookupMust(name string) *Value {
	if v := mi.Lookup(name); v != nil {
		return v
	}
	if mi.LookupInternal(name) != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s.%s is private (declare it with `public` in module %s)", mi.Id, name, mi.Id)})
	}
	panic(&diag.Error{Msg: fmt.Sprintf("%s not found", name)})
}

// Exports は公開宣言の名前を宣言順に列挙する。
func (mi *ModuleInterface) Exports() []string {
	var r []string
	for _, id := range mi.scope.order {
		if v := mi.scope.declares[id]; v != nil && v.Public {
			r = append(r, id)
		}
	}
	return r
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
	Id        string          // アセンブラシンボル名 (_<module>_<name>、_main、symbol オプション、または $N)
	Name      string          // 宣言名 (関数リテラルでは "")
	Pos       syntax.Position // 宣言の位置 (コード生成時のエラーに使う)
	Params    []Param         // 仮引数の宣言
	Args      []*Value        // 仮引数の変数 (compileLambda で Params から作られる)
	Type      *types.Type
	Options   Options       // options(...) の生の値 (segment, fastcall, symbol, ...)
	Module    *Module       // 宣言したモジュール (far call の判定に使う)
	Extern    bool          // 本体を持たない (宣言のみ)
	Body      *syntax.Block // 関数本体。nil なら extern
	Ops       []*Op         // nil 要素は最適化で削除された命令
	Vars      []*Value
	Bank      int
	Result    *Value
	Defs      []*Def
	Asm       []string
	FrameSize int
	ZpUsed    int // レジスタ割付後: 普通の関数は L の使用バイト数、fastcall は FC_FASTCALL_REG の使用バイト数 (引数・戻り値込み)

	// 呼び出し規約とフレームの配置 (internal/frames が決める。doc/v2_frame_alloc.md §6)
	ABI       ABI
	Entry     bool // static のうち、アドレスを取られた関数 (呼び出し側はスタック経由で渡し、プロローグで自分のフレームに写す)
	Interrupt bool // options(interrupt: true): 割り込みから呼ばれる (フレームは全関数と重ねない)
	Unused    bool // main / 割り込み / asm / 関数ポインタからどう辿っても届かない (出力しない。frames.Analyze が決める)
	FrameZp   bool // static: フレームがゼロページ (FC_SZP) にある
	FrameBase int  // static: 領域内のオフセット (配置後)
	// レジスタ渡し (static だけ。doc/v2_frame_alloc.md §7): RegArg は最後の引数 (1 バイト) を A で受け取る (呼び出し側が
	// A に置いて `sym` / `sym__direct` から入り、入口の `sta` でフレームに写す。フレームに書いた呼び出し (far call、
	// 引数と call の間に他の命令がある) は `sta` の後ろの `sym__frame` から入る)。RegResult は 1 バイトの戻り値を
	// フレームに書いたうえで A にも置いて返す (呼び出し側はフレームを読まない)。RegArgY は最後から 2 つ目の引数 (1 バイト) を
	// Y で受け取る (入口の `sty`。A と両方あるときの入口は `sty; sym__a: sta; sym__frame:` の順で、呼び出し側は
	// レジスタに置けた引数に応じて入る場所を選ぶ)
	RegArg    bool
	RegArgY   bool
	RegResult bool
}

// ABI は関数の呼び出し規約 (doc/v2_frame_alloc.md §6-1)。
type ABI uint8

const (
	ABIStack    ABI = iota // S+n,x のフレーム (再帰、options(abi: "stack")、extern)
	ABIFastcall            // FC_FASTCALL_REG (extern の fastcall)
	ABIStatic              // 固定アドレスのフレーム F_<sym>
	ABICc65                // cc65 の __fastcall__ (extern。最後の = 唯一の引数と戻り値を A / X で渡す。options(abi: "cc65"))
)

var abiNames = [...]string{ABIStack: "stack", ABIFastcall: "fastcall", ABIStatic: "static", ABICc65: "cc65"}

func (a ABI) String() string {
	if int(a) < len(abiNames) {
		return abiNames[a]
	}
	return fmt.Sprintf("ABI(%d)", int(a))
}

// FrameSym は静的フレームのアセンブラシンボル (F + マングルした Id。`_fib_fib` → `F_fib_fib`)。
func (l *Lambda) FrameSym() string { return "F" + Mangle(l.Id) }

// Mangle は名前をアセンブラ用の表現に変更する ($ → _D)。
func Mangle(str string) string { return strings.ReplaceAll(str, "$", "_D") }

// Switchable はこのモジュールが切替バンクに載っているか (`options(bank: N)` で N >= 0。`options(near: true)` なら固定扱い)。
// doc/v2_farcall.md §3.2
func (m *Module) Switchable() bool {
	if m.Options.Has("near") {
		return false
	}
	b, ok := m.Options.Int("bank")
	return ok && b >= 0
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
