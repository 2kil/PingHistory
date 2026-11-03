// Package pinger 负责按目标调度 ping 探测并落库。
package pinger

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"sync"
	"time"

	"pinghistory/internal/store"
)

// 匹配中英文 ping 输出中的往返时延，例如 "时间=15ms"、"时间<1ms"、"time=15ms"。
var rttPattern = regexp.MustCompile(`(?i)([=<])\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)

// probeTimeout 是单次 ping 的等待上限。
const probeTimeout = 1600 * time.Millisecond

// 数据载荷大小（ping -l / -s 参数）。
const (
	defaultPacketSize = 32
	maxPacketSize     = 65000
)

func normalizeSize(size int) int {
	if size <= 0 {
		return defaultPacketSize
	}
	if size > maxPacketSize {
		return maxPacketSize
	}
	return size
}

var (
	// probeSlots 为 nil 时表示不做并发限制（见 limit_*.go 中的平台常量）。
	probeSlots = newProbeLimiter()
)

func newProbeLimiter() chan struct{} {
	if maxConcurrentProbes <= 0 {
		return nil
	}
	return make(chan struct{}, maxConcurrentProbes)
}

func acquireProbe() {
	if probeSlots != nil {
		probeSlots <- struct{}{}
	}
}

func releaseProbe() {
	if probeSlots != nil {
		<-probeSlots
	}
}

type worker struct {
	host       string
	interval   time.Duration
	packetSize int
	cancel     context.CancelFunc
}

// Manager 管理所有目标的探测协程。
type Manager struct {
	st *store.Store

	rootCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// opMu 串行化「同步/重置/停止」这类会增删协程的操作，避免 WaitGroup 与 workers 竞态。
	opMu sync.Mutex

	mu      sync.Mutex
	workers map[int64]*worker
	last    map[int64]store.Probe
}

// New 创建探测器。
func New(st *store.Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	if maxConcurrentProbes > 0 {
		log.Printf("探测并发上限：%d", maxConcurrentProbes)
	} else {
		log.Printf("探测并发：不限制")
	}
	return &Manager{
		st:      st,
		rootCtx: ctx,
		cancel:  cancel,
		workers: make(map[int64]*worker),
		last:    make(map[int64]store.Probe),
	}
}

// Sync 与数据库中的目标列表对齐：新增、重启（主机/间隔变更）、停止（删除或停用）。
func (m *Manager) Sync() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.syncLocked()
}

func (m *Manager) syncLocked() {
	targets, err := m.st.ListTargets()
	if err != nil {
		log.Printf("同步目标失败: %v", err)
		return
	}

	want := make(map[int64]store.Target, len(targets))
	for _, t := range targets {
		if t.Enabled {
			want[t.ID] = t
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, w := range m.workers {
		t, ok := want[id]
		if !ok {
			w.cancel()
			delete(m.workers, id)
			delete(m.last, id)
			continue
		}
		if t.Host != w.host || time.Duration(t.IntervalSec)*time.Second != w.interval || normalizeSize(t.PacketSize) != w.packetSize {
			w.cancel()
			delete(m.workers, id)
		}
	}

	for id, t := range want {
		if _, ok := m.workers[id]; ok {
			continue
		}
		interval := time.Duration(t.IntervalSec) * time.Second
		if interval < time.Second {
			interval = 5 * time.Second
		}
		size := normalizeSize(t.PacketSize)
		ctx, cancel := context.WithCancel(m.rootCtx)
		m.workers[id] = &worker{host: t.Host, interval: interval, packetSize: size, cancel: cancel}
		m.wg.Add(1)
		go m.run(ctx, id, t.Host, size, interval)
	}
}

func (m *Manager) run(ctx context.Context, id int64, host string, packetSize int, interval time.Duration) {
	defer m.wg.Done()

	probe := func() {
		start := time.Now()
		acquireProbe()
		rtt, ok := Ping(host, packetSize, probeTimeout)
		releaseProbe()
		ts := time.Now().UnixMilli()
		if ok {
			// 解析出的 RTT 为 0 时使用实际耗时兜底，避免图表出现 0ms 假值。
			if rtt <= 0 {
				rtt = float64(time.Since(start).Microseconds()) / 1000
			}
		}
		var value *float64
		if ok {
			v := round2(rtt)
			value = &v
		}
		rec := store.Probe{TS: ts, RTT: value, OK: ok}

		m.mu.Lock()
		m.last[id] = rec
		m.mu.Unlock()

		if err := m.st.InsertProbe(id, ts, value, ok); err != nil && ctx.Err() == nil {
			log.Printf("写入探测结果失败 (target=%d): %v", id, err)
		}
	}

	// 立即执行一次，避免新建目标后界面长时间空白。
	probe()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probe()
		}
	}
}

// Last 返回目标最近一次探测结果（内存态）。
func (m *Manager) Last(id int64) (store.Probe, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.last[id]
	return p, ok
}

// Reset 清空全部探测历史，并让所有目标从同一时刻重新开始采集。
func (m *Manager) Reset() (int64, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	// 先停掉全部协程并等待退出，避免旧协程在清空数据后又补写一条记录。
	m.mu.Lock()
	for id, w := range m.workers {
		w.cancel()
		delete(m.workers, id)
	}
	m.mu.Unlock()
	m.wg.Wait()

	m.mu.Lock()
	m.last = make(map[int64]store.Probe)
	m.mu.Unlock()

	deleted, err := m.st.ResetProbes()
	if err != nil {
		return 0, err
	}

	// 重新拉起协程：worker 启动时会立即探测一次，使各目标的起点对齐。
	m.syncLocked()
	return deleted, nil
}

// Stop 停止全部探测协程。
func (m *Manager) Stop() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.cancel()
	m.wg.Wait()
}

// Ping 执行一次 ping 并返回往返时延（毫秒）。packetSize 为 ICMP 载荷字节数。
func Ping(host string, packetSize int, timeout time.Duration) (float64, bool) {
	out, err := runPing(host, normalizeSize(packetSize), timeout)
	if err != nil && out == "" {
		return 0, false
	}
	m := rttPattern.FindStringSubmatch(out)
	if len(m) != 3 {
		return 0, false
	}
	v, convErr := strconv.ParseFloat(m[2], 64)
	if convErr != nil {
		return 0, false
	}
	// 输出形如 "<1ms" 时取半值，保留亚毫秒精度。
	if m[1] == "<" {
		v = v / 2
	}
	return v, true
}

func round2(v float64) float64 {
	if v < 0 {
		return 0
	}
	return float64(int64(v*100+0.5)) / 100
}

// decodeOutput 直接返回原始字节。
// 中文版 Windows 的 ping 输出为 GBK 编码，但需要提取的 "=<数字>ms" 片段全部是 ASCII 字节，
// 且 GBK 双字节的第二字节最小为 0x40，不会与 "=" (0x3D) 混淆，因此不转码也能安全匹配。
func decodeOutput(b []byte) string {
	return string(b)
}
