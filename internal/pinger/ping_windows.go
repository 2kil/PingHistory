//go:build windows

package pinger

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// runPing 调用系统 ping.exe，输出文本用于解析 RTT。
// -l 指定 ICMP 载荷字节数；使用隐藏窗口方式启动，避免 GUI 程序下出现控制台闪烁。
func runPing(host string, packetSize int, timeout time.Duration) (string, error) {
	ms := int(timeout / time.Millisecond)
	if ms < 500 {
		ms = 500
	}
	cmd := exec.Command("ping", "-n", "1", "-w", strconv.Itoa(ms), "-l", strconv.Itoa(packetSize), host)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return decodeOutput(out), err
}
