//go:build !windows

// Package osutil 提供少量与桌面交互的系统调用封装。
package osutil

import (
	"os/exec"
	"runtime"
)

// OpenBrowser 使用系统默认浏览器打开 URL。
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// OpenPath 使用系统默认程序打开文件或目录。
func OpenPath(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
