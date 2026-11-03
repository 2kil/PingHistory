//go:build !windows

// Package tray 在非 Windows 平台不提供托盘能力。
package tray

import "errors"

// ErrUnsupported 在非 Windows 平台返回。
var ErrUnsupported = errors.New("当前平台不支持系统托盘")

// Options 是托盘运行参数。
type Options struct {
	URL      string
	IconPath string
	DataDir  string
	OnQuit   func()
}

// Run 在非 Windows 平台直接返回不支持的错误。
func Run(opts Options) error {
	return ErrUnsupported
}

// AcquireSingleInstance 在非 Windows 平台不做限制。
func AcquireSingleInstance(name string) bool {
	return true
}
