//go:build !windows

package quicknes

import "image"

// Open は Windows 以外ではいつも ErrUnavailable (DLL を syscall で読むので)。
func Open(rom []byte) (*Machine, error) { return nil, ErrUnavailable }

func (m *Machine) RunFrames(n int) {}
func (m *Machine) RAM() []byte     { return nil }
func (m *Machine) Close()          {}

func lastImage() *image.RGBA { return image.NewRGBA(image.Rect(0, 0, Width, Height)) }
