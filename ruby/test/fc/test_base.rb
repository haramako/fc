# coding: utf-8

require 'rspec'
require 'simplecov'
SimpleCov.start

require_relative '../../lib/fc/base'


include Fc

describe Type, 'when initialize' do

  it 'as void' do
    v = Type[ :void ]
    expect(v.kind).to eq :void
    expect(v.base).to eq nil
    expect(v.size).to eq 0
  end

  it 'as bool' do
    v = Type[ :bool ]
    expect(v.kind).to eq :bool
    expect(v.base).to eq  nil
    expect(v.size).to eq  1
  end

  it 'as int' do
    v = Type[ :int ]
    expect(v.kind).to eq  :int
    expect(v.signed).to eq false
    expect(v.base).to eq nil
    expect(v.size).to eq 1
  end

  it 'as array' do
    v = Type[ [:array, 10, :int16 ] ]
    expect(v.kind).to eq :array
    expect(v.base).to eq Type[:int16]
    expect(v.length).to eq 10
    expect(v.size).to eq 20
  end

  it 'as pointer' do
    v = Type[ [:pointer, :int ] ]
    expect(v.kind).to eq :pointer
    expect(v.size).to eq 2
    expect(v.base).to eq Type[:int]
  end

  it 'as lambda' do
    v = Type[ [:lambda, [Type[:int], Type[:int]], [:pointer, :int] ] ]
    expect(v.kind).to eq :lambda
    expect(v.size).to eq 2
    expect(v.base).to eq Type[ [:pointer,:int] ]
    expect(v.args).to eq [ Type[:int], Type[:int] ]
  end

  it 'as complex type' do
    v = Type[ [:array, 10, [:pointer, [:array, 2, :int] ] ] ]
    expect(v.to_s).to eq 'uint8[2]*[10]'
  end

end

