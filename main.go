package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"meeting-launchpad/model"
	"meeting-launchpad/router"
)

// 编译时可注入的变量，通过 -ldflags "-X main.xxx=value" 设置
// 若编译时未注入，则保持空字符串，走命令行参数逻辑
var (
	BuildRoomNo string // 编译时固定的终端编号（如 "1"）
	BuildPort   string // 编译时固定的端口（如 "8080"）
	BuildSilent string // 编译时固定的静默模式（如 "true"）
	BuildDemo   string // 编译时标记为对外演示/脱敏版本（房间信息与默认会议号使用示例值）
)

func main() {
	// 命令行参数定义
	// 优先级：命令行参数 > config.json > 编译期注入值(-ldflags) > 代码内置值
	// 这里把 flag 的默认值设为"未指定"的零值，再通过 flag.Visit 判断用户是否显式传入
	roomFlag := flag.Int("room", 0, "终端编号（不传则用编译期注入值或内置值）")
	portFlag := flag.String("port", "", "HTTP服务端口（不传则用编译期注入值或配置值）")
	silentFlag := flag.Bool("silent", false, "静默运行模式（日志输出到文件）")
	flag.Parse()

	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	// ① 先取编译期注入值（② 若命令行显式传入则优先）
	roomNo := 1
	if BuildRoomNo != "" {
		if v, err := strconv.Atoi(BuildRoomNo); err == nil {
			roomNo = v
		}
	}
	if explicit["room"] {
		roomNo = *roomFlag
	}

	port := "8080"
	if BuildPort != "" {
		port = BuildPort
	}
	if explicit["port"] {
		port = *portFlag
	}

	silent := false
	if BuildSilent != "" {
		if v, err := strconv.ParseBool(BuildSilent); err == nil {
			silent = v
		}
	}
	if explicit["silent"] {
		silent = *silentFlag
	}

	if roomNo < 1 || roomNo > 99 {
		log.Fatal("终端编号必须在 1-99 之间")
	}

	// ③ 以内置值打底，再用同级的 config.json 覆盖
	baseCfg := model.NewConfigFromBuiltin(roomNo, port, silent)
	if isDemoBuild() {
		// 对外演示 / 脱敏版本：房间信息与默认会议号使用示例值
		baseCfg = model.NewConfigFromDemo(roomNo, port, silent)
	}
	cfg := model.LoadConfig(baseCfg)

	// ④ 命令行显式传入的值优先级最高，最后再覆盖一次
	if explicit["room"] {
		cfg.RoomNo = roomNo
	}
	if explicit["port"] {
		cfg.Port = port
	}
	if explicit["silent"] {
		cfg.Silent = silent
	}
	model.SetCurrent(cfg)

	// 初始化日志记录（静默模式下日志写文件）
	logName := ""
	if cfg.Silent {
		logName = time.Now().Format("20060102_150405") + ".log"
		initFileLog(logName)
	}

	ip := getLocalIP()
	addr := fmt.Sprintf(":%s", cfg.Port)

	modeStr := "console"
	if cfg.Silent {
		modeStr = "silent"
	}

	// 记录启动日志
	log.Printf("[启动] %s %s - 终端#%d [%s], 端口:%s, 模式:%s, 配置来源:%s",
		model.AppNameEN, model.AppVersion, cfg.RoomNo, cfg.RoomName, cfg.Port, modeStr, configSource(cfg))

	// 打印启动信息（非静默模式）
	if !cfg.Silent {
		fmt.Println("========================================")
		fmt.Printf("%s %s\n", model.AppName, model.AppVersion)
		fmt.Printf("%s\n", model.AppNameEN)
		fmt.Println("========================================")
		fmt.Printf("Terminal No : %d\n", cfg.RoomNo)
		fmt.Printf("Room Name   : %s\n", cfg.RoomName)
		fmt.Printf("Room Number : %s\n", cfg.RoomNumber)
		fmt.Printf("Default No  : %s\n", cfg.DefaultMeetingNo)
		fmt.Printf("Presets     : %d\n", len(cfg.Presets))
		fmt.Printf("Local IP    : %s\n", ip)
		fmt.Printf("Port        : %s\n", cfg.Port)
		fmt.Printf("Access URL  : http://%s:%s\n", ip, cfg.Port)
		fmt.Printf("Mode        : %s\n", modeStr)
		fmt.Printf("Config      : %s\n", configSource(cfg))
		fmt.Printf("Config Path : %s\n", cfg.ConfigPath)
		fmt.Printf("Data Dir    : %s\n", model.DataDir())
		if cfg.Silent {
			fmt.Printf("Log File    : %s\n", logName)
		}
		if cfg.LoadError != "" {
			fmt.Printf("Config Warn : %s\n", cfg.LoadError)
		}
		fmt.Println("========================================")
	}

	// 启动HTTP服务器
	router.Start(cfg, addr)
}

// configSource 用一句话说明当前配置来自哪里
func configSource(cfg model.Config) string {
	if cfg.Loaded {
		return "外部配置 config.json"
	}
	if cfg.LoadError != "" {
		return "内置默认值（外部配置异常，见上方告警）"
	}
	return "内置默认值（未发现 config.json）"
}

// isDemoBuild 判断当前可执行文件是否为对外演示 / 脱敏版本
func isDemoBuild() bool {
	if BuildDemo == "" {
		return false
	}
	v, err := strconv.ParseBool(BuildDemo)
	return err == nil && v
}

// initFileLog 将日志输出重定向到文件
// fileName: 日志文件名（含扩展名）
func initFileLog(fileName string) {
	logFile, err := os.Create(fileName)
	if err != nil {
		// 如果创建失败，继续使用控制台输出
		fmt.Printf("无法创建日志文件: %v\n", err)
		return
	}

	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}

// getLocalIP 获取本机IPv4地址
// 优先选择有默认网关的物理网络接口
// 跳过虚拟适配器（vmware、virtual、vgate、docker等）
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	for _, iface := range interfaces {
		// 跳过虚拟适配器
		if containsVirtualKeyword(iface.Name) {
			continue
		}
		// 只选择活跃的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// containsVirtualKeyword 检查网络接口名称是否包含虚拟化关键词
// 用于过滤掉vmware、virtual、vgate、docker、wsl等虚拟网络适配器
func containsVirtualKeyword(name string) bool {
	keywords := []string{"vmware", "virtual", "vgate", "vmnet", "veth", "docker", "wsl", "hyper-v", "bluetooth", "蓝牙"}
	for _, kw := range keywords {
		if strings.Contains(strings.ToLower(name), kw) {
			return true
		}
	}
	return false
}
