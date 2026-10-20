//go:build !windows

package app

import (
	"os/exec"
	"sync"
	"syscall"
)

func prepareCodeProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func ownCodeProcess(cmd *exec.Cmd) (func(), error) {
	var once sync.Once
	return func() { once.Do(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }) }, nil
}
