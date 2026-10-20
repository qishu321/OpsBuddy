//go:build windows

package app

import (
	"encoding/base64"
	"errors"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	size uint32
	data *byte
}

func protect(data []byte, decode bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := dataBlob{uint32(len(data)), &data[0]}
	var out dataBlob
	name := "CryptProtectData"
	if decode {
		name = "CryptUnprotectData"
	}
	proc := syscall.NewLazyDLL("crypt32.dll").NewProc(name)
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, errors.New("Windows 凭据加密/解密失败")
	}
	defer syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree").Call(uintptr(unsafe.Pointer(out.data)))
	return append([]byte{}, unsafe.Slice(out.data, int(out.size))...), nil
}
func seal(s string) (string, error) {
	b, e := protect([]byte(s), false)
	return base64.StdEncoding.EncodeToString(b), e
}
func unseal(s string) (string, error) {
	b, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		return "", e
	}
	b, e = protect(b, true)
	return string(b), e
}
