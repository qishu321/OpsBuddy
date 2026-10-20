//go:build windows

package desktop

import (
	"errors"
	"syscall"
	"unsafe"
)

func shellOpen(path, arguments string) error {
	operation, _ := syscall.UTF16PtrFromString("open")
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	args, err := syscall.UTF16PtrFromString(arguments)
	if err != nil {
		return err
	}
	result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(operation)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, 1)
	if result <= 32 {
		return errors.New("Windows 无法打开该文件，请检查关联程序是否已安装")
	}
	return nil
}

func revealPath(path string) error { return shellOpen("explorer.exe", "/select,\""+path+"\"") }
