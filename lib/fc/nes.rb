# coding: utf-8

def parse_nes( file )
  data = open( file, 'r:ASCII-8BIT' ){|f|f.read}

  # ヘッダの解析
  head, _1a, prog_count, chr_count, ctrl1, ctrl2, _00, _00, pal_ntsc, _00x5 = data.unpack( 'A3C8A5' )
  raise "invalid iNES file '#{file}'" if head != 'NES'

  # ROMデータの読み出し
  prog_bank = []
  (prog_count*2).times do |i|
    prog_bank << data[16 + i*8*1024, 8*1024].unpack('C*')
  end

  chr_bank = []
  chr_count.times do |i|
    chr_bank << data[16 + prog_count*16*1024 + i*8*1024, 8*1024].unpack('C*')
  end

  # プログラムサイズを測る
  prog_size = 0
  prog_bank.each do |mem|
    bank_size = nil
    (mem.size-7).downto(0) do |i|
      bank_size = i
      break if mem[i] != 255
    end
    prog_size += bank_size
  end

  { prog_size: prog_size, prog_bank: prog_bank, chr_bank: chr_bank }
end

