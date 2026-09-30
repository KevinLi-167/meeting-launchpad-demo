package model

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestDefaultClientProcessNames 已核实过的会议类型都必须有默认进程名
// 漏掉一个会让「运行中会议检测」对这类客户端完全失效（检测不到 → 照常执行）
func TestDefaultClientProcessNames(t *testing.T) {
	names := DefaultClientProcessNames()
	for _, k := range []MeetingType{MeetingTypeQuanshi, MeetingTypeTencent, MeetingTypeZoom, MeetingTypeXylink} {
		if len(names[string(k)]) == 0 {
			t.Fatalf("已核实的类型 %q 缺少默认进程名", k)
		}
	}
	// 小鱼易连的真实进程名已在安装目录核实为 XYWinQT.exe
	if got := names[string(MeetingTypeXylink)]; len(got) != 1 || got[0] != "XYWinQT.exe" {
		t.Fatalf("小鱼易连默认进程名应为 XYWinQT.exe，实际 %v", got)
	}
}

// TestConfigFileInfoMatchesEnsure 页面弹窗展示的路径必须与随后真正打开的文件一致
func TestConfigFileInfoMatchesEnsure(t *testing.T) {
	dir := t.TempDir()
	cfg := NewConfigFromBuiltin(1, "8080", false)
	cfg.ConfigPath = filepath.Join(dir, "config.json")

	infoPath, exists := ConfigFileInfo(cfg)
	if exists {
		t.Fatalf("文件尚未创建，exists 应为 false")
	}
	if infoPath != cfg.ConfigPath {
		t.Fatalf("ConfigFileInfo 返回路径不一致: %q", infoPath)
	}

	openPath, created, err := EnsureConfigFile(cfg)
	if err != nil {
		t.Fatalf("EnsureConfigFile 失败: %v", err)
	}
	if !created {
		t.Fatalf("首次调用应标记为新建")
	}
	if infoPath != openPath {
		t.Fatalf("展示路径与实际打开路径不一致: %q vs %q", infoPath, openPath)
	}

	if _, existsAfter := ConfigFileInfo(cfg); !existsAfter {
		t.Fatalf("模板创建后 exists 应为 true")
	}
}

func TestNewConfigFromBuiltin(t *testing.T) {
	cfg := NewConfigFromBuiltin(2, "8080", true)
	if cfg.RoomNo != 2 {
		t.Fatalf("RoomNo = %d", cfg.RoomNo)
	}
	if cfg.RoomName != RoomConfigs[2].RoomName {
		t.Fatalf("RoomName 未取到内置值: %q", cfg.RoomName)
	}
	if cfg.DefaultMeetingNo != RoomConfigs[2].DefaultNo {
		t.Fatalf("DefaultMeetingNo 未取到内置值: %q", cfg.DefaultMeetingNo)
	}
	if cfg.ClientRunningAction != ClientRunningWarn {
		t.Fatalf("默认策略应为 warn，实际 %q", cfg.ClientRunningAction)
	}
	if cfg.StartTimeInputMode != TimeInputNative {
		t.Fatalf("默认输入模式应为 native，实际 %q", cfg.StartTimeInputMode)
	}
}

func TestNewConfigFromBuiltinUnknownRoom(t *testing.T) {
	cfg := NewConfigFromBuiltin(9, "8080", false)
	if cfg.RoomName == "" || cfg.RoomNumber == "" {
		t.Fatalf("未知终端编号应给出通用兜底值，实际 %+v", cfg)
	}
	if cfg.DefaultMeetingNo != "" {
		t.Fatalf("未知终端编号不应凭空产生默认会议号，实际 %q", cfg.DefaultMeetingNo)
	}
}

func TestApplyConfigOverrides(t *testing.T) {
	cfg := NewConfigFromBuiltin(1, "8080", false)

	roomNo := 3
	roomName := "演示会议室"
	port := "9090"
	silent := true
	action := ClientRunningSkip
	mode := TimeInputText
	presets := []PresetButton{{Label: "全时云 …9802", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "89339802"}}

	f := fileConfig{
		RoomNo:              &roomNo,
		RoomName:            &roomName,
		Port:                &port,
		Silent:              &silent,
		Presets:             &presets,
		ClientRunningAction: &action,
		StartTimeInputMode:  &mode,
	}
	applyConfigOverrides(&cfg, f)

	if cfg.RoomNo != 3 || cfg.RoomName != "演示会议室" || cfg.Port != "9090" || !cfg.Silent {
		t.Fatalf("标量字段覆盖失败: %+v", cfg)
	}
	if cfg.ClientRunningAction != ClientRunningSkip || cfg.StartTimeInputMode != TimeInputText {
		t.Fatalf("策略字段覆盖失败: action=%q mode=%q", cfg.ClientRunningAction, cfg.StartTimeInputMode)
	}
	if len(cfg.Presets) != 1 {
		t.Fatalf("预设未被覆盖: %+v", cfg.Presets)
	}
	// 未在文件中出现的字段必须保留内置值
	if cfg.RoomNumber != RoomConfigs[1].RoomNumber {
		t.Fatalf("未覆盖字段不应被清空: %q", cfg.RoomNumber)
	}
	if cfg.DefaultMeetingNo != RoomConfigs[1].DefaultNo {
		t.Fatalf("未覆盖字段不应被清空: %q", cfg.DefaultMeetingNo)
	}
}

