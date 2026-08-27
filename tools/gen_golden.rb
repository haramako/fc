# coding: utf-8
#
# Ruby版(オラクル)から golden データ一式を生成する。
#   ruby tools/gen_golden.rb
# リポジトリルートから実行すること。testdata/golden/ 以下に出力する。
#
# 生成物:
#   ast/test/<name>.ast|.pos      test/*.fc (errors.fc除く) のパース直後AST
#   ast/fclib/**/<name>.ast|.pos  fclib/**/*.fc (x6502除く)
#   ir/<name>.ir                  HLC完了直後のIR
#   allocir/<name>.air            レジスタ割付+delete_unuse直後のIR
#   asm/<name>/<mod>.s|.inc       正規化済みアセンブラ(IRコメント行除去)
#   asm/test_basic_nes/...        NESターゲットのビルド(1本のみ)
#   bin/<name>.bin                emuターゲットのバイナリ
#   bin/test_basic_nes.nes        NESターゲットのバイナリ
#   stdout/<name>.txt|.exit       実行時出力と終了コード
#
require 'fileutils'
require 'stringio'
require 'pathname'

ROOT = Pathname(File.expand_path('../..', __FILE__))
$LOAD_PATH << (ROOT + 'lib').to_s

require 'fc/compiler'
require_relative 'dumper'

GOLDEN = ROOT + 'testdata/golden'

def write_golden( rel, content )
  path = GOLDEN + rel
  FileUtils.mkdir_p( path.dirname )
  if content.encoding == Encoding::BINARY
    File.binwrite( path, content )
  else
    File.write( path, content )
  end
end

# ---------------------------------------------------------------
# AST golden
# ---------------------------------------------------------------

ast_sources = []
Dir.glob( (ROOT+'test/*.fc').to_s ).sort.each do |f|
  next if File.basename(f) == 'errors.fc' # コンパイル失敗が正の断片集なので除外
  ast_sources << [f, "ast/test/#{File.basename(f,'.fc')}"]
end
Dir.glob( (ROOT+'fclib/**/*.fc').to_s ).sort.each do |f|
  rel = Pathname(f).relative_path_from(ROOT+'fclib').to_s
  next if rel.start_with?('x6502') # スコープ外
  ast_sources << [f, "ast/fclib/#{rel.sub(/\.fc\z/,'')}"]
end

ast_sources.each do |src, out|
  puts "ast: #{src}"
  ast_str, pos_str = GoldenDump.dump_ast_file( src )
  write_golden( out + '.ast', ast_str )
  write_golden( out + '.pos', pos_str )
end

# ---------------------------------------------------------------
# ビルド golden (ir / allocir / asm / bin / stdout)
# ---------------------------------------------------------------

# LIB_PATHはビルドごとに増える(compileがfclib/<target>を追加する)ので、都度復元する
ORIG_LIB_PATH = Fc::LIB_PATH.dup

def build_golden( name, target: 'emu', run: true )
  Fc::LIB_PATH.replace( ORIG_LIB_PATH.dup )
  col = GoldenDump::Collector.new
  GoldenDump.collector = col

  out = StringIO.new
  code = nil
  Dir.chdir( (ROOT+'test').to_s ) do
    compiler = Fc::Compiler.new
    code = compiler.build( "#{name}.fc", { target: target, run: run, stdout: out } )
  end
  GoldenDump.collector = nil

  suffix = target == 'nes' ? '_nes' : ''
  key = name + suffix

  # ir / allocir
  write_golden( "ir/#{key}.ir", col.ir )
  write_golden( "allocir/#{key}.air", col.allocs.join )

  # asm (正規化済み)
  col.hlc.modules.each do |id, mod|
    ['s','inc'].each do |ext|
      text = File.read( (ROOT+"test/.fc-build/_#{id}.#{ext}").to_s, encoding: 'utf-8' )
      write_golden( "asm/#{key}/#{id}.#{ext}", GoldenDump.normalize_asm(text) + "\n" )
    end
  end

  # bin
  binname = target == 'nes' ? 'a.nes' : 'a.bin'
  ext = target == 'nes' ? 'nes' : 'bin'
  write_golden( "bin/#{key}.#{ext}", File.binread( (ROOT+"test/#{binname}").to_s ) )

  # stdout / exit code
  if run
    write_golden( "stdout/#{key}.txt", out.string )
    write_golden( "stdout/#{key}.exit", "#{code}\n" )
    raise "#{name} exited with #{code}" if code != 0
  end
end

tests = Dir.glob( (ROOT+'test/test_*.fc').to_s ).sort.map{|f| File.basename(f, '.fc') }
tests.each do |name|
  puts "build: #{name}"
  build_golden( name )
end

# NESターゲット経路のgolden(ビルドのみ、実行はしない)
puts "build: test_basic (nes)"
build_golden( 'test_basic', target: 'nes', run: false )

puts "done. golden generated at #{GOLDEN}"
