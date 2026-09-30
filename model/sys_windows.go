//go:build windows

package model

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// 本文件只使用 Go 标准库的 syscall 调用 Windows API，
// 不引入任何第三方依赖，保持"单文件交付、零外部依赖"的既有形态。

var (
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")
	shell32DLL  = syscall.NewLazyDLL("shell32.dll")
	user32DLL   = syscall.NewLazyDLL("user32.dll")

	procCreateToolhelp32Snapshot = kernel32DLL.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32DLL.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32DLL.NewProc("Process32NextW")
	procCloseHandle              = kernel32DLL.NewProc("CloseHandle")
	procShellExecuteW            = shell32DLL.NewProc("ShellExecuteW")
	procMessageBoxTimeoutW       = user32DLL.NewProc("MessageBoxTimeoutW")
)

const (
	th32csSnapProcess  = 0x00000002
	invalidHandleValue = ^uintptr(0)
	swShownormal       = 1
	mbYesNo            = 0x00000004
	mbIconQuestion     = 0x00000020
	mbTopmost          = 0x00040000
	mbSetForeground    = 0x00010000
	idYes              = 6
	idTimeout          = 32000
	seErrMin           = 32 // ShellExecuteW 返回值 <= 32 表示失败
)

// processEntry32W 对应 Windows PROCESSENTRY32W 结构
type processEntry32W struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// IsAnyProcessRunning 判断给定进程名中是否有任意一个正在运行
// 返回是否命中，以及命中的进程名（未命中时为空）
func IsAnyProcessRunning(names []string) (bool, string) {
	if len(names) == 0 {
		return false, ""
	}
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		if s := strings.ToLower(strings.TrimSpace(n)); s != "" {
			want[s] = struct{}{}
		}
	}
	if len(want) == 0 {
		return false, ""
	}

	snapshot, _, _ := procCreateToolhelp32Snapshot.Call(uintptr(th32csSnapProcess), 0)
	if snapshot == invalidHandleValue || snapshot == 0 {
		return false, ""
	}
	defer procCloseHandle.Call(snapshot)

	var entry processEntry32W
	entry.Size = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	for ret != 0 {
		name := syscall.UTF16ToString(entry.ExeFile[:])
		if _, ok := want[strings.ToLower(name)]; ok {
			return true, name
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		ret, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	return false, ""
}

// shellExecuteW 调用 ShellExecuteW 以系统默认方式打开目标
func shellExecuteW(target string) error {
	verbPtr, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(targetPtr)),
		0,
		0,
		uintptr(swShownormal),
	)
	if ret <= seErrMin {
		return fmt.Errorf("ShellExecuteW 返回错误码 %d", ret)
	}
	return nil
}

// LaunchURL 唤起会议客户端 / 打开链接
// 优先使用 ShellExecuteW：它不经过 shell 解析，URL 中的 & 等字符不会被当成命令分隔符；
// 失败时回退到原有的 cmd /c start 方式，保证可用性不下降。
func LaunchURL(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("启动目标为空")
	}
	if err := shellExecuteW(target); err == nil {
		return nil
	} else {
		log.Printf("[启动] ShellExecute 失败(%v)，回退 cmd /c start", err)
	}
	// start 的第一个参数会被当作窗口标题，因此显式传空标题，避免误判
	return exec.Command("cmd", "/c", "start", "", target).Start()
}

// OpenConfigFile 在服务端本机打开配置文件
// 优先使用记事本：会议室终端上 .json 通常没有文件关联，若直接交给系统默认程序，
// Windows 会弹出"选择打开方式"对话框 —— 而服务是后台静默运行的，这个对话框没有人看得见。
// 记事本始终可用、行为确定、无对话框；记事本不可用时再回退系统默认程序。
func OpenConfigFile(path string) error {
	if path == "" {
		return fmt.Errorf("配置文件路径为空")
	}
	if err := exec.Command("notepad.exe", path).Start(); err == nil {
		return nil
	} else {
		log.Printf("[配置] 记事本打开失败(%v)，改用系统默认程序", err)
	}
	return shellExecuteW(path)
}

// ConfirmDialog 弹出"是/否"确认框，带超时
// 超时或弹窗不可用时按 defaultYes 处理 —— 保住"无人值守也能开会"这个立身之本
func ConfirmDialog(title, text string, timeoutSec int, defaultYes bool) bool {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	titlePtr, err1 := syscall.UTF16PtrFromString(title)
	textPtr, err2 := syscall.UTF16PtrFromString(text)
	if err1 != nil || err2 != nil {
		return defaultYes
	}

	ret, _, callErr := procMessageBoxTimeoutW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbYesNo|mbIconQuestion|mbTopmost|mbSetForeground),
		0,
		uintptr(timeoutSec*1000),
	)
	if ret == 0 {
		// 弹窗不可用（例如进程运行在非交互会话）：不阻塞主链路，按默认策略继续
		log.Printf("[弹窗] 确认框不可用(%v)，按默认策略继续执行", callErr)
		return defaultYes
	}
	if ret == idTimeout {
		log.Printf("[弹窗] %d 秒内无响应，按默认策略继续执行", timeoutSec)
		return defaultYes
	}
	return ret == idYes
}