func TestNormalizeConfigFallbacks(t *testing.T) {
	cfg := Config{
		RoomNo:              0,
		RoomName:            "",
		RoomNumber:          "",
		Port:                "",
		DefaultMeetingType:  "skype",
		ClientRunningAction: "hack",
		StartTimeInputMode:  "wheel",
		ConfirmTimeoutSec:   0,
		Presets: []PresetButton{
			{Label: "", MeetingType: "skype"},
			{Label: "合法", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "89339802"},
			{Label: "非法号", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "abc"},
			{Label: "第四个", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "1234567"},
		},
	}
	normalizeConfig(&cfg)

	if cfg.RoomNo != 1 || cfg.RoomName == "" || cfg.RoomNumber == "" || cfg.Port != "8080" {
		t.Fatalf("空值兜底失败: %+v", cfg)
	}
	if cfg.DefaultMeetingType != string(MeetingTypeQuanshi) {
		t.Fatalf("非法会议类型未回退: %q", cfg.DefaultMeetingType)
	}
	if cfg.ClientRunningAction != ClientRunningWarn {
		t.Fatalf("非法策略未回退: %q", cfg.ClientRunningAction)
	}
	if cfg.StartTimeInputMode != TimeInputNative {
		t.Fatalf("非法输入模式未回退: %q", cfg.StartTimeInputMode)
	}
	if cfg.ConfirmTimeoutSec != 30 {
		t.Fatalf("超时秒数未回退: %d", cfg.ConfirmTimeoutSec)
	}
	// 4 条里 2 条非法 → 只保留 2 条合法预设，且非法项被剔除
	if len(cfg.Presets) != 2 {
		t.Fatalf("应保留 2 条合法预设，实际 %d: %+v", len(cfg.Presets), cfg.Presets)
	}
	for _, p := range cfg.Presets {
		if p.Label == "" || p.MeetingNo == "abc" {
			t.Fatalf("非法预设未被剔除: %+v", p)
		}
	}
	if cfg.ClientProcessNames == nil || len(cfg.ClientProcessNames) == 0 {
		t.Fatalf("进程名列表不应为空")
	}
}

// TestNormalizeConfigCapsPresets 合法预设超过上限时只保留前 MaxPresets 条
func TestNormalizeConfigCapsPresets(t *testing.T) {
	cfg := NewConfigFromBuiltin(1, "8080", false)
	for i, no := range []string{"1000000001", "1000000002", "1000000003", "1000000004"} {
		cfg.Presets = append(cfg.Presets, PresetButton{
			Label:       "预设",
			MeetingType: string(MeetingTypeQuanshi),
			MeetingNo:   no,
		})
		_ = i
	}
	normalizeConfig(&cfg)

	if len(cfg.Presets) != MaxPresets {
		t.Fatalf("预设应被截断到 %d 条，实际 %d", MaxPresets, len(cfg.Presets))
	}
	if cfg.Presets[0].MeetingNo != "1000000001" || cfg.Presets[MaxPresets-1].MeetingNo != "1000000003" {
		t.Fatalf("应保留前 %d 条，实际 %+v", MaxPresets, cfg.Presets)
	}
}

// TestTemplateJSONParsable 配置文件模板必须是合法 JSON，
// 且其中的 _说明 字段被程序忽略后仍能正常解析为 fileConfig
func TestTemplateJSONParsable(t *testing.T) {
	cfg := NewConfigFromBuiltin(1, "8080", true)
	data, err := TemplateJSON(cfg)
	if err != nil {
		t.Fatalf("生成模板失败: %v", err)
	}

	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("模板不是合法 JSON: %v", err)
	}
	if _, ok := probe["_说明"]; !ok {
		t.Fatalf("模板应包含 _说明 字段用于自我说明")
	}

	var f fileConfig
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("模板无法解析为 fileConfig: %v", err)
	}
	if f.RoomNo == nil || *f.RoomNo != 1 {
		t.Fatalf("模板中的 room_no 未被解析: %+v", f.RoomNo)
	}
	if f.Silent == nil || !*f.Silent {
		t.Fatalf("模板中的 silent 未被解析: %+v", f.Silent)
	}
}
