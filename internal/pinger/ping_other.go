//go:build !windows

package pinger

import (
	"os/exec"
	"strconv"
	"time"
)

func runPing(host string, packetSize int, timeout time.Duration) (string, error) {
	sec := int(timeout / time.Second)
	if sec < 1 {
		sec = 1
	}
	cmd := exec.Command("ping", "-c", "1", "-W", strconv.Itoa(sec), "-s", strconv.Itoa(packetSize), host)
	out, err := cmd.CombinedOutput()
	return decodeOutput(out), err
}
