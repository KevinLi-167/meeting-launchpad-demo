package model

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// AppName / AppVersion 用于启动信息、页面页脚与日志标识
const (
	AppName    = "会议终端一键入会系统"
	AppNameEN  = "Meeting Launchpad"
	AppVersion = "v2.1"
)

// 运行策略取值
const (
	// ClientRunningWarn 检测到客户端已在运行时照常执行，仅记录（默认，保持既有行为不变）
	ClientRunningWarn = "warn"
	// ClientRunningSkip 检测到客户端已在运行时跳过，不抢占
	ClientRunningSkip = "skip"
	// ClientRunningConfirm 检测到客户端已在运行时弹窗询问，超时按默认执行
	ClientRunningConfirm = "confirm"
	// ClientRunningOff 不做运行中检测
	ClientRunningOff = "off"

	// TimeInputNative 使用浏览器原生日期时间控件（手机端体验最佳）
	TimeInputNative = "native"
	// TimeInputText 使用可键盘输入的文本框（桌面端便于直接输入）
	TimeInputText = "text"
)

// configFileName 同级配置文件名
const configFileName = "config.json"

// PresetButton 预设的一键快捷启动按钮
// 显示名称为「会议类型 + 短会议号」，点击后自动填充表单并二次确认
type PresetButton struct {
	Label       string `json:"label"`                  // 按钮显示名
	MeetingType string `json:"meeting_type"`           // quanshi / tencent / zoom / teams / xylink / custom
	MeetingNo   string `json:"meeting_no,omitempty"`   // 会议号（custom 类型可留空）
	CustomURL   string `json:"custom_url,omitempty"`   // 自定义链接（仅 custom 类型）
	MeetingName string `json:"meeting_name,omitempty"` // 会议名称（可选）
}

// Config 终端运行配置
// 优先级：命令行参数 > config.json > 编译期注入值 > 代码内置值
// 设计前提：配置文件永远可选 —— 没有它，程序以内置值正常运行
type Config struct {
	RoomNo             int    `json:"room_no"`
	RoomNumber         string `json:"room_number"`
	RoomName           string `json:"room_name"`
	DefaultMeetingNo   string `json:"default_meeting_no"`
	DefaultMeetingType string `json:"default_meeting_type"`
	Port               string `json:"port"`
	Silent             bool   `json:"silent"`

	Presets []PresetButton `json:"presets"`

	// ClientProcessNames 分会议类型的客户端进程名，用于「运行中会议检测」
	ClientProcessNames map[string][]string `json:"client_process_names"`

	// ClientRunningAction 运行中检测策略：warn（默认）/ skip / confirm / off
	ClientRunningAction string `json:"client_running_action"`
	// ConfirmTimeoutSec 确认弹窗等待秒数，超时按「默认执行」处理
	ConfirmTimeoutSec int `json:"confirm_timeout_sec"`

	// StartTimeInputMode 开始时间输入方式：native（原生控件，默认）/ text（可键盘输入）
	StartTimeInputMode string `json:"start_time_input_mode"`

	// HideConfigEntry 为 true 时，页面不渲染「修改默认配置」入口
	// 该入口会在会议室终端本机弹出窗口，是否保留取决于部署方的管理边界（开源场景各不相同）
	// 无论是否隐藏，config.json 本身始终有效：直接编辑它并重启服务即可生效
	HideConfigEntry bool `json:"hide_config_entry"`

	// ---- 以下为运行期字段，不写入配置文件 ----
	ConfigPath string `json:"-"` // 实际使用的配置文件路径
	Loaded     bool   `json:"-"` // 是否成功读取到外部配置
	LoadError  string `json:"-"` // 读取/解析失败原因（非致命，用于页面提示）
}

