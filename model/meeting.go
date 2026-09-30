package model

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 会议类型枚举
type MeetingType string

const (
	MeetingTypeQuanshi MeetingType = "quanshi" // 全时云会议
	MeetingTypeTencent MeetingType = "tencent" // 腾讯会议
	MeetingTypeZoom    MeetingType = "zoom"    // Zoom 会议
	MeetingTypeTeams   MeetingType = "teams"   // Microsoft Teams 会议
	MeetingTypeXylink  MeetingType = "xylink"  // 小鱼易连会议
	MeetingTypeCustom  MeetingType = "custom"  // 自定义链接（通用兜底通道）
)

// 触发方式
const (
	TriggerScheduled = "scheduled" // 预约执行（到点自动触发）
	TriggerImmediate = "immediate" // 立即执行（一键快捷启动 / 「立刻」按钮）
)

// 任务状态
const (
	StatusPending   = "pending"   // 待执行
	StatusRunning   = "running"   // 正在执行（运行期瞬时状态，重启后视为 pending）
	StatusCompleted = "completed" // 已执行
	StatusMissed    = "missed"    // 已错过（超出补执行时间窗）
)

// 执行结果（写入 tasks_log.jsonl 的 result 字段）
const (
	ResultSuccess         = "success"           // 成功唤起会议客户端
	ResultFailed          = "failed"            // 唤起失败
	ResultMissed          = "missed"            // 超过补执行时间窗，未执行
	ResultSkippedRunning  = "skipped_running"   // 检测到客户端已在运行，按策略跳过
	ResultCancelledByUser = "cancelled_by_user" // 现场人员在确认弹窗中选择"否"
)

// 输入长度与格式上限
const (
	MaxPresets      = 3    // 预设按钮数量上限
	MaxMeetingNoLen = 32   // 会议号长度上限
	MinMeetingNoLen = 4    // 会议号长度下限
	MaxURLLen       = 2048 // 自定义链接长度上限
	MaxTextLen      = 64   // 名称/申请人等文本字段长度上限
	MaxApplicantLen = 32   // 申请人长度上限
)

// 会议号只允许数字：既满足全时云/腾讯/Teams 的实际形态，
// 也从根上消除拼接进启动链接时的注入风险
var meetingNoPattern = regexp.MustCompile(`^[0-9]+$`)

// Meeting 会议结构体，存储会议相关信息
type Meeting struct {
	ID          string      `json:"id"`                     // 唯一标识符，由时间戳生成
	StartTime   time.Time   `json:"start_time"`             // 会议开始时间（计划执行时间）
	EndTime     *time.Time  `json:"end_time,omitempty"`     // 会议结束时间（可选）
	MeetingNo   string      `json:"meeting_no"`             // 会议号
	MeetingType MeetingType `json:"meeting_type"`           // 会议类型：quanshi/tencent/zoom/teams/xylink/custom
	CustomURL   string      `json:"custom_url,omitempty"`   // 自定义URL（仅custom类型有效）
	MeetingName string      `json:"meeting_name,omitempty"` // 会议名称（可选）
	Applicant   string      `json:"applicant,omitempty"`    // 申请人（可选）
	Status      string      `json:"status"`                 // 状态：pending/running/completed/missed

	// ---- 审计字段：只落盘，页面不展示 ----
	ScheduledTime time.Time `json:"scheduled_time"`      // 任务登记时刻
	ClientIP      string    `json:"client_ip,omitempty"` // 发起方 IP
	TriggerType   string    `json:"trigger_type"`        // scheduled / immediate
}

// MeetingRequest 添加会议的请求结构，用于接收JSON数据
type MeetingRequest struct {
	StartTime   string      `json:"start_time"`             // 开始时间（RFC3339格式）
	EndTime     string      `json:"end_time,omitempty"`     // 结束时间（可选，RFC3339格式）
	MeetingNo   string      `json:"meeting_no"`             // 会议号
	MeetingType MeetingType `json:"meeting_type,omitempty"` // 会议类型（可选，默认全时云）
	CustomURL   string      `json:"custom_url,omitempty"`   // 自定义URL（仅custom类型有效）
	MeetingName string      `json:"meeting_name,omitempty"` // 会议名称（可选）
	Applicant   string      `json:"applicant,omitempty"`    // 申请人（可选）
	TriggerType string      `json:"trigger_type,omitempty"` // scheduled（默认）/ immediate
}

