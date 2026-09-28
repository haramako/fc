package ir

// 調査用の設定 (最適化のパスの入れ切り、トレース、検証)。
//
// 以前は FC_DISABLE を ir が sync.Once で 1 回だけ読み、FC_TRACE_* / FC_DUMP_IR / FC_VERIFY_REGS を opt / regalloc /
// codegen がそれぞれ os.Getenv で読んでいた。プロセスで 1 つの値しか持てないので、段を切って比べるテスト
// (TestRandomMetamorphic) は fcc を別のプロセスで動かす必要があった。ここでは環境変数を読むのは ConfigFromEnv だけ
// (driver が BuildOptions.Config の既定に使う) で、値はビルドごとに sema.Program → ir.Module に付き、各段は
// lmd.Cfg() で引く。
//
// FC_DISABLE=名前,名前,... で切れる名前: ssa mul indexoff induction unroll devirt autoinline sink fuse fieldindex coalesce
// chain narrow scale commute carry split rotate dup ywalk inline resident func-resident step shift8 fuse-index fnptr-reg
// switch peephole (doc/development_notes.md (7): 退行やバグは切って比べる)。FC_NO_RESIDENT=1 は resident と同じ。
// FC_TRACE_<NAME>=値 は Trace("<name>") で引く (resident / signed / logs / log_id / induction / unroll / pc)。

import (
	"os"
	"strings"
)

// Config は 1 回のビルドの調査用の設定。nil でもメソッドは使える (何も切らない・何も出さない)。
type Config struct {
	disabled   map[string]bool
	trace      map[string]string
	dumpIR     bool // FC_DUMP_IR: 最適化と割付の後の IR を stderr に出す
	verifyRegs bool // FC_VERIFY_REGS: 生成した命令列でレジスタの検査をする (テストと fuzz で有効。codegen/verify.go)
}

// NewConfig は names を切った設定 (テスト用)。
func NewConfig(disabled ...string) *Config {
	c := &Config{disabled: map[string]bool{}, trace: map[string]string{}}
	for _, n := range disabled {
		if n = strings.TrimSpace(n); n != "" {
			c.disabled[n] = true
		}
	}
	return c
}

// ConfigFromEnv は環境変数 (FC_DISABLE / FC_NO_RESIDENT / FC_TRACE_* / FC_DUMP_IR / FC_VERIFY_REGS) から作る。
func ConfigFromEnv() *Config {
	c := NewConfig(strings.Split(os.Getenv("FC_DISABLE"), ",")...)
	if os.Getenv("FC_NO_RESIDENT") != "" {
		c.disabled["resident"] = true
	}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "FC_TRACE_") && v != "" {
			c.trace[strings.ToLower(strings.TrimPrefix(k, "FC_TRACE_"))] = v
		}
	}
	c.dumpIR = os.Getenv("FC_DUMP_IR") != ""
	c.verifyRegs = os.Getenv("FC_VERIFY_REGS") != ""
	return c
}

// DumpIR は最適化と割付の後の IR を stderr に出すか (FC_DUMP_IR)。
func (c *Config) DumpIR() bool { return c != nil && c.dumpIR }

// VerifyRegs は生成した命令列でレジスタの検査をするか (FC_VERIFY_REGS)。
func (c *Config) VerifyRegs() bool { return c != nil && c.verifyRegs }

// SetVerifyRegs はレジスタの検査を入れ切りする (テスト用)。
func (c *Config) SetVerifyRegs(on bool) { c.verifyRegs = on }

// Disabled は name のパス (機能) を切っているか。
func (c *Config) Disabled(name string) bool { return c != nil && c.disabled[name] }

// Trace は FC_TRACE_<NAME> の値 ("" なら出さない)。
func (c *Config) Trace(name string) string {
	if c == nil {
		return ""
	}
	return c.trace[name]
}

// Cfg はモジュールの設定 (nil でも使える)。
func (m *Module) Cfg() *Config {
	if m == nil {
		return nil
	}
	return m.Config
}

// Cfg は関数の設定 (Module の Config。テストで Module の無い関数なら nil)。
func (l *Lambda) Cfg() *Config { return l.Module.Cfg() }

// ModulesCfg はプログラム全体の処理 (インライン展開・直接化) で引く設定 (全モジュールで同じ)。
func ModulesCfg(mods []*Module) *Config {
	if len(mods) == 0 {
		return nil
	}
	return mods[0].Config
}
