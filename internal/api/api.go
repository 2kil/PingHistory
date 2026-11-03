// Package api 提供 Web 控制台所需的 HTTP 接口与静态资源服务。
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"pinghistory/internal/config"
	"pinghistory/internal/pinger"
	"pinghistory/internal/store"
)

// hostPattern 限制目标主机名/IP 的字符集，避免非法输入。
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9._:\[\]%-]+$`)

// API 组合了数据层与业务层。
type API struct {
	st      *store.Store
	pm      *pinger.Manager
	assets  fs.FS
	version string
	port    int
	cfg     *config.Manager
	started time.Time

	// 概览接口结果缓存：前端每 3 秒轮询一次，缓存 2 秒可显著降低低功耗设备上的查询开销。
	cacheMu sync.Mutex
	cache   map[string]cacheEntry
}

type cacheEntry struct {
	at   time.Time
	body []byte
}

const overviewCacheTTL = 2 * time.Second

// cacheTTL 按区间长度决定缓存时长：区间越长，聚合扫描越贵，但数据变化越不明显，
// 允许更长的缓存以显著降低低功耗设备上的 CPU 占用。
func cacheTTL(rangeSec int) time.Duration {
	switch {
	case rangeSec > 24*3600:
		return 20 * time.Second
	case rangeSec > 6*3600:
		return 12 * time.Second
	case rangeSec > 3600:
		return 6 * time.Second
	default:
		return overviewCacheTTL
	}
}

// New 创建 HTTP 处理器。assets 为前端构建产物（web/dist）。
func New(st *store.Store, pm *pinger.Manager, assets fs.FS, version string, port int, cfg *config.Manager) http.Handler {
	a := &API{
		st:      st,
		pm:      pm,
		assets:  assets,
		version: version,
		port:    port,
		cfg:     cfg,
		started: time.Now(),
		cache:   make(map[string]cacheEntry),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/overview", a.handleOverview)
	mux.HandleFunc("GET /api/targets", a.handleListTargets)
	mux.HandleFunc("POST /api/targets", a.handleCreateTarget)
	mux.HandleFunc("PATCH /api/targets/{id}", a.handleUpdateTarget)
	mux.HandleFunc("DELETE /api/targets/{id}", a.handleDeleteTarget)
	mux.HandleFunc("GET /api/targets/{id}/series", a.handleSeries)
	mux.HandleFunc("GET /api/targets/{id}/probes", a.handleProbes)
	mux.HandleFunc("POST /api/reset", a.handleReset)
	mux.HandleFunc("PATCH /api/settings", a.handleUpdateSettings)
	mux.HandleFunc("/", a.handleSPA)

	return logRequests(mux)
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	total, _ := a.st.PingCount()
	cfg := a.cfg.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        a.version,
		"port":           a.port,
		"uptime_sec":     int(time.Since(a.started).Seconds()),
		"total_probes":   total,
		"retention_days": cfg.RetentionDays,
		"time":           time.Now().UnixMilli(),
	})
}

type targetView struct {
	store.Target
	Last   *store.Probe  `json:"last"`
	Stats  store.Stats   `json:"stats"`
	Series []store.Point `json:"series"`
}

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	rangeSec := clampInt(intQuery(r, "range", 900), 60, 7*24*3600)
	points := clampInt(intQuery(r, "points", 240), 20, 2000)

	key := fmt.Sprintf("%d:%d", rangeSec, points)
	if body, ok := a.fromCache(key, cacheTTL(rangeSec)); ok {
		writeRawJSON(w, body)
		return
	}

	// 同一次请求内固定 until，保证所有目标使用完全一致的时间桶栅格。
	now := time.Now().UnixMilli()
	since := now - int64(rangeSec)*1000

	targets, err := a.st.ListTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	views := make([]targetView, 0, len(targets))
	for _, t := range targets {
		v := targetView{Target: t, Series: []store.Point{}}
		if stats, sErr := a.st.Stats(t.ID, since, rangeSec <= 6*3600); sErr == nil {
			v.Stats = stats
		}
		if series, sErr := a.st.Series(t.ID, since, now, points); sErr == nil {
			v.Series = series
		}
		if last, ok := a.pm.Last(t.ID); ok {
			lastCopy := last
			v.Last = &lastCopy
		}
		views = append(views, v)
	}

	payload := map[string]any{
		"server_time": time.Now().UnixMilli(),
		"range_sec":   rangeSec,
		"targets":     views,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.toCache(key, body)
	writeRawJSON(w, body)
}

// fromCache / toCache 维护概览接口的短期缓存。
func (a *API) fromCache(key string, ttl time.Duration) ([]byte, bool) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	entry, ok := a.cache[key]
	if !ok || time.Since(entry.at) > ttl {
		return nil, false
	}
	return entry.body, true
}

func (a *API) toCache(key string, body []byte) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	// 只保留最近使用的少量键，避免缓存无限增长。
	if len(a.cache) > 8 {
		a.cache = make(map[string]cacheEntry)
	}
	a.cache[key] = cacheEntry{at: time.Now(), body: body}
}

// invalidateCache 在目标增删改或重置后调用，保证界面立刻看到新数据。
func (a *API) invalidateCache() {
	a.cacheMu.Lock()
	a.cache = make(map[string]cacheEntry)
	a.cacheMu.Unlock()
}

func writeRawJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (a *API) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := a.st.ListTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

type createTargetRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	IntervalSec int    `json:"interval_sec"`
	PacketSize  int    `json:"packet_size"`
}

func (a *API) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var req createTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}

	host, msg := normalizeHost(req.Host)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = host
	}
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}

	interval := req.IntervalSec
	if interval == 0 {
		interval = 5
	}
	interval = clampInt(interval, 1, 3600)

	size := req.PacketSize
	if size == 0 {
		size = 32
	}
	size = clampInt(size, 1, 65000)

	t, err := a.st.AddTarget(name, host, interval, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.pm.Sync()
	a.invalidateCache()
	writeJSON(w, http.StatusCreated, t)
}

type updateTargetRequest struct {
	Name        *string `json:"name"`
	Host        *string `json:"host"`
	IntervalSec *int    `json:"interval_sec"`
	PacketSize  *int    `json:"packet_size"`
	Enabled     *bool   `json:"enabled"`
}

func (a *API) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "目标 ID 不合法")
		return
	}
	var req updateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}

	patch := store.TargetPatch{Enabled: req.Enabled}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "名称不能为空")
			return
		}
		if len([]rune(name)) > 64 {
			name = string([]rune(name)[:64])
		}
		patch.Name = &name
	}
	if req.Host != nil {
		host, msg := normalizeHost(*req.Host)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		patch.Host = &host
	}
	if req.IntervalSec != nil {
		v := clampInt(*req.IntervalSec, 1, 3600)
		patch.IntervalSec = &v
	}
	if req.PacketSize != nil {
		v := clampInt(*req.PacketSize, 1, 65000)
		patch.PacketSize = &v
	}

	t, err := a.st.UpdateTarget(id, patch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.pm.Sync()
	a.invalidateCache()
	writeJSON(w, http.StatusOK, t)
}

// normalizeHost 校验并规整主机地址，返回错误提示（空串表示通过）。
func normalizeHost(raw string) (string, string) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", "请填写要监测的主机（域名或 IP）"
	}
	if len(host) > 253 || !hostPattern.MatchString(host) || strings.HasPrefix(host, "-") {
		return "", "主机格式不合法，仅支持域名或 IP 地址"
	}
	return host, ""
}

func (a *API) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "目标 ID 不合法")
		return
	}
	if err := a.st.DeleteTarget(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.pm.Sync()
	a.invalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (a *API) handleSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "目标 ID 不合法")
		return
	}
	rangeSec := clampInt(intQuery(r, "range", 3600), 60, 7*24*3600)
	points := clampInt(intQuery(r, "points", 240), 20, 2000)
	now := time.Now().UnixMilli()
	since := now - int64(rangeSec)*1000

	series, err := a.st.Series(id, since, now, points)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats, _ := a.st.Stats(id, since, rangeSec <= 6*3600)
	writeJSON(w, http.StatusOK, map[string]any{"series": series, "stats": stats})
}

func (a *API) handleProbes(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "目标 ID 不合法")
		return
	}
	limit := clampInt(intQuery(r, "limit", 30), 1, 500)
	probes, err := a.st.RecentProbes(id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"probes": probes})
}

type resetRequest struct {
	// BeforeDays 为清除范围：>0 表示只删除 N 天之前的记录，0 表示清空全部。
	BeforeDays int `json:"before_days"`
}

// handleReset 按范围删除探测历史。
// before_days = 0 时清空全部记录，并让所有目标从同一时刻重新开始采集；
// before_days > 0 时只删除该天数之前的记录，不影响正在进行的采集。
func (a *API) handleReset(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	// 允许空请求体（等价于全部清空）。
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.BeforeDays < 0 {
		req.BeforeDays = 0
	}

	var (
		deleted int64
		err     error
	)
	if req.BeforeDays == 0 {
		deleted, err = a.pm.Reset()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		log.Printf("统计已重置：清除 %d 条探测记录，全部目标重新开始采集", deleted)
	} else {
		cutoff := time.Now().AddDate(0, 0, -req.BeforeDays)
		deleted, err = a.st.Prune(cutoff)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		log.Printf("已删除 %d 天前的探测记录：%d 条", req.BeforeDays, deleted)
	}

	a.invalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":     deleted,
		"before_days": req.BeforeDays,
		"reset_at":    time.Now().UnixMilli(),
	})
}

type settingsRequest struct {
	RetentionDays *int `json:"retention_days"`
}

// handleUpdateSettings 更新运行设置并持久化。目前仅支持自动保留天数。
// retention_days = 0 表示永久保留。修改后立即按新策略清理一次超期数据。
func (a *API) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}
	if req.RetentionDays == nil {
		writeError(w, http.StatusBadRequest, "没有需要更新的设置")
		return
	}

	cfg, err := a.cfg.SetRetentionDays(*req.RetentionDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.invalidateCache()
	log.Printf("自动保留天数已更新为 %d（0 表示永久）", cfg.RetentionDays)

	if cfg.RetentionDays > 0 {
		// 立即生效：异步清理一次超期数据，避免等待下一轮定时任务。
		days := cfg.RetentionDays
		go func() {
			if n, pErr := a.st.Prune(time.Now().AddDate(0, 0, -days)); pErr == nil && n > 0 {
				log.Printf("按新的保留策略清理了 %d 条历史记录", n)
				a.invalidateCache()
			}
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"retention_days": cfg.RetentionDays})
}

// handleSPA 托管前端单页应用，未命中的路由回退到 index.html。
func (a *API) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}

	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}

	data, err := fs.ReadFile(a.assets, name)
	if err != nil {
		data, err = fs.ReadFile(a.assets, "index.html")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "前端资源缺失，请先构建 web 目录")
			return
		}
		name = "index.html"
	}

	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func pathID(r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func intQuery(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && rec.status >= 400 {
			log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}
