package store

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound 表示目标不存在。
var ErrNotFound = errors.New("目标不存在")

const schema = `
CREATE TABLE IF NOT EXISTS targets (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT    NOT NULL,
	host         TEXT    NOT NULL,
	interval_sec INTEGER NOT NULL DEFAULT 5,
	packet_size  INTEGER NOT NULL DEFAULT 32,
	enabled      INTEGER NOT NULL DEFAULT 1,
	created_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS probes (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	ts        INTEGER NOT NULL,
	rtt_ms    REAL,
	ok        INTEGER NOT NULL,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_probes_target_ts ON probes(target_id, ts);
CREATE INDEX IF NOT EXISTS idx_probes_ts ON probes(ts);
`

// Store 封装 SQLite 持久化。
type Store struct {
	db *sql.DB
}

// Target 是一个被监测的 ping 目标。
type Target struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	IntervalSec int    `json:"interval_sec"`
	PacketSize  int    `json:"packet_size"` // ping 的 -l / -s 参数（字节）
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
}

// Probe 是一次探测结果。
type Probe struct {
	TS  int64    `json:"ts"`
	RTT *float64 `json:"rtt"`
	OK  bool     `json:"ok"`
}

// Point 是聚合后的图表数据点。
type Point struct {
	T    int64    `json:"t"`
	RTT  *float64 `json:"rtt"`
	Max  *float64 `json:"max_rtt"`
	Loss float64  `json:"loss"`
	N    int      `json:"n"`
}

// Stats 是区间内的统计指标。
type Stats struct {
	Avg *float64 `json:"avg"`
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
	// MaxTS 是最大值出现的时刻，便于在界面上定位尖峰。
	MaxTS int64   `json:"max_ts"`
	Loss  float64 `json:"loss"`
	Total int     `json:"total"`
	Lost  int     `json:"lost"`
}

