//go:build windows && amd64

package pinger

// Windows x64 桌面环境算力充足，进程创建开销可接受，不做并发限制，
// 避免多个目标同一时刻到点时相互排队导致采样时间偏移。
const maxConcurrentProbes = 0
