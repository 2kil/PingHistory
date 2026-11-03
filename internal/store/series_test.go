package store

import (
	"path/filepath"
	"testing"
	"time"
)

// TestSeriesSharesBucketGrid 验证：同一个 until 下，不同目标的聚合时间桶必须落在同一栅格。
// 这是总览多目标叠加的前提——只要有一个目标算出不同桶宽，前端叠加后每条曲线都会被拆成孤立点。
func TestSeriesSharesBucketGrid(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	fast, err := st.AddTarget("fast", "223.5.5.5", 1, 32)
	if err != nil {
		t.Fatalf("建目标失败: %v", err)
	}
	slow, err := st.AddTarget("slow", "223.5.5.6", 5, 32)
	if err != nil {
		t.Fatalf("建目标失败: %v", err)
	}

	now := time.Now().UnixMilli()
	since := now - 3600_000

	seed := func(id int64, step int64, rtt float64) {
		for ts := now - 600_000; ts <= now; ts += step {
			v := rtt
			if err := st.InsertProbe(id, ts, &v, true); err != nil {
				t.Fatalf("写入样本失败: %v", err)
			}
		}
	}
	seed(fast.ID, 1000, 5)
	seed(slow.ID, 5000, 30)

	const points = 240
	seriesFast, err := st.Series(fast.ID, since, now, points)
	if err != nil {
		t.Fatalf("聚合失败: %v", err)
	}
	seriesSlow, err := st.Series(slow.ID, since, now, points)
	if err != nil {
		t.Fatalf("聚合失败: %v", err)
	}
	if len(seriesFast) == 0 || len(seriesSlow) == 0 {
		t.Fatal("聚合结果为空")
	}

	bucket := (now - since) / points
	grid := make(map[int64]struct{}, len(seriesFast))
	for _, p := range seriesFast {
		if p.T%bucket != 0 {
			t.Fatalf("桶 %d 未对齐到 %dms 栅格", p.T, bucket)
		}
		grid[p.T] = struct{}{}
	}
	for _, p := range seriesSlow {
		if _, ok := grid[p.T]; !ok {
			t.Fatalf("目标 slow 的桶 %d 与目标 fast 的栅格不一致", p.T)
		}
	}
}

// TestSeriesRejectsMixedUntil 固定复现修复前的故障模式：
// 只要 until 相差 1ms 就可能让桶宽差 1ms，两个目标的时间栅格完全错开（重叠为 0）。
func TestSeriesRejectsMixedUntil(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	target, err := st.AddTarget("t", "223.5.5.5", 1, 32)
	if err != nil {
		t.Fatalf("建目标失败: %v", err)
	}
	now := time.Now().UnixMilli()
	since := now - 3600_000
	for ts := now - 600_000; ts <= now; ts += 1000 {
		v := 5.0
		if err := st.InsertProbe(target.ID, ts, &v, true); err != nil {
			t.Fatalf("写入样本失败: %v", err)
		}
	}

	// span=3599999 → 桶宽 14999；span=3600000 → 桶宽 15000，栅格相差 1ms。
	a, err := st.Series(target.ID, since, now-1, 240)
	if err != nil {
		t.Fatalf("聚合失败: %v", err)
	}
	b, err := st.Series(target.ID, since, now, 240)
	if err != nil {
		t.Fatalf("聚合失败: %v", err)
	}

	setA := make(map[int64]struct{}, len(a))
	for _, p := range a {
		setA[p.T] = struct{}{}
	}
	overlap := 0
	for _, p := range b {
		if _, ok := setA[p.T]; ok {
			overlap++
		}
	}
	if overlap != 0 {
		t.Fatalf("预期栅格完全错开，实际重叠 %d 个", overlap)
	}
}
