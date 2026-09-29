package quicknes

// libretro の API を syscall で呼ぶ (cgo を使わない)。コールバックは syscall.NewCallback で 1 度だけ作り、今の Machine に回す。

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"syscall"
	"unsafe"
)

const defaultPath = `C:\Applications\libretro\quicknes_libretro.dll`

type gameInfo struct {
	path *byte
	data uintptr
	size uintptr
	meta *byte
}

type retroVariable struct {
	key   *byte
	value *byte
}

var (
	dll       *syscall.LazyDLL
	loadErr   error
	callbacks [6]uintptr
	cur       *Machine
	pixFmt    uint32
	frame     []byte
	fw, fh    int
	fpitch    int
	romData   []byte                                // retro_load_game に渡した ROM (コアが持っている間は生かす)
	variables = map[string][]byte{                   // コアの設定 (終端 0 の値。コアが指す間は生かす)
		"quicknes_use_overscan_h":  []byte("enabled\x00"),
		"quicknes_use_overscan_v":  []byte("enabled\x00"),
		"quicknes_no_sprite_limit": []byte("disabled\x00"),
	}
)

// libretro の入力の番号 (RETRO_DEVICE_ID_JOYPAD_*) → ボタンのビット
var joypad = map[uintptr]uint8{0: ButtonB, 2: ButtonSelect, 3: ButtonStart, 4: ButtonUp, 5: ButtonDown, 6: ButtonLeft, 7: ButtonRight, 8: ButtonA}

func path() string {
	if p := os.Getenv("FC_QUICKNES"); p != "" {
		return p
	}
	return defaultPath
}

func cstring(p *byte) string {
	if p == nil {
		return ""
	}
	var b []byte
	for q := unsafe.Pointer(p); *(*byte)(q) != 0; q = unsafe.Add(q, 1) {
		b = append(b, *(*byte)(q))
	}
	return string(b)
}

func load() error {
	if dll != nil || loadErr != nil {
		return loadErr
	}
	p := path()
	if _, err := os.Stat(p); err != nil {
		loadErr = ErrUnavailable
		return loadErr
	}
	d := syscall.NewLazyDLL(p)
	if err := d.Load(); err != nil {
		loadErr = fmt.Errorf("quicknes: %w", err)
		return loadErr
	}
	dll = d
	callbacks = [6]uintptr{
		syscall.NewCallback(func(cmd, data uintptr) uintptr {
			switch cmd {
			case 10: // SET_PIXEL_FORMAT
				pixFmt = *(*uint32)(foreign(data))
				return 1
			case 15: // GET_VARIABLE
				v := (*retroVariable)(foreign(data))
				if val, ok := variables[cstring(v.key)]; ok {
					v.value = &val[0]
					return 1
				}
				return 0
			}
			return 0
		}),
		syscall.NewCallback(func(data, w, h, pitch uintptr) uintptr {
			if data != 0 {
				fw, fh, fpitch = int(w), int(h), int(pitch)
				frame = append(frame[:0], unsafe.Slice((*byte)(foreign(data)), fh*fpitch)...)
			}
			return 0
		}),
		syscall.NewCallback(func(l, r uintptr) uintptr { return 0 }),
		syscall.NewCallback(func(d, n uintptr) uintptr { return n }),
		syscall.NewCallback(func() uintptr { return 0 }),
		syscall.NewCallback(func(port, device, index, id uintptr) uintptr {
			if cur == nil || device != 1 || port > 1 {
				return 0
			}
			if cur.buttons[port]&joypad[id] != 0 {
				return 1
			}
			return 0
		}),
	}
	return nil
}

// foreign はコア (DLL) が渡した番地をポインタにする (Go のヒープの外のメモリなので GC は関わらない)。
func foreign(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

func call(name string, args ...uintptr) uintptr {
	r, _, _ := dll.NewProc(name).Call(args...)
	return r
}

// Open はコアを起こして rom を読み込む (Close まで大域の錠を持つ)。
func Open(rom []byte) (*Machine, error) {
	lock.Lock()
	if err := load(); err != nil {
		lock.Unlock()
		return nil, err
	}
	m := &Machine{}
	cur = m
	frame = frame[:0]
	call("retro_set_environment", callbacks[0])
	call("retro_set_video_refresh", callbacks[1])
	call("retro_set_audio_sample", callbacks[2])
	call("retro_set_audio_sample_batch", callbacks[3])
	call("retro_set_input_poll", callbacks[4])
	call("retro_set_input_state", callbacks[5])
	call("retro_init")
	romData = append([]byte(nil), rom...)
	gi := gameInfo{data: uintptr(unsafe.Pointer(&romData[0])), size: uintptr(len(romData))}
	if call("retro_load_game", uintptr(unsafe.Pointer(&gi)))&0xff == 0 {
		call("retro_deinit")
		cur = nil
		lock.Unlock()
		return nil, fmt.Errorf("quicknes: the core could not load the ROM (unsupported mapper?)")
	}
	return m, nil
}

// RunFrames は n フレーム動かす。
func (m *Machine) RunFrames(n int) {
	for i := 0; i < n; i++ {
		call("retro_run")
		m.frames++
	}
}

// RAM は CPU の内部 RAM ($0000〜$07FF) の写し。
func (m *Machine) RAM() []byte {
	p := call("retro_get_memory_data", 2) // RETRO_MEMORY_SYSTEM_RAM
	n := call("retro_get_memory_size", 2)
	if p == 0 || n == 0 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice((*byte)(foreign(p)), n)...)
}

// Close はコアを止めて錠を放す。
func (m *Machine) Close() {
	if cur != m {
		return
	}
	call("retro_unload_game")
	call("retro_deinit")
	cur = nil
	romData = nil
	lock.Unlock()
}

// lastImage は最後のフレームを 256×240 の画像にする (コアの画面が小さければ真ん中に置く)。
func lastImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	ox, oy := (Width-fw)/2, (Height-fh)/2
	for y := 0; y < fh && y+oy < Height; y++ {
		for x := 0; x < fw && x+ox < Width; x++ {
			var c color.RGBA
			switch pixFmt {
			case 2: // RGB565
				v := uint16(frame[y*fpitch+x*2]) | uint16(frame[y*fpitch+x*2+1])<<8
				c = color.RGBA{uint8(v>>11) << 3, uint8(v>>5&63) << 2, uint8(v&31) << 3, 255}
			case 1: // XRGB8888
				o := y*fpitch + x*4
				c = color.RGBA{frame[o+2], frame[o+1], frame[o], 255}
			default: // 0RGB1555
				v := uint16(frame[y*fpitch+x*2]) | uint16(frame[y*fpitch+x*2+1])<<8
				c = color.RGBA{uint8(v>>10&31) << 3, uint8(v>>5&31) << 3, uint8(v&31) << 3, 255}
			}
			img.SetRGBA(x+ox, y+oy, c)
		}
	}
	return img
}