// RoomConfig 会议室配置结构
type RoomConfig struct {
	RoomNumber string // 物理房间号
	RoomName   string // 会议室显示名称
	DefaultNo  string // 默认会议号
}

// RoomConfigs 各会议室的代码内置默认值
// 说明：这是"一份源码产出多个终端实例"的兜底来源。
// 实际部署值可以通过同名 config.json 覆盖，无需修改源码、无需重新编译。
// key: 终端编号
//
// 🔴 本仓库是面向外部公开的脱敏示例版：下表的会议室名称、房间号、默认会议号
// **全部是虚构的示例值**，与任何真实单位、真实会议室无关。
// 请用可执行文件同级的 config.json 覆盖成你自己的实际值（见 config.example.json）。
var RoomConfigs = map[int]RoomConfig{
	1: {RoomNumber: "R101", RoomName: "第 1 会议室", DefaultNo: "89339802"},
	2: {RoomNumber: "R102", RoomName: "第 2 会议室", DefaultNo: "88990011"},
	3: {RoomNumber: "R103", RoomName: "第 3 会议室", DefaultNo: "88990022"},
	4: {RoomNumber: "R104", RoomName: "第 4 会议室", DefaultNo: "88990033"},
}

// IsValidMeetingType 校验会议类型取值
func IsValidMeetingType(t MeetingType) bool {
	switch t {
	case MeetingTypeQuanshi, MeetingTypeTencent, MeetingTypeZoom, MeetingTypeTeams, MeetingTypeXylink, MeetingTypeCustom:
		return true
	default:
		return false
	}
}

// ValidateMeetingNo 校验会议号
// 规则：纯数字 + 长度 4–32。长难链接、含字母或带参数的会议标识一律走「自定义」通道原样直传。
func ValidateMeetingNo(no string) error {
	no = strings.TrimSpace(no)
	if no == "" {
		return fmt.Errorf("会议号不能为空")
	}
	if len(no) > MaxMeetingNoLen {
		return fmt.Errorf("会议号长度不能超过 %d 位（当前 %d 位）", MaxMeetingNoLen, len(no))
	}
	if len(no) < MinMeetingNoLen {
		return fmt.Errorf("会议号长度不能少于 %d 位", MinMeetingNoLen)
	}
	if !meetingNoPattern.MatchString(no) {
		return fmt.Errorf("会议号只能是数字；含字母、空格、符号或参数的会议标识请选择「自定义」并粘贴完整链接")
	}
	return nil
}

// ValidateText 校验普通文本字段（会议名称 / 申请人等）
func ValidateText(field, val string, max int) error {
	if len([]rune(val)) > max {
		return fmt.Errorf("%s长度不能超过 %d 个字符", field, max)
	}
	if strings.ContainsAny(val, "\r\n") {
		return fmt.Errorf("%s不能包含换行符", field)
	}
	return nil
}

