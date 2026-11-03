package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Config 是持久化在数据目录中的运行配置。
type Config struct {
	// Port 为 Web 控制台端口，默认 8787。
	Port int `json:"port"`
	// Listen 为监听地址。默认 0.0.0.0（所有网卡，便于局域网访问）；
	// 若只想本机访问，改为 127.0.0.1。
	Listen string `json:"listen"`
	// OpenBrowser 控制启动时是否自动打开浏览器。
	OpenBrowser bool `json:"open_browser"`
	// RetentionDays 为探测历史保留天数，0 表示永久保留，默认 7 天。
	RetentionDays int `json:"retention_days"`
}

func defaults() Config {
	return Config{
		Port: 8787,
		// 默认监听所有网卡，方便用手机/其他电脑打开控制台。
		// 控制台没有鉴权，如需限制访问请改为 127.0.0.1。
		Listen: "0.0.0.0",
		// 路由等无桌面环境没有浏览器可开，默认关闭。
		OpenBrowser:   runtime.GOOS == "windows",
		RetentionDays: 7,
	}
}

// Load 读取配置文件；文件不存在时写入默认配置。
func Load(path string) (Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, Save(path, cfg)
		}
		return cfg, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaults(), fmt.Errorf("解析配置文件失败: %w", err)
	}
	cfg.normalize()

	// 旧版本配置文件缺少新增字段（如 listen），补齐后写回，便于用户看到并修改当前生效值。
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["listen"]; !ok {
			if err := Save(path, cfg); err != nil {
				return cfg, nil // 写回失败不影响启动
			}
		}
	}
	return cfg, nil
}

// Save 原子写入配置文件。
func Save(path string, cfg Config) error {
	cfg.normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Config) normalize() {
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = 8787
	}
	if c.Listen == "" {
		c.Listen = "0.0.0.0"
	}
	if c.RetentionDays < 0 {
		c.RetentionDays = 7
	}
	if c.RetentionDays > 365 {
		c.RetentionDays = 365
	}
}

// Manager 持有运行期配置，提供并发安全的读取与持久化更新。
type Manager struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// NewManager 用已加载的配置创建管理器。
func NewManager(path string, cfg Config) *Manager {
	return &Manager{path: path, cfg: cfg}
}

// Get 返回当前配置的副本。
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// SetRetentionDays 更新保留天数并立即写盘；0 表示永久保留。
func (m *Manager) SetRetentionDays(days int) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if days < 0 {
		days = 0
	}
	if days > 365 {
		days = 365
	}
	m.cfg.RetentionDays = days
	if err := Save(m.path, m.cfg); err != nil {
		return m.cfg, err
	}
	return m.cfg, nil
}

// SetPort 仅在端口顺延时使用，写回配置文件。
func (m *Manager) SetPort(port int) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.Port = port
	if err := Save(m.path, m.cfg); err != nil {
		return m.cfg, err
	}
	return m.cfg, nil
}