// Open 打开（并初始化）数据库。
// 针对低功耗设备（OpenWrt/ARM）做了以下调优：
//   - wal_autocheckpoint=256：WAL 最多滞留约 1MB（默认 1000 页 ≈ 4MB）
//   - journal_size_limit=1MB：checkpoint 后截断 WAL，避免持续占用闪存
//   - cache_size=-1024：页缓存压到 1MB（默认 2MB）
//   - temp_store=memory：临时表放内存，避免写闪存
func Open(path string) (*Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 写入串行化，单连接可彻底避免 SQLITE_BUSY。
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	// 低功耗设备（OpenWrt/ARM）的额外调优。单连接下这些 PRAGMA 对后续查询全部生效。
	for _, pragma := range []string{
		`PRAGMA wal_autocheckpoint = 256`,     // WAL 最多滞留约 1MB（默认约 4MB）
		`PRAGMA journal_size_limit = 1048576`, // checkpoint 后截断 WAL，避免长期占用闪存
		`PRAGMA cache_size = -1024`,           // 页缓存 1MB（默认 2MB）
		`PRAGMA temp_store = MEMORY`,          // 临时表放内存，不写闪存
		`PRAGMA optimize`,                     // 让查询规划器收集统计信息
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("应用 %s 失败: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化表结构失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrate 为旧版本数据库补齐新增列。
func (s *Store) migrate() error {
	rows, err := s.db.Query(`PRAGMA table_info(targets)`)
	if err != nil {
		return fmt.Errorf("读取表结构失败: %w", err)
	}
	hasPacketSize := false
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == "packet_size" {
			hasPacketSize = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	if hasPacketSize {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE targets ADD COLUMN packet_size INTEGER NOT NULL DEFAULT 32`); err != nil {
		return fmt.Errorf("升级表结构失败: %w", err)
	}
	return nil
}

func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// Close 关闭数据库。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// AddTarget 新建监测目标。
func (s *Store) AddTarget(name, host string, intervalSec, packetSize int) (*Target, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(
		`INSERT INTO targets (name, host, interval_sec, packet_size, enabled, created_at) VALUES (?, ?, ?, ?, 1, ?)`,
		name, host, intervalSec, packetSize, now,
	)
	if err != nil {
		return nil, fmt.Errorf("写入目标失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetTarget(id)
}

// GetTarget 按 ID 读取目标。
func (s *Store) GetTarget(id int64) (*Target, error) {
	row := s.db.QueryRow(
		`SELECT id, name, host, interval_sec, packet_size, enabled, created_at FROM targets WHERE id = ?`, id)
	return scanTarget(row)
}

// ListTargets 列出全部目标（按创建顺序）。
func (s *Store) ListTargets() ([]Target, error) {
	rows, err := s.db.Query(
		`SELECT id, name, host, interval_sec, packet_size, enabled, created_at FROM targets ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询目标失败: %w", err)
	}
	defer rows.Close()

	out := make([]Target, 0, 8)
	for rows.Next() {
		row := rows
		t, err := scanTarget(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// TargetPatch 描述目标的可选更新字段。
type TargetPatch struct {
	Name        *string
	Host        *string
	IntervalSec *int
	PacketSize  *int
	Enabled     *bool
}

// UpdateTarget 局部更新目标。
func (s *Store) UpdateTarget(id int64, p TargetPatch) (*Target, error) {
	sets := make([]string, 0, 5)
	args := make([]any, 0, 6)
	if p.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *p.Name)
	}
	if p.Host != nil {
		sets = append(sets, "host = ?")
		args = append(args, *p.Host)
	}
	if p.IntervalSec != nil {
		sets = append(sets, "interval_sec = ?")
		args = append(args, *p.IntervalSec)
	}
	if p.PacketSize != nil {
		sets = append(sets, "packet_size = ?")
		args = append(args, *p.PacketSize)
	}
	if p.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolToInt(*p.Enabled))
	}
	if len(sets) == 0 {
		return s.GetTarget(id)
	}
	args = append(args, id)
	res, err := s.db.Exec(`UPDATE targets SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("更新目标失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetTarget(id)
}

// DeleteTarget 删除目标及其历史数据。
func (s *Store) DeleteTarget(id int64) error {
	res, err := s.db.Exec(`DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除目标失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertProbe 写入一次探测结果。
func (s *Store) InsertProbe(targetID int64, ts int64, rtt *float64, ok bool) error {
	var value any
	if ok && rtt != nil {
		value = *rtt
	}
	_, err := s.db.Exec(
		`INSERT INTO probes (target_id, ts, rtt_ms, ok) VALUES (?, ?, ?, ?)`,
		targetID, ts, value, boolToInt(ok),
	)
	return err
}

// RecentProbes 返回最近 n 条探测记录（时间倒序）。
func (s *Store) RecentProbes(targetID int64, limit int) ([]Probe, error) {
	rows, err := s.db.Query(
		`SELECT ts, rtt_ms, ok FROM probes WHERE target_id = ? ORDER BY ts DESC LIMIT ?`, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Probe, 0, limit)
	for rows.Next() {
		var (
			ts  int64
			rtt sql.NullFloat64
			ok  int
		)
		if err := rows.Scan(&ts, &rtt, &ok); err != nil {
			return nil, err
		}
		p := Probe{TS: ts, OK: ok == 1}
		if rtt.Valid {
			v := rtt.Float64
			p.RTT = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Series 返回按桶聚合的时间序列，用于折线图。
// 每个桶内计算成功样本的平均延迟与丢包比例；全部失败时 RTT 为 null。
// Series 按时间桶聚合单个目标的曲线数据。
//
// 注意：bucket 的大小由 (until - since) / points 决定，必须由调用方传入同一个 until。
// 早期实现在函数内部各自取 time.Now()，多个目标串行查询时若耗时跨越整数边界，
// 各目标会算出相差 1ms 的桶宽、时间栅格错开，前端叠加后每行只命中一个目标，
// 折线被打散成孤立点。
func (s *Store) Series(targetID int64, since, until int64, points int) ([]Point, error) {
	if points < 10 {
		points = 10
	}
	span := until - since
	if span <= 0 {
		span = int64(points) * 1000
	}
	bucket := span / int64(points)
	if bucket < 1000 {
		bucket = 1000
	}

	rows, err := s.db.Query(`
		SELECT (ts / ?1) * ?1 AS b,
		       AVG(rtt_ms) AS avg_rtt,
		       MAX(rtt_ms) AS max_rtt,
		       COUNT(*) AS total,
		       SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END) AS lost
		FROM probes
		WHERE target_id = ?2 AND ts >= ?3
		GROUP BY b
		ORDER BY b`, bucket, targetID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Point, 0, points)
	for rows.Next() {
		var (
			b      int64
			avg    sql.NullFloat64
			maxRtt sql.NullFloat64
			n      int
			lost   sql.NullInt64
		)
		if err := rows.Scan(&b, &avg, &maxRtt, &n, &lost); err != nil {
			return nil, err
		}
		p := Point{T: b, N: n}
		if avg.Valid {
			v := avg.Float64
			p.RTT = &v
		}
		if maxRtt.Valid {
			v := maxRtt.Float64
			p.Max = &v
		}
		if n > 0 {
			p.Loss = float64(lost.Int64) / float64(n)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Stats 统计区间内的延迟与丢包。
// withMaxTS 为 true 时会额外定位峰值出现时刻；该查询需要排序扫描，
// 在长区间（如 7 天、数十万行）上代价较高，调用方应按区间长度决定是否启用。
func (s *Store) Stats(targetID int64, since int64, withMaxTS bool) (Stats, error) {
	var (
		avg, min, max sql.NullFloat64
		total, lost   sql.NullInt64
	)
	err := s.db.QueryRow(`
		SELECT AVG(rtt_ms), MIN(rtt_ms), MAX(rtt_ms), COUNT(*),
		       SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END)
		FROM probes
		WHERE target_id = ? AND ts >= ?`, targetID, since).
		Scan(&avg, &min, &max, &total, &lost)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Stats{}, err
	}

	st := Stats{Total: int(total.Int64), Lost: int(lost.Int64)}
	if avg.Valid {
		v := avg.Float64
		st.Avg = &v
	}
	if min.Valid {
		v := min.Float64
		st.Min = &v
	}
	if max.Valid {
		v := max.Float64
		st.Max = &v

		if withMaxTS {
			// 定位最大值出现的时刻，供界面展示"最高"对应的时间点。
			var maxTS sql.NullInt64
			if qErr := s.db.QueryRow(
				`SELECT ts FROM probes WHERE target_id = ? AND ts >= ? AND ok = 1 ORDER BY rtt_ms DESC, ts DESC LIMIT 1`,
				targetID, since,
			).Scan(&maxTS); qErr == nil && maxTS.Valid {
				st.MaxTS = maxTS.Int64
			}
		}
	}
	if st.Total > 0 {
		st.Loss = float64(st.Lost) / float64(st.Total)
	}
	return st, nil
}

// ResetProbes 删除全部探测记录，保留目标配置，并回收 WAL 文件。
func (s *Store) ResetProbes() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM probes`)
	if err != nil {
		return 0, fmt.Errorf("清空历史数据失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		log.Printf("回收 WAL 失败: %v", err)
	}
	return n, nil
}

// Prune 分批删除超过保留期的探测记录。
// 每批 2000 行，避免在低性能设备上产生长时间写锁；返回删除总数。
func (s *Store) Prune(olderThan time.Time) (int64, error) {
	const batch = 2000
	cutoff := olderThan.UnixMilli()
	total := int64(0)

	for i := 0; i < 200; i++ {
		res, err := s.db.Exec(
			`DELETE FROM probes WHERE id IN (SELECT id FROM probes WHERE ts < ? LIMIT ?)`,
			cutoff, batch,
		)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < batch {
			break
		}
		// 让出时间片，避免长时间占用写锁。
		time.Sleep(20 * time.Millisecond)
	}
	if total > 0 {
		_, _ = s.db.Exec(`PRAGMA optimize`)
	}
	return total, nil
}

// PingCount 返回历史探测记录总数。
func (s *Store) PingCount() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM probes`).Scan(&n)
	return n, err
}

func scanTarget(sc interface{ Scan(...any) error }) (*Target, error) {
	var (
		t       Target
		enabled int
	)
	if err := sc.Scan(&t.ID, &t.Name, &t.Host, &t.IntervalSec, &t.PacketSize, &enabled, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.Enabled = enabled == 1
	return &t, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