// fileConfig 用于解析 config.json：全部使用指针，以区分"字段未出现"与"字段值为零值"
type fileConfig struct {
	Note                []string             `json:"_说明"`
	RoomNo              *int                 `json:"room_no"`
	RoomNumber          *string              `json:"room_number"`
	RoomName            *string              `json:"room_name"`
	DefaultMeetingNo    *string              `json:"default_meeting_no"`
	DefaultMeetingType  *string              `json:"default_meeting_type"`
	Port                *string              `json:"port"`
	Silent              *bool                `json:"silent"`
	Presets             *[]PresetButton      `json:"presets"`
	ClientProcessNames  *map[string][]string `json:"client_process_names"`
	ClientRunningAction *string              `json:"client_running_action"`
	ConfirmTimeoutSec   *int                 `json:"confirm_timeout_sec"`
	StartTimeInputMode  *string              `json:"start_time_input_mode"`
	HideConfigEntry     *bool                `json:"hide_config_entry"`
}

// current 保存当前终端生效的配置（启动时设置一次，运行期只读）
var current Config

// Current 返回当前生效配置
func Current() Config { return current }

// SetCurrent 设置当前生效配置
func SetCurrent(c Config) { current = c }

// DefaultClientProcessNames 内置的客户端进程名（可用配置覆盖）
//
// 只收录**已确认**的进程名：这里写错会导致"运行中检测"误判（把没开的会认成已开
// 而跳过执行），比不写更危险 —— 未收录的类型一律视为"未运行"，即照常执行。
//
// 只写进程名，不写安装路径：不同机器的安装位置并不统一（系统盘/数据盘、用户目录
// 均可），路径对检测毫无用处，反而会误导使用者以为要照着改。
//
// 五种类型的进程名均已核实：
//
//	quanshi  TangMeeting.exe
//	tencent  WeMeetApp.exe
//	zoom     Zoom.exe
//	teams    ms-teams.exe     （新版 Teams；旧版为 Teams.exe，一并保留以兼容）
//	xylink   XYWinQT.exe      （小鱼易连）
//
// 匹配不区分大小写，因此写 wemeetapp.exe 也能命中 WeMeetApp.exe。
// 需要调整时请在 config.json 的 client_process_names 里覆盖对应键，例如：
//
//	"client_process_names": { "teams": ["实际进程名.exe"] }
func DefaultClientProcessNames() map[string][]string {
	return map[string][]string{
		string(MeetingTypeQuanshi): {"TangMeeting.exe"},
		string(MeetingTypeTencent): {"wemeetapp.exe", "WeMeet.exe"}, // 实际为 WeMeetApp.exe
		string(MeetingTypeZoom):    {"Zoom.exe"},
		string(MeetingTypeTeams):   {"ms-teams.exe", "Teams.exe"}, // 新/旧两版 Teams
		string(MeetingTypeXylink):  {"XYWinQT.exe"},
	}
}

// genericRoomDefaults 未预置的房间号的通用兜底
func genericRoomDefaults(roomNo int) RoomConfig {
	return RoomConfig{
		RoomNumber: fmt.Sprintf("R%03d", roomNo),
		RoomName:   fmt.Sprintf("会议室 %d", roomNo),
		DefaultNo:  "",
	}
}

// demoRoomDefaults 对外演示 / 脱敏发布使用的示例值
//
// 本仓库的 RoomConfigs 本身即示例值，这里直接复用，保证
// 「编译期注入的 demo 形态」与「不带任何开关的普通构建」结果完全一致 ——
// 两条路径都不会带出真实会议室信息。
func demoRoomDefaults(roomNo int) RoomConfig {
	if room, ok := RoomConfigs[roomNo]; ok {
		return room
	}
	return genericRoomDefaults(roomNo)
}

// NewConfigFromDemo 以示例值构造配置，用于对外发布 / 公开演示
// 与内置值的唯一区别是房间信息与默认会议号全部替换为示例值
func NewConfigFromDemo(roomNo int, port string, silent bool) Config {
	cfg := NewConfigFromBuiltin(roomNo, port, silent)
	room := demoRoomDefaults(roomNo)
	cfg.RoomNumber = room.RoomNumber
	cfg.RoomName = room.RoomName
	cfg.DefaultMeetingNo = room.DefaultNo
	return cfg
}

