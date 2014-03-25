module R6502
  class Assembler
    def opcode(instr, mode)
      { :adc => {:imm => 0x69, :zp => 0x65, :zpx => 0x75, :zpy => nil,  :abs => 0x6d, :absx => 0x7d, :absy => 0x79, :ind => nil,  :indx => 0x61, :indy => 0x71, :imp => nil,  :rel => nil },
        :and => {:imm => 0x29, :zp => 0x25, :zpx => 0x35, :zpy => nil,  :abs => 0x2d, :absx => 0x3d, :absy => 0x39, :ind => nil,  :indx => 0x21, :indy => 0x31, :imp => nil,  :rel => nil },
        :asl => {:imm => nil,  :zp => 0x06, :zpx => 0x16, :zpy => nil,  :abs => 0x0e, :absx => 0x1e, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x0a, :rel => nil },
        :bit => {:imm => nil,  :zp => 0x24, :zpx => nil,  :zpy => nil,  :abs => 0x2c, :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :bpl => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0x10},
        :bmi => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0x30},
        :bvc => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0x50},
        :bvs => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0x70},
        :bcc => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0x90},
        :bcs => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0xb0},
        :bne => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0xd0},
        :beq => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => 0xf0},
        :brk => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x00, :rel => nil },
        :cmp => {:imm => 0xc9, :zp => 0xc5, :zpx => 0xd5, :zpy => nil,  :abs => 0xcd, :absx => 0xdd, :absy => 0xd9, :ind => nil,  :indx => 0xc1, :indy => 0xd1, :imp => nil,  :rel => nil },
        :cpx => {:imm => 0xe0, :zp => 0xe4, :zpx => nil,  :zpy => nil,  :abs => 0xec, :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :cpy => {:imm => 0xc0, :zp => 0xc4, :zpx => nil,  :zpy => nil,  :abs => 0xcc, :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :dec => {:imm => nil,  :zp => 0xc6, :zpx => 0xd6, :zpy => nil,  :abs => 0xce, :absx => 0xde, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :eor => {:imm => 0x49, :zp => 0x45, :zpx => 0x55, :zpy => nil,  :abs => 0x4d, :absx => 0x5d, :absy => 0x59, :ind => nil,  :indx => 0x41, :indy => 0x51, :imp => nil,  :rel => nil },
        :clc => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x18, :rel => nil },
        :sec => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x38, :rel => nil },
        :cli => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x58, :rel => nil },
        :sei => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x78, :rel => nil },
        :clv => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xb8, :rel => nil },
        :cld => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xd8, :rel => nil },
        :sed => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xf8, :rel => nil },
        :inc => {:imm => nil,  :zp => 0xe6, :zpx => 0xf6, :zpy => nil,  :abs => 0xee, :absx => 0xfe, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :jmp => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => 0x4c, :absx => nil,  :absy => nil,  :ind => 0x6c, :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :jsr => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => 0x20, :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :lda => {:imm => 0xa9, :zp => 0xa5, :zpx => 0xb5, :zpy => nil,  :abs => 0xad, :absx => 0xbd, :absy => 0xb9, :ind => nil,  :indx => 0xa1, :indy => 0xb1, :imp => nil,  :rel => nil },
        :ldx => {:imm => 0xa2, :zp => 0xa6, :zpx => nil,  :zpy => 0xb6, :abs => 0xae, :absx => nil,  :absy => 0xbe, :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :ldy => {:imm => 0xa0, :zp => 0xa4, :zpx => 0xb4, :zpy => nil,  :abs => 0xac, :absx => 0xbc, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :lsr => {:imm => nil,  :zp => 0x46, :zpx => 0x56, :zpy => nil,  :abs => 0x4e, :absx => 0x5e, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x4a, :rel => nil },
        :nop => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xea, :rel => nil },
        :ora => {:imm => 0x09, :zp => 0x05, :zpx => 0x15, :zpy => nil,  :abs => 0x0d, :absx => 0x1d, :absy => 0x19, :ind => nil,  :indx => 0x01, :indy => 0x11, :imp => nil,  :rel => nil },
        :tax => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xaa, :rel => nil },
        :txa => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x8a, :rel => nil },
        :dex => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xca, :rel => nil },
        :inx => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xe8, :rel => nil },
        :tay => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xa8, :rel => nil },
        :tya => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x98, :rel => nil },
        :dey => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x88, :rel => nil },
        :iny => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xc8, :rel => nil },
        :ror => {:imm => nil,  :zp => 0x66, :zpx => 0x76, :zpy => nil,  :abs => 0x6e, :absx => 0x7e, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x6a, :rel => nil },
        :rol => {:imm => nil,  :zp => 0x26, :zpx => 0x36, :zpy => nil,  :abs => 0x2e, :absx => 0x3e, :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x2a, :rel => nil },
        :rti => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x40, :rel => nil },
        :rts => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x60, :rel => nil },
        :sbc => {:imm => 0xe9, :zp => 0xe5, :zpx => 0xf5, :zpy => nil,  :abs => 0xed, :absx => 0xfd, :absy => 0xf9, :ind => nil,  :indx => 0xe1, :indy => 0xf1, :imp => nil,  :rel => nil },
        :sta => {:imm => nil,  :zp => 0x85, :zpx => 0x95, :zpy => nil,  :abs => 0x8d, :absx => 0x9d, :absy => 0x99, :ind => nil,  :indx => 0x81, :indy => 0x91, :imp => nil,  :rel => nil },
        :txs => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x9a, :rel => nil },
        :tsx => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0xba, :rel => nil },
        :pha => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x48, :rel => nil },
        :pla => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x68, :rel => nil },
        :php => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x08, :rel => nil },
        :plp => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => 0x28, :rel => nil },
        :stx => {:imm => nil,  :zp => 0x86, :zpx => nil,  :zpy => 0x96, :abs => 0x8e, :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :sty => {:imm => nil,  :zp => 0x84, :zpx => 0x94, :zpy => nil,  :abs => 0x8c, :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil },
        :nil => {:imm => nil,  :zp => nil,  :zpx => nil,  :zpy => nil,  :abs => nil,  :absx => nil,  :absy => nil,  :ind => nil,  :indx => nil,  :indy => nil,  :imp => nil,  :rel => nil }
      }[instr][mode]
    end
  end
end