//go:build !(windows && amd64)

package pinger

// 低功耗设备（路由器、ARM 盒子等）同时拉起多个 ping 进程会造成瞬时 CPU 尖峰，
// 这里限制并发为 5。
const maxConcurrentProbes = 5