// NewConfigFromBuiltin 以代码内置值构造一份配置
// roomNo / port / silent 通常来自编译期注入（-ldflags -X）或命令行参数
func NewConfigFromBuiltin(roomNo int, port string, silent bool) Config {
	room, ok := RoomConfigs[roomNo]
	if !ok {
		room = genericRoomDefaults(roomNo)
	}
	return Config{
		RoomNo:              roomNo,
		RoomNumber:          room.RoomNumber,
		RoomName:            room.RoomName,
		DefaultMeetingNo:    room.DefaultNo,
		DefaultMeetingType:  string(MeetingTypeQuanshi),
		Port:                port,
		Silent:              silent,
		Presets:             []PresetButton{},
		ClientProcessNames:  DefaultClientProcessNames(),
		ClientRunningAction: ClientRunningWarn,
		ConfirmTimeoutSec:   30,
		StartTimeInputMode:  TimeInputNative,
		// 默认保留「修改默认配置」入口；开源场景下由部署方按自己的管理边界决定是否关闭
		HideConfigEntry: false,
	}
}

// ExecutableDir 返回可执行文件所在目录（配置与数据文件以它为基准，不依赖当前工作目录）
func ExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			return wd
		}
		return "."
	}
	return filepath.Dir(exe)
}

// ConfigFilePath 返回配置文件路径（可执行文件同级目录）
func ConfigFilePath() string {
	return filepath.Join(ExecutableDir(), configFileName)
}

// LoadConfig 加载配置：内置值打底，再用同级的 config.json 逐字段覆盖
// 任何失败都不会导致启动失败，只记录原因并回退内置值
func LoadConfig(base Config) Config {
	cfg := base
	cfg.ConfigPath = ConfigFilePath()

	raw, err := os.ReadFile(cfg.ConfigPath)
	switch {
	case err != nil && os.IsNotExist(err):
		// 配置文件不存在是正常情况：直接用内置默认值
	case err != nil:
		cfg.LoadError = fmt.Sprintf("读取配置文件失败：%v（已使用内置默认值）", err)
	default:
		if len(strings.TrimSpace(string(raw))) == 0 {
			cfg.LoadError = fmt.Sprintf("配置文件为空（已使用内置默认值，请检查 %s）", cfg.ConfigPath)
			break
		}
		var f fileConfig
		if uErr := json.Unmarshal(raw, &f); uErr != nil {
			cfg.LoadError = fmt.Sprintf("配置文件格式有误：%v（已使用内置默认值，请检查 %s）", uErr, cfg.ConfigPath)
			break
		}
		applyConfigOverrides(&cfg, f)
		cfg.Loaded = true
	}

	normalizeConfig(&cfg)
	SetCurrent(cfg)
	if cfg.LoadError != "" {
		log.Printf("[配置] %s", cfg.LoadError)
	} else if cfg.Loaded {
		log.Printf("[配置] 已加载外部配置: %s", cfg.ConfigPath)
	} else {
		log.Printf("[配置] 未发现外部配置，使用内置默认值: %s", cfg.ConfigPath)
	}
	return cfg
}

// applyConfigOverrides 用配置文件中"出现过的字段"覆盖内置值
// 逐字段判断而不是整体替换，保证只写关心的字段也能生效
func applyConfigOverrides(cfg *Config, f fileConfig) {
	if f.RoomNo != nil && *f.RoomNo > 0 {
		cfg.RoomNo = *f.RoomNo
	}
	if s := trimPtr(f.RoomNumber); s != "" {
		cfg.RoomNumber = s
	}
	if s := trimPtr(f.RoomName); s != "" {
		cfg.RoomName = s
	}
	if f.DefaultMeetingNo != nil {
		cfg.DefaultMeetingNo = strings.TrimSpace(*f.DefaultMeetingNo)
	}
	if s := trimPtr(f.DefaultMeetingType); s != "" {
		cfg.DefaultMeetingType = s
	}
	if s := trimPtr(f.Port); s != "" {
		cfg.Port = s
	}
	if f.Silent != nil {
		cfg.Silent = *f.Silent
	}
	if f.Presets != nil {
		cfg.Presets = *f.Presets
	}
	if f.ClientProcessNames != nil && len(*f.ClientProcessNames) > 0 {
		merged := DefaultClientProcessNames()
		for k, v := range *f.ClientProcessNames {
			merged[k] = v
		}
		cfg.ClientProcessNames = merged
	}
	if s := trimPtr(f.ClientRunningAction); s != "" {
		cfg.ClientRunningAction = s
	}
	if f.ConfirmTimeoutSec != nil && *f.ConfirmTimeoutSec > 0 {
		cfg.ConfirmTimeoutSec = *f.ConfirmTimeoutSec
	}
	if s := trimPtr(f.StartTimeInputMode); s != "" {
		cfg.StartTimeInputMode = s
	}
	if f.HideConfigEntry != nil {
		cfg.HideConfigEntry = *f.HideConfigEntry
	}
}

func trimPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// normalizeConfig 收敛配置取值，剔除非法项，保证运行期取值可控
func normalizeConfig(cfg *Config) {
	if cfg.RoomNo <= 0 {
		cfg.RoomNo = 1
	}
	if cfg.RoomNumber == "" {
		cfg.RoomNumber = genericRoomDefaults(cfg.RoomNo).RoomNumber
	}
	if cfg.RoomName == "" {
		cfg.RoomName = genericRoomDefaults(cfg.RoomNo).RoomName
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if !IsValidMeetingType(MeetingType(cfg.DefaultMeetingType)) {
		cfg.DefaultMeetingType = string(MeetingTypeQuanshi)
	}
	switch cfg.ClientRunningAction {
	case ClientRunningWarn, ClientRunningSkip, ClientRunningConfirm, ClientRunningOff:
	default:
		log.Printf("[配置] client_running_action 取值非法(%q)，已回退 %s", cfg.ClientRunningAction, ClientRunningWarn)
		cfg.ClientRunningAction = ClientRunningWarn
	}
	if cfg.ConfirmTimeoutSec <= 0 {
		cfg.ConfirmTimeoutSec = 30
	}
	if cfg.ConfirmTimeoutSec > 300 {
		cfg.ConfirmTimeoutSec = 300
	}
	switch cfg.StartTimeInputMode {
	case TimeInputNative, TimeInputText:
	default:
		log.Printf("[配置] start_time_input_mode 取值非法(%q)，已回退 %s", cfg.StartTimeInputMode, TimeInputNative)
		cfg.StartTimeInputMode = TimeInputNative
	}
	if cfg.ClientProcessNames == nil {
		cfg.ClientProcessNames = DefaultClientProcessNames()
	}

	// 预设按钮：最多 MaxPresets 个，非法项直接剔除
	valid := make([]PresetButton, 0, MaxPresets)
	for _, p := range cfg.Presets {
		if len(valid) >= MaxPresets {
			log.Printf("[配置] 预设按钮超过 %d 个，多余项已忽略", MaxPresets)
			break
		}
		if err := ValidatePreset(p); err != nil {
			log.Printf("[配置] 预设按钮 %q 非法，已忽略：%v", p.Label, err)
			continue
		}
		valid = append(valid, p)
	}
	cfg.Presets = valid
}

// TemplateJSON 生成配置文件模板（不存在时写入，供使用者填写）
// 说明文本放在 _说明 字段里：JSON 本身不支持注释，这样既能自解释，又仍是合法 JSON
func TemplateJSON(cfg Config) ([]byte, error) {
	tpl := struct {
		Note                []string            `json:"_说明"`
		RoomNo              int                 `json:"room_no"`
		RoomNumber          string              `json:"room_number"`
		RoomName            string              `json:"room_name"`
		DefaultMeetingNo    string              `json:"default_meeting_no"`
		DefaultMeetingType  string              `json:"default_meeting_type"`
		Port                string              `json:"port"`
		Silent              bool                `json:"silent"`
		Presets             []PresetButton      `json:"presets"`
		ClientProcessNames  map[string][]string `json:"client_process_names"`
		ClientRunningAction string              `json:"client_running_action"`
		ConfirmTimeoutSec   int                 `json:"confirm_timeout_sec"`
		StartTimeInputMode  string              `json:"start_time_input_mode"`
		HideConfigEntry     bool                `json:"hide_config_entry"`
	}{
		Note: []string{
			AppName + "（" + AppNameEN + "）" + AppVersion + " 配置文件",
			"1. 本文件放在程序（可执行文件）同级目录，文件名固定为 config.json。",
			"2. 本文件可选：删除它之后，程序仍会使用内置默认值正常运行。",
			"3. 本文件只在服务启动时读取一次，修改后请重启服务生效。",
			"4. 不需要的字段可以整行删除 —— 未出现的字段一律使用内置默认值。",
			"5. presets 为页面上的「一键快捷启动」按钮，最多 3 个；",
			"   presets 与 default_meeting_type 的取值：",
			"   quanshi(全时云) / tencent(腾讯会议) / zoom / teams / xylink(小鱼易连) / custom(自定义链接)；",
			"   填 custom 时必须同时给 custom_url，且只支持 http / https；",
			"   client_running_action 取 warn / skip / confirm / off，默认 warn；",
			"   start_time_input_mode 取 native / text，默认 native。",
			"6. client_process_names 用于「运行中会议检测」，键是会议类型、值是进程名数组（只写进程名，不含路径）。",
			"   五种类型的内置值均已核实：全时云 / 腾讯会议 / Zoom / Teams / 小鱼易连。",
			"   只填已确认无误的进程名 —— 填错会把没开的会误判为已开，请谨慎。",
			"7. hide_config_entry 为 true 时，页面上不再显示「修改默认配置」入口。",
			"   该入口会在会议室电脑本机弹出窗口，是否需要由部署方按管理边界决定；",
			"   无论是否隐藏，本文件始终有效 —— 直接编辑它并重启服务即可。",
			"8. _说明 字段仅用于自我说明，程序会忽略。",
		},
		RoomNo:              cfg.RoomNo,
		RoomNumber:          cfg.RoomNumber,
		RoomName:            cfg.RoomName,
		DefaultMeetingNo:    cfg.DefaultMeetingNo,
		DefaultMeetingType:  cfg.DefaultMeetingType,
		Port:                cfg.Port,
		Silent:              cfg.Silent,
		Presets:             cfg.Presets,
		ClientProcessNames:  cfg.ClientProcessNames,
		ClientRunningAction: cfg.ClientRunningAction,
		ConfirmTimeoutSec:   cfg.ConfirmTimeoutSec,
		StartTimeInputMode:  cfg.StartTimeInputMode,
		HideConfigEntry:     cfg.HideConfigEntry,
	}
	return json.MarshalIndent(tpl, "", "  ")
}

// resolveConfigPath 取出本次运行实际使用的配置文件路径
// （配置加载时已写入 cfg.ConfigPath，兜底再算一次，保证任何调用点结果一致）
func resolveConfigPath(cfg Config) string {
	if cfg.ConfigPath != "" {
		return cfg.ConfigPath
	}
	return ConfigFilePath()
}

// ConfigFileInfo 返回当前实际生效的配置文件路径，以及该文件此刻是否已存在
// 页面在弹出「修改默认配置」确认框时用它动态取路径 —— 这样弹窗里展示的路径
// 与随后真正被打开的文件永远是同一个，不受页面缓存或旧标签页影响
func ConfigFileInfo(cfg Config) (path string, exists bool) {
	path = resolveConfigPath(cfg)
	if _, err := os.Stat(path); err == nil {
		exists = true
	}
	return path, exists
}

// EnsureConfigFile 保证配置文件存在，返回文件路径与是否为新建
// 供页面「修改默认配置」按钮调用：不存在则先写模板，再交给系统默认程序打开
func EnsureConfigFile(cfg Config) (path string, created bool, err error) {
	path = resolveConfigPath(cfg)
	if _, statErr := os.Stat(path); statErr == nil {
		return path, false, nil
	} else if !os.IsNotExist(statErr) {
		return path, false, fmt.Errorf("检查配置文件失败：%v", statErr)
	}

	data, mErr := TemplateJSON(cfg)
	if mErr != nil {
		return path, false, fmt.Errorf("生成配置模板失败：%v", mErr)
	}
	if wErr := os.WriteFile(path, data, 0644); wErr != nil {
		return path, false, fmt.Errorf("写入配置文件失败（目录可能没有写权限）：%v", wErr)
	}
	log.Printf("[配置] 已创建配置模板: %s", path)
	return path, true, nil
}
