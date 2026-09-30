//go:build !windows

package model

import (
	"fmt"
	"os/exec"
	"runtime"
)

// 非 Windows 平台的空实现，仅用于保证代码可跨平台编译与静态检查通过。
// 本项目实际部署目标为 Windows 会议室终端。

// IsAnyProcessRunning 非 Windows 平台不做进程检测
func IsAnyProcessRunning(names []string) (bool, string) { return false, "" }

// LaunchURL 非 Windows 平台使用系统默认打开方式
func LaunchURL(target string) error {
	if target == "" {
		return fmt.Errorf("启动目标为空")
	}
	return exec.Command("xdg-open", target).Start()
}

// OpenConfigFile 非 Windows 平台使用系统默认打开方式
func OpenConfigFile(path string) error {
	if path == "" {
		return fmt.Errorf("配置文件路径为空")
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", path).Start()
	}
	return exec.Command("xdg-open", path).Start()
}

// ConfirmDialog 非 Windows 平台无弹窗能力，直接按默认策略处理
func ConfirmDialog(title, text string, timeoutSec int, defaultYes bool) bool {
	return defaultYes
}
