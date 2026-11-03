package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pinghistory/internal/api"
	"pinghistory/internal/config"
	"pinghistory/internal/osutil"
	"pinghistory/internal/pinger"
	"pinghistory/internal/store"
	"pinghistory/internal/tray"
)

//go:embed all:web/dist
var webAssets embed.FS

//go:embed assets/app.ico
var appIcon []byte

const version = "1.0.0"

func main() {
	// 路由/服务器等无桌面环境既没有托盘也没有浏览器，默认关闭。
	isDesktop := runtime.GOOS == "windows"
	var (
		portFlag   = flag.Int("port", 0, "Web 控制台端口（覆盖 config.json 中的设置）")
		dataFlag   = flag.String("data", "", "数据目录（默认使用可执行文件同级的 data 目录）")
		listenFlag = flag.String("listen", "", "监听地址（默认读取 config.json，缺省 127.0.0.1；局域网访问设为 0.0.0.0）")
		noTray     = flag.Bool("no-tray", !isDesktop, "不创建系统托盘图标（非 Windows 平台默认开启）")
		noBrowser  = flag.Bool("no-browser", !isDesktop, "启动时不自动打开浏览器（非 Windows 平台默认开启）")
	)
	flag.Parse()

	dataDir, err := resolveDataDir(*dataFlag)
	if err != nil {
		fatal("解析数据目录失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o755); err != nil {
		fatal("创建数据目录失败: %v", err)
	}
	setupLogging(filepath.Join(dataDir, "logs", "app.log"))

	cfg, cfgErr := config.Load(filepath.Join(dataDir, "config.json"))
	if cfgErr != nil {
		log.Printf("配置读取异常，已使用默认值: %v", cfgErr)
	}
	if *portFlag > 0 {
		cfg.Port = *portFlag
	}
	if *listenFlag != "" {
		cfg.Listen = *listenFlag
	}

	// 单实例：已有实例在运行时直接唤起浏览器。
	if !*noTray && !tray.AcquireSingleInstance(fmt.Sprintf("Local\\PingHistory-%d", cfg.Port)) {
		log.Printf("检测到 %d 端口的实例已在运行，直接打开控制台", cfg.Port)
		if !*noBrowser {
			_ = osutil.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d", cfg.Port))
		}
		return
	}

	st, err := store.Open(filepath.Join(dataDir, "pinghistory.db"))
	if err != nil {
		fatal("打开数据库失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	pm := pinger.New(st)
	defer pm.Stop()
	pm.Sync()

	dist, err := fs.Sub(webAssets, "web/dist")
	if err != nil {
		fatal("加载前端资源失败: %v", err)
	}

	ln, port, err := listenOn(cfg.Listen, cfg.Port)
	if err != nil {
		fatal("%v", err)
	}
	cfgMgr := config.NewManager(filepath.Join(dataDir, "config.json"), cfg)
	if port != cfg.Port {
		log.Printf("端口 %d 不可用，已改用 %d", cfg.Port, port)
		if _, saveErr := cfgMgr.SetPort(port); saveErr != nil {
			log.Printf("保存配置失败: %v", saveErr)
		}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	srv := &http.Server{
		Handler:           api.New(st, pm, dist, version, port, cfgMgr),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serverErr <- serveErr
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go pruneLoop(ctx, st, cfgMgr)

	quit := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(quit)
	}()

	if cfg.OpenBrowser && !*noBrowser {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if openErr := osutil.OpenBrowser(url); openErr != nil {
				log.Printf("自动打开浏览器失败（可手动访问 %s）: %v", url, openErr)
			}
		}()
	}

	iconPath := ""
	if !*noTray {
		// 只有需要托盘时才把图标释放到数据目录，避免在路由上白占闪存。
		iconPath = ensureIcon(dataDir)
	}
	if !isLoopbackListen(cfg.Listen) {
		log.Printf("已监听 %s:%d，局域网可通过 http://<本机IP>:%d 访问（控制台未设鉴权）", cfg.Listen, port, port)
	}
	log.Printf("PingHistory %s 已启动 · 控制台 %s · 数据目录 %s", version, url, dataDir)

	if !*noTray {
		if trayErr := tray.Run(tray.Options{
			URL:      url,
			IconPath: iconPath,
			DataDir:  dataDir,
			OnQuit:   cancel,
		}); trayErr != nil {
			log.Printf("系统托盘不可用，继续以后台模式运行: %v", trayErr)
		}
	}

	select {
	case err := <-serverErr:
		log.Printf("HTTP 服务异常退出: %v", err)
	case <-quit:
	}

	log.Printf("正在退出…")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭 HTTP 服务: %v", err)
	}
	log.Printf("已退出")
}

// resolveDataDir 决定数据目录：优先命令行参数，其次可执行文件同级目录。
// 在 go run / 临时目录下运行时退回当前工作目录，避免数据写进临时构建目录。
func resolveDataDir(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		low := strings.ToLower(dir)
		if !strings.Contains(low, "go-build") && !strings.Contains(low, "\\temp\\") {
			// 构建产物统一放在 dist/ 下，数据目录仍落在其父目录，
			// 这样从 dist\PingHistory.exe 启动时不会另起一份空白数据。
			if strings.EqualFold(filepath.Base(dir), "dist") {
				dir = filepath.Dir(dir)
			}
			return filepath.Join(dir, "data"), nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, "data"), nil
}

// isLoopbackListen 判断监听地址是否只绑定回环地址。
func isLoopbackListen(addr string) bool {
	switch addr {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	}
	return false
}

// listenOn 从首选端口开始尝试绑定指定地址，最多顺延 30 个端口。
func listenOn(host string, preferred int) (net.Listener, int, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if preferred <= 0 || preferred > 65535 {
		preferred = 8787
	}
	last := preferred + 29
	if last > 65535 {
		last = 65535
	}
	for p := preferred; p <= last; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			return ln, p, nil
		}
	}
	return nil, 0, fmt.Errorf("端口 %d-%d 在 %s 上均被占用，请在 config.json 中修改 port", preferred, last, host)
}

// ensureIcon 把内嵌图标释放到数据目录，供 Shell_NotifyIcon 加载。
func ensureIcon(dataDir string) string {
	path := filepath.Join(dataDir, "app.ico")
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(appIcon)) {
		return path
	}
	if err := os.WriteFile(path, appIcon, 0o644); err != nil {
		log.Printf("释放托盘图标失败: %v", err)
		return ""
	}
	return path
}

func pruneLoop(ctx context.Context, st *store.Store, mgr *config.Manager) {
	// 低存储设备上更频繁地小批量清理，避免单次删除大量数据造成长时间写锁。
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	prune := func() {
		// 保留天数可能随时被界面修改，每轮都重新读取；0 表示永久保留。
		days := mgr.Get().RetentionDays
		if days <= 0 {
			return
		}
		before := time.Now().AddDate(0, 0, -days)
		if n, err := st.Prune(before); err != nil {
			log.Printf("清理历史数据失败: %v", err)
		} else if n > 0 {
			log.Printf("已清理 %d 条超过 %d 天的探针记录", n, days)
		}
	}
	prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}

func setupLogging(path string) {
	// 日志超过 2MB 时轮转一份，避免长期运行把闪存写满。
	if info, err := os.Stat(path); err == nil && info.Size() > 2<<20 {
		rotated := path + ".1"
		_ = os.Remove(rotated)
		_ = os.Rename(path, rotated)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.SetOutput(os.Stderr)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[pinghistory] ")
}

func fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("致命错误: %s", msg)
	os.Exit(1)
}
