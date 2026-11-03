//go:build windows

// Package osutil 提供少量与桌面交互的系统调用封装。
package osutil

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

const swShownormal = 1

func shellExecute(verb, target string) error {
	verbPtr, err := syscall.UTF16PtrFromString(verb)
	if err != nil {
		return err
	}
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(targetPtr)),
		0,
		0,
		swShownormal,
	)
	// ShellExecuteW 返回值大于 32 表示成功。
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW 失败，返回码 %d", ret)
	}
	return nil
}

// OpenBrowser 使用系统默认浏览器打开 URL。
func OpenBrowser(url string) error {
	return shellExecute("open", url)
}

// OpenPath 使用系统默认程序打开文件或目录。
func OpenPath(path string) error {
	return shellExecute("open", path)
}
