//go:build linux

package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const linuxSecretPrefix = "linux-aesgcm-v1:"

// Linux uses a per-user random 256-bit key, outside the workspace database.
// Filesystem access to both the database and this key permits decryption.
// This is local encrypted storage, not a hardware-backed credential vault.
func linuxCredentialKey() ([]byte, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(config, "opsbuddy-credentials")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0077 != 0 || stat.Uid != uint32(os.Getuid()) {
		return nil, errors.New("凭据目录必须属于当前用户且权限为0700")
	}
	path := filepath.Join(dir, "key")
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		// Link a fully written temporary file without replacing an existing key.
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.CreateTemp(dir, ".key-")
		if e != nil {
			return nil, e
		}
		temp := f.Name()
		defer os.Remove(temp)
		if _, e = f.Write(key); e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if e = os.Link(temp, path); e != nil && !os.IsExist(e) {
			return nil, e
		}
	} else if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok = info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || stat.Uid != uint32(os.Getuid()) || info.Size() != 32 {
		return nil, errors.New("凭据密钥必须属于当前用户且是0600的32字节文件")
	}
	key := make([]byte, 32)
	_, err = io.ReadFull(f, key)
	return key, err
}
func linuxAEAD() (cipher.AEAD, error) {
	key, err := linuxCredentialKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func seal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	a, err := linuxAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return linuxSecretPrefix + base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(s), []byte(linuxSecretPrefix))), nil
}
func unseal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if !strings.HasPrefix(s, linuxSecretPrefix) {
		return "", errors.New("Linux 无法读取其他平台的凭据，请重新配置 API Key")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, linuxSecretPrefix))
	if err != nil {
		return "", err
	}
	a, err := linuxAEAD()
	if err != nil {
		return "", err
	}
	if len(data) < a.NonceSize()+a.Overhead() {
		return "", errors.New("凭据密文损坏")
	}
	plain, err := a.Open(nil, data[:a.NonceSize()], data[a.NonceSize():], []byte(linuxSecretPrefix))
	if err != nil {
		return "", errors.New("凭据解密失败")
	}
	return string(plain), nil
}
