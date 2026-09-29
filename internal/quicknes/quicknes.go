// Package quicknes は libretro の QuickNES のコア (quicknes_libretro.dll) で NES の ROM を動かし、画面を画像にする。
// internal/nes (内蔵のランナー) は PPU の中身を記録するだけで描画の途中の変化 (スクロールの分割・ラスター効果) を描かないので、
// 画面の見た目を確かめるテストに使う。コアは本物の PPU と同じく走査線ごとに描き、MMC3 の走査線の IRQ も動く。
//
// コアの場所は環境変数 FC_QUICKNES か C:\Applications\libretro\quicknes_libretro.dll。無ければ Open が ErrUnavailable を
// 返す (テストは Skip する)。libretro のコアはプロセスに 1 つしか動かせないので、Open から Close までは大域の錠を持つ
// (同時に Open すると待つ)。
package quicknes

import (
	"errors"
	"image"
	"sync"
)

// ボタン (internal/nes と同じビット)。
const (
	ButtonA = 1 << iota
	ButtonB
	ButtonSelect
	ButtonStart
	ButtonUp
	ButtonDown
	ButtonLeft
	ButtonRight
)

// Width / Height は Image の大きさ (NES の画面そのまま。コアの上下左右の切り落としは切っている)。
const (
	Width  = 256
	Height = 240
)

// ErrUnavailable はコアが見つからない (Windows 以外、または DLL が無い)。
var ErrUnavailable = errors.New("quicknes: libretro core not found (set FC_QUICKNES to quicknes_libretro.dll)")

// Machine は 1 本の ROM を動かしているコア。
type Machine struct {
	buttons [2]uint8
	frames  int
}

var lock sync.Mutex

// Frames は動かしたフレーム数。
func (m *Machine) Frames() int { return m.frames }

// SetButtons は port (0 = 1P、1 = 2P) の押しているボタンを決める。
func (m *Machine) SetButtons(port int, b uint8) { m.buttons[port&1] = b }

// Image は最後のフレームの画面。
func (m *Machine) Image() *image.RGBA { return lastImage() }
