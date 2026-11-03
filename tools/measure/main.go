// Command measure 是一次性的容量测试工具：写入固定数量的样本，
// 统计数据库文件的实际体积，用于评估低存储设备上的占用。
package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"pinghistory/internal/store"
)

func main() {
	dir, err := os.MkdirTemp("", "ph-measure-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "pinghistory.db")
	st, err := store.Open(path)
	if err != nil {
		panic(err)
	}

	target, err := st.AddTarget("measure", "223.5.5.5", 1, 32)
	if err != nil {
		panic(err)
	}

	const rows = 20000
	now := time.Now().UnixMilli()
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < rows; i++ {
		rtt := 4.5 + rnd.Float64()*3
		v := float64(int64(rtt*100+0.5)) / 100
		ok := i%97 != 0
		var value *float64
		if ok {
			value = &v
		}
		if err := st.InsertProbe(target.ID, now-int64(rows-i)*1000, value, ok); err != nil {
			panic(err)
		}
	}

	// 触发一次 checkpoint，使数据从 WAL 落入主库，模拟长期运行后的稳态。
	_ = st.Close()

	dbInfo, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	walInfo, err := os.Stat(path + "-wal")
	walSize := int64(0)
	if err == nil {
		walSize = walInfo.Size()
	}

	fmt.Printf("rows=%d\n", rows)
	fmt.Printf("db=%d bytes (%.1f KB)\n", dbInfo.Size(), float64(dbInfo.Size())/1024)
	fmt.Printf("wal=%d bytes (%.1f KB)\n", walSize, float64(walSize)/1024)
	fmt.Printf("per-row=%.1f bytes\n", float64(dbInfo.Size())/float64(rows))
}
