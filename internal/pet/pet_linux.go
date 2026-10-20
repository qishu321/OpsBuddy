//go:build linux

package pet

import "errors"

// No background window, timer, or renderer is created on Linux.
type Companion struct{}

func New(string, func()) (*Companion, error) {
	return nil, errors.New("原生桌宠暂仅支持 Windows")
}
func (*Companion) Settings() Settings                 { return Settings{} }
func (*Companion) Configure(Settings) error           { return errors.New("原生桌宠暂仅支持 Windows") }
func (*Companion) Diagnostics() Diagnostics           { return Diagnostics{} }
func (*Companion) ActivityDetail(string, int, string) {}
func (*Companion) Close()                             {}