// ValidateCustomURL 校验自定义链接
// 只做「协议白名单 + 长度上限 + 换行符拦截」，不对链接结构做任何解析 ——
// 这正是"最坏输入也能跑通"的兜底通道设计：字段多、不固定、无文档的长难链接原样直传即可。
func ValidateCustomURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("自定义链接不能为空")
	}
	if len(rawURL) > MaxURLLen {
		return fmt.Errorf("自定义链接长度不能超过 %d 个字符（当前 %d 个）", MaxURLLen, len(rawURL))
	}
	if strings.ContainsAny(rawURL, "\r\n") {
		return fmt.Errorf("自定义链接不能包含换行符")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("链接格式无法解析：%v", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("自定义链接只支持 http / https 开头，当前为 %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("自定义链接缺少有效的主机名")
	}
	return nil
}

// ValidatePreset 校验一个预设按钮的合法性
func ValidatePreset(p PresetButton) error {
	if strings.TrimSpace(p.Label) == "" {
		return fmt.Errorf("按钮显示名不能为空")
	}
	if err := ValidateText("按钮显示名", p.Label, MaxTextLen); err != nil {
		return err
	}
	if !IsValidMeetingType(MeetingType(p.MeetingType)) {
		return fmt.Errorf("会议类型取值非法：%q", p.MeetingType)
	}
	if MeetingType(p.MeetingType) == MeetingTypeCustom {
		if err := ValidateCustomURL(p.CustomURL); err != nil {
			return err
		}
		return nil
	}
	if err := ValidateMeetingNo(p.MeetingNo); err != nil {
		return err
	}
	return ValidateText("会议名称", p.MeetingName, MaxTextLen)
}

// ShortMeetingNo 生成会议号的短形式，用于按钮显示（保留后 4 位便于辨识）
func ShortMeetingNo(no string) string {
	no = strings.TrimSpace(no)
	if len(no) <= 4 {
		return no
	}
	return "…" + no[len(no)-4:]
}

// ToQuanshiURL 生成全时云会议的启动URL
// 格式: tang:///app:6/skin:1/site:60000/...
// 无需后缀: + "/wn:/we:/wuid:/wucid:83924743/wmid:83924743/wfrom:meeting/extName:/extType:/lang:cn/beeState:0/mobile:/callBack:"
func (m *Meeting) ToQuanshiURL() string {
	return "tang:///app:6/skin:1/site:60000/Version:0/action:3/wp:" + m.MeetingNo
}

// ToTencentURL 生成腾讯会议的启动URL
// 格式: wemeet://page/inmeeting?meeting_code=会议号
func (m *Meeting) ToTencentURL() string {
	return "wemeet://page/inmeeting?meeting_code=" + m.MeetingNo
}

// ToTeamsURL 生成Microsoft Teams会议的启动URL
// 格式(只使用会议ID）: msteams://teams.live.com/meet/会议号
// 会议ID+密码：msteams://teams.live.com/meet/会议号?p=会议密码
func (m *Meeting) ToTeamsURL() string {
	return "msteams://teams.live.com/meet/" + m.MeetingNo
}

// ToZoomURL 生成 Zoom 会议的启动URL
// 格式: https://zoom.us/j/会议号
//
// 为什么用 https 而不是 zoommtg:// 自定义协议：
//
//	① 本项目手上已验证可用的两条 Zoom 入会地址都是 https（/j/ 与 /wc/），
//	   而 zoommtg:// 在各终端的注册情况并不一致，采用已验证的形态最稳妥；
//	② 与本项目"零依赖、只依赖各客户端自己注册的协议/文件关联"的既有取舍一致。
//
// /j/ 形态会由 Zoom 的网页做一次"唤起本机客户端"的跳转，
// 与本项目"唤起会议室终端上的会议客户端"的定位一致。
// 若某些终端上需要直接落到 Zoom 网页客户端，把下面这行换成：
//
//	return "https://zoom.us/wc/" + m.MeetingNo + "/join"
func (m *Meeting) ToZoomURL() string {
	return "https://zoom.us/j/" + m.MeetingNo
}

// ToXylinkURL 生成小鱼易连的启动URL
// 格式: xylink://joinMeeting?number=会议号
//
// 完整形态还可带 topic / pwd / scheduleStartTime 等参数（见项目笔记
// 「小鱼易连-唤起客户端相关.md」），但实测**只保留 number 即可唤起**，
// 故与其它会议类型保持同一取舍：只用会议号，不引入额外字段。
//
// 已知限制：需要会议密码（pwd）的房间用不了 —— 密码是独立于会议号的另一段
// 数字，当前数据结构里没有承载它的字段。若后续确有需要，再评估是否新增
// 可选字段（届时 custom 通道也可作为临时手段）。
func (m *Meeting) ToXylinkURL() string {
	return "xylink://joinMeeting?number=" + m.MeetingNo
}

// ToCustomURL 返回自定义URL（已通过ValidateCustomURL校验）
func (m *Meeting) ToCustomURL() string {
	return m.CustomURL
}

// ToURL 根据会议类型返回对应的启动URL
func (m *Meeting) ToURL() string {
	switch m.MeetingType {
	case MeetingTypeTencent:
		return m.ToTencentURL()
	case MeetingTypeZoom:
		return m.ToZoomURL()
	case MeetingTypeTeams:
		return m.ToTeamsURL()
	case MeetingTypeXylink:
		return m.ToXylinkURL()
	case MeetingTypeCustom:
		return m.ToCustomURL()
	default:
		return m.ToQuanshiURL()
	}
}

// Clone 返回会议的副本，避免外部持有内部指针造成并发写入
func (m *Meeting) Clone() *Meeting {
	if m == nil {
		return nil
	}
	c := *m
	if m.EndTime != nil {
		et := *m.EndTime
		c.EndTime = &et
	}
	return &c
}
