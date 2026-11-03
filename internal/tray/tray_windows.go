//go:build windows

// Package tray 以 Win32 Shell_NotifyIcon 原生实现系统托盘图标：
// 左键单击/双击打开 Web 控制台，右键弹出菜单（打开控制台 / 打开数据目录 / 退出）。
package tray

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"pinghistory/internal/osutil"
)

const (
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmNull    = 0x0000

	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	wmApp          = 0x8000
	wmTrayCallback = wmApp + 1

	nimAdd     = 0x00000000
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	imageIcon      = 1
	lrLoadFromFile = 0x00000010

	mfString       = 0x00000000
	mfSeparator    = 0x00000800
	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020
	tpmReturnCmd   = 0x0100

	idiApplication = 32512
	smCxSmIcon     = 49

	trayIconID      = 1
	menuOpenCommand = 1001
	menuOpenDataDir = 1002
	menuQuitApp     = 1003
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type pointT struct {
	X int32
	Y int32
}

type msgT struct {
	hwnd     windows.Handle
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       pointT
	lPrivate uint32
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             windows.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     windows.Handle
}

// Options 是托盘运行参数。
type Options struct {
	// URL 为 Web 控制台地址。
	URL string
	// IconPath 为 .ico 文件路径。
	IconPath string
	// DataDir 为数据目录（右键菜单可打开）。
	DataDir string
	// OnQuit 在用户选择"退出"时回调。
	OnQuit func()
}

type trayState struct {
	opts   Options
	hwnd   windows.Handle
	nid    notifyIconDataW
	icon   windows.Handle
	mu     sync.Mutex
	exited bool
}

var active *trayState

// Run 创建托盘图标并进入消息循环，直到用户选择退出。返回错误表示托盘不可用。
func Run(opts Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	st := &trayState{opts: opts}
	active = st

	if err := st.init(); err != nil {
		return err
	}
	defer st.cleanup()

	st.loop()

	if opts.OnQuit != nil {
		opts.OnQuit()
	}
	return nil
}

func (s *trayState) init() error {
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	className, err := syscall.UTF16PtrFromString("PingHistoryTrayWindow")
	if err != nil {
		return err
	}

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProcAddr,
		hInstance:     windows.Handle(hInstance),
		lpszClassName: className,
	}
	if atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return fmt.Errorf("注册窗口类失败: %v", regErr)
	}

	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0, 0, 0, 0,
		0, 0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("创建托盘消息窗口失败: %v", createErr)
	}
	s.hwnd = windows.Handle(hwnd)

	s.icon = loadIcon(s.opts.IconPath)

	s.nid = notifyIconDataW{
		cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:             s.hwnd,
		uID:              trayIconID,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayCallback,
		hIcon:            s.icon,
	}
	tip := "PingHistory · " + s.opts.URL
	copy(s.nid.szTip[:len(s.nid.szTip)-1], windows.StringToUTF16(tip))

	if ret, _, notifyErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&s.nid))); ret == 0 {
		return fmt.Errorf("添加托盘图标失败: %v", notifyErr)
	}
	log.Printf("系统托盘已就绪（左键打开控制台，右键菜单退出）")
	return nil
}

func loadIcon(path string) windows.Handle {
	if path != "" {
		size, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
		if size == 0 {
			size = 16
		}
		pathPtr, err := syscall.UTF16PtrFromString(path)
		if err == nil {
			h, _, _ := procLoadImageW.Call(0,
				uintptr(unsafe.Pointer(pathPtr)),
				imageIcon, size, size, lrLoadFromFile)
			if h != 0 {
				return windows.Handle(h)
			}
		}
	}
	h, _, _ := procLoadIconW.Call(0, idiApplication)
	return windows.Handle(h)
}

func (s *trayState) loop() {
	var msg msgT
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(ret) {
		case -1, 0:
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (s *trayState) cleanup() {
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&s.nid)))
	if s.hwnd != 0 {
		procDestroyWindow.Call(uintptr(s.hwnd))
	}
}

func (s *trayState) openConsole() {
	if err := osutil.OpenBrowser(s.opts.URL); err != nil {
		log.Printf("打开浏览器失败: %v", err)
	}
}

func (s *trayState) quit() {
	s.mu.Lock()
	if s.exited {
		s.mu.Unlock()
		return
	}
	s.exited = true
	s.mu.Unlock()

	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&s.nid)))
	procPostMessageW.Call(uintptr(s.hwnd), wmClose, 0, 0)
}

func (s *trayState) showMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	appendMenu(hMenu, mfString, menuOpenCommand, "打开控制台")
	appendMenu(hMenu, mfString, menuOpenDataDir, "打开数据目录")
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	appendMenu(hMenu, mfString, menuQuitApp, "退出 PingHistory")

	var pt pointT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// 必须先置前台，否则点击别处时菜单不会消失。
	procSetForegroundWindow.Call(uintptr(s.hwnd))
	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		tpmRightButton|tpmBottomAlign|tpmReturnCmd,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(s.hwnd),
		0,
	)
	procPostMessageW.Call(uintptr(s.hwnd), wmNull, 0, 0)

	switch int(cmd) {
	case menuOpenCommand:
		s.openConsole()
	case menuOpenDataDir:
		if err := osutil.OpenPath(s.opts.DataDir); err != nil {
			log.Printf("打开数据目录失败: %v", err)
		}
	case menuQuitApp:
		s.quit()
	}
}

func appendMenu(hMenu uintptr, flags uintptr, id uintptr, text string) {
	ptr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procAppendMenuW.Call(hMenu, flags, id, uintptr(unsafe.Pointer(ptr)))
}

var wndProcAddr uintptr

func init() {
	wndProcAddr = syscall.NewCallback(wndProc)
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	st := active
	if st == nil {
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return ret
	}
	switch message {
	case wmTrayCallback:
		switch uint32(lParam) & 0xFFFF {
		case wmLButtonUp, wmLButtonDblClk:
			st.openConsole()
			return 0
		case wmRButtonUp:
			st.showMenu()
			return 0
		}
	case wmClose:
		procPostQuitMessage.Call(0)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

var (
	mutexOnce sync.Mutex
	mutexHeld windows.Handle
)

// AcquireSingleInstance 通过命名互斥体保证同一端口只有一个实例。
// 返回 false 表示已有实例在运行。
func AcquireSingleInstance(name string) bool {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return true
	}

	// 先尝试打开已存在的同名内核对象：能打开说明已有实例。
	if h, openErr := windows.OpenMutex(windows.SYNCHRONIZE, false, ptr); openErr == nil {
		_ = windows.CloseHandle(h)
		return false
	}

	h, createErr := windows.CreateMutex(nil, false, ptr)
	if h == 0 {
		log.Printf("创建互斥体失败，跳过单实例检查: %v", createErr)
		return true
	}
	// 对象已被其他进程创建时，CreateMutex 仍返回有效句柄，但最后错误码为 ERROR_ALREADY_EXISTS。
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(h)
		return false
	}

	mutexOnce.Lock()
	mutexHeld = h
	mutexOnce.Unlock()
	return true
}

// ErrUnsupported 在非 Windows 平台返回。
var ErrUnsupported = errors.New("当前平台不支持系统托盘")
