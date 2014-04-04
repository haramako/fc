# -*- coding: utf-8 -*-
require 'rbconfig'
require 'fileutils'
require 'open3'
require 'fc/hlc'
require 'fc/llc'
require 'r6502'

class R6502::Cpu
  def step_silent
    instr, mode = instr_mode( mem.get(pc) )
    arg = decode_arg( mode, mem.get(pc+1), mem.get(pc+2) )
    method( instr ).call( arg, mode )
  end
end

module Fc

  class CommandError < RuntimeError
    attr_reader :command, :result
    def initialize( message, command, result )
      super message
      @command = command
      @result = result
    end
  end

  class Compiler

    if /mswin(?!ce)|mingw|cygwin|bccwin/ === RbConfig::CONFIG['target_os']
      CA65 = 'ca65'
      LD65 = 'ld65'
    else
      CA65 = 'ca65'
      LD65 = 'ld65'
    end

    def initialize
      @debug = Fc::DEBUG_LEVEL
    end

    def dout( level, *args )
      #:nocov:
      if @debug >= level
        puts args.join(" ")
      end
      #:nocov:
    end

    def find_share( filename )
      dir = FC_HOME + 'share'
      return dir + @target + filename if File.exists? dir + @target + filename
      return dir + filename if File.exists? dir + filename
      raise "file '#{filename}' not found in share directories"
    end

    def build( filename, opt = Hash.new )
      opt = {target:'emu'}.merge(opt)
      if opt[:out].nil?
        if opt[:target] == 'nes'
          opt[:out] = 'a.nes'
        else
          opt[:out] = 'a.bin'
        end
      end
      @target = opt[:target]
      opt[:out] = Pathname.new(opt[:out])
      opt[:html] = opt[:out].sub_ext('.html')
      opt[:stdout] ||= STDOUT
        

      FileUtils.mkdir_p( BUILD_PATH )
      hlc = compile( filename, opt )
      
      output_html hlc, opt[:html] if opt[:debug_info]
      
      compile2( hlc, opt )
      objs = assemble( hlc, opt )

      make_runtime opt[:target]
      exit if opt[:compile_only]

      make_base hlc

      link hlc, objs, opt

      output_html hlc, opt[:html] if opt[:debug_info]
      
      execute opt[:out], opt[:stdout] if opt[:run]

    rescue CommandError => err
      raise Fc::CompileError.new( err.message + "\n" + err.command.to_s + "\n" + err.result )
    end

    def output_html( hlc, filename )
      html = HtmlOutput.new.module_to_html( hlc.modules )
      IO.write filename, html
    end

    def compile_only( filename, opt = Hash.new )
      opt[:compile_only] = true
      build( filename, opt )
    end

    def execute( filename, out )
      # 実行する
      case @target
      when 'emu'
        require 'r6502'
        start_addr = 0x1000
        mem = R6502::Memory.new
        IO.binread(filename).each_byte.with_index do |b,i|
          mem.set start_addr+i, b
        end
        cpu = R6502::Cpu.new(mem)
        cpu.pc = start_addr
        mem.set 0xffff, 255
        mem.set 0xfffe, 255
        while mem.get(0xffff) == 255
          cpu.step_silent
          if mem.get(0xfffe) != 255
            if mem.get(0xfffe) == 1
              addr = mem.get(0xfff0) + (mem.get(0xfff1) << 8)
              str = ''
              while mem.get(addr) != 0
                str += mem.get(addr).chr
                addr += 1
              end
              out.print str
            else
              num = mem.get(0xfff2) + (mem.get(0xfff3) << 8)
              out.print num
            end
            mem.set 0xfffe, 255
          end
        end
        result = mem.get(0xffff)
        
        # emu = Fc::FC_HOME + 'bin/emu6502'
        # result = `node #{emu} #{filename}`
        # print result
      when 'x6502'
        system "x6502 #{filename}"
      end
    end

    # ソースコードを中間コードにコンパイル
    def compile( filename, opt = Hash.new )

      base = File.basename(opt[:out],'.*')
      Fc::LIB_PATH << Pathname(Fc::FC_HOME + 'fclib' + opt[:target])

      hlc = Fc::Hlc.new
      hlc.compile( filename )

      hlc
    end

    # 中間コードをアセンブラファイルにコンパイルする
    def compile2( hlc, opt )
      llc = Fc::Llc.new( opt )

      hlc.modules.each do |path,mod|
        next if mod.from_fcm
        asm, inc = llc.compile( mod )
        IO.write( BUILD_PATH+("_#{mod.id}.inc"), inc.join("\n") )
        IO.write( BUILD_PATH+("_#{mod.id}.s"), asm.join("\n") )
      end
    end

    # アセンブラファイルをオブジェクトファイルにコンパイルする
    def assemble( hlc, opt )
      objs = []
      hlc.modules.each do |path,mod|
        #.oファイルの作成
        objs << BUILD_PATH+('_'+mod.id.to_s+'.o')
        next if mod.from_fcm
        ca65 BUILD_PATH+('_'+mod.id.to_s+'.s')
      end

      objs
    end

    def make_runtime( target )
      ca65 find_share('runtime.asm')
      ca65 FC_HOME+'fclib'+target+'runtime_init.asm'
    end

    def make_base( hlc )
      case hlc.options[:mapper]
      when nil, "MMC0"
        inesmap = 0
      when "MMC3"
        inesmap = 4
      when Numeric
        inesmap = hlc.options[:mapper]
      end

      # オプションのアセンブラ
      inesprg = (hlc.options[:bank_count] || 4 ) / 2
      ineschr = (hlc.options[:char_banks] || 1 )
      options = { inesprg: inesprg, ineschr: ineschr, inesmir: 1, inesmap: inesmap }
      template = IO.read( find_share('base.asm.erb') ) 
      str = ERB.new(template,nil,'-').result(binding)
      IO.write( BUILD_PATH+'base.s', str )
      ca65 BUILD_PATH+'base.s'
    end

    # リンクする
    def link( hlc, objs, opt )

      ineschr = (hlc.options[:char_banks] || 1 )

      if hlc.options[:bank_count]
        bank_num = hlc.options[:bank_count]
        banks = (0...bank_num).map do |i|
          size = 0x2000
          size -= 6 if i == bank_num-1 # vectors size
          org = 0x8000 + (i%4) * 0x2000
          {size:size, org:org}
        end
      else
        banks = [{size:0x8000-6, org:0x8000}]
      end
      segs = hlc.modules.map do |name,m| 
        bank = m.options[:bank] || 0
        bank = banks.size + bank if bank < 0
        if m.options[:org]
          banks[bank][:org] = m.options[:org]
        end
        {name: m.id.to_s, bank: bank}
      end
      
      cfg = ERB.new(IO.read( find_share('ld65.cfg.erb') ),nil,'-').result(binding)
      IO.write( BUILD_PATH+'ld65.cfg', cfg )

      sh( LD65, '-m', opt[:out].sub_ext('.map'), '-o', opt[:out], '-C', BUILD_PATH+'ld65.cfg', 
          BUILD_PATH+'base.o', BUILD_PATH+'runtime_init.o', BUILD_PATH+'runtime.o', *objs )

    end

    def ca65( path )
      sh( CA65, '-o', BUILD_PATH+path.basename.sub_ext('.o'),
          '-I', FC_HOME+'share', '-I', BUILD_PATH, '-I', FC_HOME+'fclib', '-I', '.',
          '-I', FC_HOME+'fclib'+@target, path )
    end

    def sh( *args )
      command = args.map(&:to_s)
      dout 1, command.join(" ")
      Open3.popen2e( *command ) do |i, oe, th|
        v = th.value
        if v != 0
          raise CommandError.new( "#{args[0]} returns #{v}", command, oe.read )
        end
      end
    end

  end

end
