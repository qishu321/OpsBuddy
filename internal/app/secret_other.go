//go:build !windows && !linux

package app

import "errors"

func seal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return "", errors.New("此版本的凭据存储仅支持 Windows；尚未实现当前系统的安全凭据后端")
}
func unseal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return "", errors.New("当前系统不支持读取 Windows 凭据")
}
