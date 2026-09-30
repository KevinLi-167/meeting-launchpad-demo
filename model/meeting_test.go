package model

import (
	"strings"
	"testing"
)

func TestValidateMeetingNo(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"正常会议号", "89339802", false},
		{"最短4位", "1234", false},
		{"32位上限", strings.Repeat("9", 32), false},
		{"空值", "", true},
		{"过短", "123", true},
		{"超长", strings.Repeat("9", 33), true},
		{"含字母", "abc12345", true},
		{"含空格", "1234 5678", true},
		{"含斜杠", "1234/5678", true},
		{"含点号", "1234..5678", true},
		{"含与号", "1234&whoami", true},
		{"含换行", "1234\n5678", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateMeetingNo(c.input)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateMeetingNo(%q) err=%v, wantErr=%v", c.input, err, c.wantErr)
			}
		})
	}
}

func TestValidateCustomURL(t *testing.T) {
	// 长难链接：字段多、含 % 编码与嵌套参数，不做解析，只校验协议与长度
	longTeamsLink := "https://teams.microsoft.com/l/meetup-join/19%3ameeting_NDA3YTkx%40thread.v2/0" +
		"?context=%7b%22Tid%22%3a%22c2b1c0f4-1111-2222-3333-444455556666%22%2c%22Oid%22%3a%22aabbccdd%22%7d"
	depLogin := "https://web.shanbay.com/bayaccount/login?next=https%3A%2F%2Fweb.shanbay.com%2Freading%2Fweb-news%2Farticles"

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"普通https", "https://example.com/meet/123", false},
		{"普通http", "http://192.168.1.50:8080/x", false},
		{"Teams长难链接", longTeamsLink, false},
		{"带嵌套回调的登录链接", depLogin, false},
		{"空值", "", true},
		{"非http协议", "ftp://example.com/a", true},
		{"自定义协议", "tang:///app:6/wp:123", true},
		{"缺少主机", "https:///path", true},
		{"含换行", "https://example.com/\nX", true},
		{"超长", "https://example.com/" + strings.Repeat("a", MaxURLLen), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateCustomURL(c.input)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateCustomURL err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestValidatePreset(t *testing.T) {
	ok := PresetButton{Label: "全时云 …9802", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "89339802"}
	if err := ValidatePreset(ok); err != nil {
		t.Fatalf("合法预设被拒绝: %v", err)
	}

	bad := []PresetButton{
		{Label: "", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "89339802"},
		{Label: "全时云", MeetingType: "skype", MeetingNo: "1234567"},
		{Label: "全时云", MeetingType: string(MeetingTypeQuanshi), MeetingNo: "abc"},
		{Label: "自定义", MeetingType: string(MeetingTypeCustom), CustomURL: "file:///C:/x.txt"},
	}
	for i, p := range bad {
		if err := ValidatePreset(p); err == nil {
			t.Fatalf("第 %d 个非法预设未被拒绝: %+v", i, p)
		}
	}
}

func TestToURL(t *testing.T) {
	cases := []struct {
		meetingType MeetingType
		no          string
		customURL   string
		want        string
	}{
		{MeetingTypeQuanshi, "89339802", "", "tang:///app:6/skin:1/site:60000/Version:0/action:3/wp:89339802"},
		{MeetingTypeTencent, "123456789", "", "wemeet://page/inmeeting?meeting_code=123456789"},
		{MeetingTypeZoom, "98971300852", "", "https://zoom.us/j/98971300852"},
		{MeetingTypeTeams, "99887766", "", "msteams://teams.live.com/meet/99887766"},
		{MeetingTypeXylink, "9044840941", "", "xylink://joinMeeting?number=9044840941"},
		{MeetingTypeCustom, "", "https://example.com/a?b=1&c=2", "https://example.com/a?b=1&c=2"},
	}
	for _, c := range cases {
		m := &Meeting{MeetingType: c.meetingType, MeetingNo: c.no, CustomURL: c.customURL}
		if got := m.ToURL(); got != c.want {
			t.Fatalf("ToURL(%s) = %q, want %q", c.meetingType, got, c.want)
		}
	}
}

func TestIsValidMeetingType(t *testing.T) {
	// 每新增一种会议类型，这里必须同步加一行 —— 否则它会静默退化成"非法类型"
	valid := []MeetingType{
		MeetingTypeQuanshi,
		MeetingTypeTencent,
		MeetingTypeZoom,
		MeetingTypeTeams,
		MeetingTypeXylink,
		MeetingTypeCustom,
	}
	for _, mt := range valid {
		if !IsValidMeetingType(mt) {
			t.Fatalf("IsValidMeetingType(%q) = false, 期望 true", mt)
		}
	}
	invalid := []MeetingType{"", "skype", "Quanshi", "zoom ", "zoommtg"}
	for _, mt := range invalid {
		if IsValidMeetingType(mt) {
			t.Fatalf("IsValidMeetingType(%q) = true, 期望 false", mt)
		}
	}
}

func TestShortMeetingNo(t *testing.T) {
	if got := ShortMeetingNo("89339802"); got != "…9802" {
		t.Fatalf("ShortMeetingNo = %q", got)
	}
	if got := ShortMeetingNo("1234"); got != "1234" {
		t.Fatalf("ShortMeetingNo 短号不应截断，得到 %q", got)
	}
}
