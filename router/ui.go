package router

import (
	"encoding/json"
	"html/template"
	"strings"

	"meeting-launchpad/model"
)

// pageData 页面渲染数据
type pageData struct {
	Version     string
	Config      model.Config
	ConfigWarn  string
	ConfigState string
	Presets     []model.PresetButton
	TimeMode    string
	AppJSON     template.JS
	// HideConfigEntry 为真时不在页面上渲染「修改默认配置」入口
	// 开源场景下不同部署方的管理边界不同，由部署者通过配置决定
	HideConfigEntry bool
}

// buildPageData 组装页面数据
// 注意：配置文件路径**不注入页面**，由 /api/config/info 在点击时实时返回 ——
// 避免页面停留过久后弹窗里显示的是旧实例的路径
func buildPageData(cfg model.Config) pageData {
	cfgWarn := cfg.LoadError
	if cfgWarn == "" && !cfg.Loaded {
		cfgWarn = ""
	}

	state := "内置默认值"
	if cfg.Loaded {
		state = "已加载同级 config.json"
	}

	app := map[string]any{
		"timeMode":      cfg.StartTimeInputMode,
		"defaultNo":     cfg.DefaultMeetingNo,
		"defaultType":   cfg.DefaultMeetingType,
		"presets":       cfg.Presets,
		"roomName":      cfg.RoomName,
		"appName":       model.AppName,
		"version":       model.AppVersion,
		"presetTimeout": 0,
	}
	raw, err := json.Marshal(app)
	if err != nil {
		raw = []byte("{}")
	}

	return pageData{
		Version:         model.AppVersion,
		Config:          cfg,
		ConfigWarn:      cfgWarn,
		ConfigState:     state,
		Presets:         cfg.Presets,
		TimeMode:        cfg.StartTimeInputMode,
		AppJSON:         template.JS(raw), // 由本程序自产的配置数据，非用户输入
		HideConfigEntry: cfg.HideConfigEntry,
	}
}

// renderPage 渲染页面 HTML
func renderPage(cfg model.Config) string {
	var sb strings.Builder
	if err := pageTemplate.Execute(&sb, buildPageData(cfg)); err != nil {
		return "<!DOCTYPE html><html><body><h1>页面渲染失败</h1><p>" + template.HTMLEscapeString(err.Error()) + "</p></body></html>"
	}
	return sb.String()
}

// pageTemplate 内联页面模板
// 页面刻意不做成"配置界面"：需要改默认配置时，由「修改默认配置」按钮在会议室终端本机打开 config.json
var pageTemplate = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Config.RoomName}} · 会议终端一键入会系统</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Microsoft YaHei", "PingFang SC", Arial, sans-serif; background: #f5f5f5; padding: 16px; color: #333; }
        .container { max-width: 900px; margin: 0 auto; }
        .card { background: #fff; border-radius: 8px; padding: 16px; margin-bottom: 14px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); }
        h1 { font-size: 20px; margin-bottom: 8px; }
        h2 { font-size: 15px; margin-bottom: 12px; color: #333; }
        .room-line { font-size: 13px; color: #666; line-height: 1.7; }
        .muted { color: #94a3b8; }
        .banner { border-radius: 6px; padding: 10px 12px; font-size: 13px; margin-bottom: 14px; line-height: 1.6; }
        .banner.warn { background: #fff7ed; border-left: 4px solid #f59e0b; color: #92400e; }
        .banner.ok { background: #ecfdf5; border-left: 4px solid #10b981; color: #065f46; }
        .banner.err { background: #fef2f2; border-left: 4px solid #ef4444; color: #991b1b; }
        .banner.info { background: #eff6ff; border-left: 4px solid #3b82f6; color: #1e40af; }
        .preset-row { display: flex; flex-wrap: wrap; gap: 10px; }
        .preset { background: #0ea5e9; color: #fff; border: none; border-radius: 6px; padding: 12px 16px; font-size: 14px; font-weight: 600; cursor: pointer; flex: 1 1 200px; text-align: center; }
        .preset:hover { background: #0284c7; }
        .preset:disabled { background: #cbd5e1; cursor: not-allowed; }
        .preset-empty { font-size: 13px; color: #94a3b8; }
        .confirm-bar { margin-top: 12px; background: #fffbeb; border: 1px solid #fcd34d; border-radius: 6px; padding: 12px; font-size: 14px; line-height: 1.7; }
        .confirm-bar b { color: #b45309; }
        .confirm-actions { margin-top: 10px; display: flex; gap: 10px; flex-wrap: wrap; }
        .btn { border: none; border-radius: 4px; padding: 10px 18px; font-size: 14px; cursor: pointer; }
        .btn-primary { background: #007bff; color: #fff; }
        .btn-primary:hover { background: #0056b3; }
        .btn-primary:disabled { background: #93c5fd; cursor: not-allowed; }
        .btn-go { background: #16a34a; color: #fff; }
        .btn-go:hover { background: #15803d; }
        .btn-ghost { background: #f1f5f9; color: #334155; }
        .btn-ghost:hover { background: #e2e8f0; }
        .btn-danger { background: #dc3545; color: #fff; }
        .btn-danger:hover { background: #c82333; }
        .btn-info { background: #0284c7; color: #fff; }
        .btn-info:hover { background: #0369a1; }
        .btn-info:disabled { background: #7dd3fc; cursor: not-allowed; }
        .btn-sm { padding: 6px 12px; font-size: 13px; }
        /* 顶部卡片：左侧标题与房间信息，右侧操作按钮 */
        .header-card { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; flex-wrap: wrap; }
        .header-main { flex: 1 1 320px; min-width: 0; }
        .header-actions { display: flex; gap: 8px; flex-shrink: 0; flex-wrap: wrap; }
        /* 二次确认弹窗：本系统唯一的「需要人点一下」的破坏性前置操作 */
        .modal-mask { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(15,23,42,0.45); display: flex; align-items: center; justify-content: center; padding: 16px; z-index: 100; }
        .modal-mask[hidden] { display: none; }
        .modal-box { background: #fff; border-radius: 10px; padding: 20px; max-width: 440px; width: 100%; box-shadow: 0 10px 30px rgba(0,0,0,0.22); }
        .modal-box h3 { font-size: 16px; margin-bottom: 10px; color: #0f172a; }
        .modal-box p { font-size: 13px; color: #475569; line-height: 1.75; margin-bottom: 8px; }
        .modal-target { font-size: 12px; color: #64748b; background: #f8fafc; border-radius: 4px; padding: 8px 10px; word-break: break-all; margin-top: 4px; }
        .modal-actions { display: flex; gap: 10px; justify-content: flex-end; margin-top: 16px; }
        .form-group { margin-bottom: 14px; }
        label { display: block; margin-bottom: 5px; color: #666; font-size: 13px; }
        input, select { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; background: #fff; }
        input:focus, select:focus { outline: none; border-color: #007bff; }
        .input-row { display: flex; gap: 8px; align-items: stretch; }
        .input-row input { flex: 1; }
        .input-row .btn { white-space: nowrap; }
        .hint { font-size: 12px; color: #94a3b8; margin-top: 4px; line-height: 1.6; }
        .meeting-item { border: 1px solid #e2e8f0; border-radius: 6px; padding: 12px; margin-bottom: 10px; }
        .meeting-item h3 { font-size: 14px; margin-bottom: 6px; color: #1e293b; }
        .meeting-item p { font-size: 13px; color: #64748b; margin-bottom: 3px; line-height: 1.6; }
        .status { display: inline-block; padding: 3px 8px; border-radius: 20px; font-size: 12px; }
        .status.pending { background: #ffc107; color: #333; }
        .status.running { background: #0ea5e9; color: #fff; }
        .status.completed { background: #28a745; color: #fff; }
        .status.missed { background: #94a3b8; color: #fff; }
        .item-actions { margin-top: 10px; }
        /* 「最近一次执行」：位于待执行队列卡片之后的独立摘要卡片 */
        .last-done-body { font-size: 13px; color: #475569; line-height: 1.9; }
        .last-done-body strong { color: #1e293b; }
        .last-done .res { display: inline-block; padding: 2px 8px; border-radius: 20px; font-size: 12px; background: #e2e8f0; color: #475569; }
        .last-done .res.success { background: #dcfce7; color: #166534; }
        .last-done .res.failed { background: #fee2e2; color: #991b1b; }
        .last-done .res.skipped_running { background: #fef3c7; color: #92400e; }
        .footer-note { font-size: 12px; color: #94a3b8; line-height: 1.8; text-align: center; padding-bottom: 16px; }
        @media (max-width: 560px) {
            body { padding: 10px; }
            h1 { font-size: 18px; }
            .preset { flex: 1 1 100%; }
            .input-row { flex-wrap: wrap; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="card header-card">
            <div class="header-main">
                <h1>会议终端一键入会系统</h1>
                <div class="room-line">
                    <div><strong>{{.Config.RoomName}}</strong>（房间号 {{.Config.RoomNumber}}）· 终端 #{{.Config.RoomNo}}</div>
                    <div class="muted">默认会议号：{{if .Config.DefaultMeetingNo}}{{.Config.DefaultMeetingNo}}{{else}}（未设置，请在默认配置中填写）{{end}}</div>
                    <div class="muted">配置来源：{{.ConfigState}}</div>
                </div>
                <!-- 提示：会议时间到后会自动启动对应的会议客户端，请确保已安装相关客户端 -->
                <!-- 提示：添加的任务会持久化保存，即使重启电脑也不会丢失 -->
            </div>
            <div class="header-actions">
                <button type="button" class="btn btn-info btn-sm" onclick="showHelp()">使用说明</button>
                {{if not .HideConfigEntry}}<button type="button" class="btn btn-primary btn-sm" id="cfgBtn" onclick="askOpenConfig()">修改默认配置</button>{{end}}
            </div>
        </div>

        {{if .ConfigWarn}}<div class="banner warn">{{.ConfigWarn}}</div>{{end}}
        <div id="banner" class="banner ok" hidden></div>

        <div class="card">
            <h2>一键快捷启动</h2>
            {{if .Presets}}
            <div class="preset-row" id="presetRow">
                {{range $i, $p := .Presets}}<button type="button" class="preset" data-i="{{$i}}" onclick="runPreset({{$i}})">{{$p.Label}}</button>{{end}}
            </div>
            <!-- 预设按钮：点击后自动填充表单，并弹出二次确认，一次交互同时完成"省填写"与"防误触" -->
            {{else}}
            <div class="preset-empty">尚未配置预设按钮。如需一键启动固定的常用会议，可在「修改默认配置」中的 presets 里添加（最多 3 个）。</div>
            {{end}}
            <div class="confirm-bar" id="presetConfirm" hidden></div>
        </div>

        <div class="card">
            <h2>预约入会</h2>
            <form id="meetingForm">
                <div class="form-group">
                    <label>会议类型:</label>
                    <!-- 全时云(tang://)、腾讯会议(wemeet://)、Zoom(https://zoom.us/j/)、
                         Teams(msteams://)、自定义(http/https)
                         说明：Zoom 采用的是它官方已验证可用的 https 入会地址，
                         由该网页完成一次「唤起本机 Zoom 客户端」的跳转 -->
                    <select id="meetingType" onchange="syncTypeFields()">
                        <option value="quanshi">全时云会议</option>
                        <option value="tencent">腾讯会议</option>
                        <option value="zoom">Zoom会议</option>
                        <option value="teams">Teams会议</option>
                        <option value="xylink">小鱼易连</option>
                        <option value="custom">自定义</option>
                    </select>
                </div>
                <div class="form-group" id="meetingNoGroup">
                    <label>会议号:</label>
                    <input type="text" id="meetingNo" inputmode="numeric" value="{{.Config.DefaultMeetingNo}}" placeholder="请输入会议号（数字）">
                    <!-- 选择「自定义」时本字段整块隐藏（前端隐藏、后端也置空），
                         因为自定义通道用的是完整链接，不使用会议号 -->
                </div>
                <div class="form-group" id="customURLGroup" style="display:none;">
                    <label>自定义链接 (仅支持http/https):</label>
                    <input type="text" id="customURL" placeholder="粘贴要打开的完整链接">
                    <!-- 注意：自定义URL仅支持http/https协议，不支持其他自定义协议 -->
                    <!-- 提示：字段多、格式不固定、带参数的复杂链接（如 Teams 长链接）无需解析，直接粘贴到此处即可 -->
                </div>
                <div class="form-group">
                    <label>开始时间:</label>
                    <!-- 选择会议计划开始的时间，到时间后系统会自动启动会议客户端 -->
                    <div class="input-row">
                        {{if eq .TimeMode "text"}}
                        <input type="text" id="startTime" placeholder="YYYY-MM-DD HH:MM" inputmode="numeric" autocomplete="off">
                        {{else}}
                        <input type="datetime-local" id="startTime" step="60">
                        {{end}}
                        <button type="button" class="btn btn-ghost" id="nowBtn" onclick="setNow()">当前时刻</button>
                    </div>
                    <div class="hint" id="timeHint">{{if eq .TimeMode "text"}}格式：2026-09-28 14:30（也可只填 14:30 表示今天）{{else}}点「当前时刻」可一键填入此刻的时间{{end}}</div>
                </div>
                <div class="form-group">
                    <label>结束时间(可选):</label>
                    <!-- 结束时间仅用于记录，不影响会议的实际结束 -->
                    <!-- ===== 时间控件对照实验（已结束，注释保留以便回溯）=====
                         曾做过：方案A「浏览器原生 datetime-local」与方案B「纯文本手填」分别装在
                                 两个时间框上（开始时间=A、结束时间=B），用真实手感横向对比。
                         结论：**纯文本手填方案不采用**，两个框统一回退为原生控件（方案A）。
                         遗留：桌面端浏览器原生的滚轮灵敏度过高、选定后需点空白才收起，
                               均为浏览器自身行为，非本项目可控；手机端原生控件体验良好，保持不变。
                         保留的余地：start_time_input_mode 配置项仍然有效（text 时两端一起变文本框），
                                   仅作为「若将来确认需要按端差异化」的开关，默认值为 native。
                         详见待办 D16 / D17。 -->
                    <div class="input-row">
                        {{if eq .TimeMode "text"}}
                        <input type="text" id="endTime" placeholder="YYYY-MM-DD HH:MM" inputmode="numeric" autocomplete="off">
                        {{else}}
                        <input type="datetime-local" id="endTime" step="60">
                        {{end}}
                    </div>
                    <div class="hint" id="endTimeHint">{{if eq .TimeMode "text"}}格式：2026-09-28 15:30（也可只填 15:30 表示今天）{{else}}用于记录会议结束时间，不影响会议实际结束{{end}}</div>
                </div>
                <div class="form-group">
                    <label>会议名称(可选):</label>
                    <input type="text" id="meetingName" placeholder="请输入会议名称">
                </div>
                <div class="form-group">
                    <label>申请人(可选):</label>
                    <input type="text" id="applicant" placeholder="请输入申请人姓名">
                </div>
                <!-- 点击提交后任务会添加到队列并持久化到本地文件，关机重启不丢失 -->
                <button type="submit" class="btn btn-primary" id="submitBtn">添加会议</button>
            </form>
        </div>

        <div class="card">
            <h2>待执行会议队列</h2>
            <div id="meetingList">
                <!-- 任务列表每5秒自动刷新；已执行的任务会自动移除并记录到日志 -->
                <p class="muted">加载中…</p>
            </div>
        </div>

        <!-- 最近一次执行：独立卡片，位于待执行队列之后；无执行记录时整块隐藏 -->
        <div class="card last-done" id="lastDoneCard" hidden>
            <h2>最近一次执行</h2>
            <div class="last-done-body" id="lastDoneBody"></div>
            <div class="hint">取自本机执行日志，只表示「客户端已被唤起」，不等同于「已确认入会」。</div>
        </div>

        <!-- 操作按钮已上移至顶部卡片右侧；此处只保留版本说明 -->
        <div class="footer-note">
            {{.Version}} · 部署于会议室终端本机，仅在局域网内可达
        </div>
    </div>

    <!-- 修改默认配置的二次确认弹窗 -->
    <!-- 为什么需要它：该操作会在「会议室电脑本机」弹出记事本窗口，会议进行中会打扰现场，
         因此必须在网页侧先明确告知并二次确认，而不是点一下就直接弹窗。 -->
    {{if not .HideConfigEntry}}
    <div class="modal-mask" id="cfgModal" hidden>
        <div class="modal-box">
            <h3>将在会议室终端本机打开配置文件</h3>
            <p>确认后，会在运行本服务的<b>会议室电脑上</b>用记事本打开 <b>config.json</b>。</p>
            <p>如果该会议室<b>正在开会</b>，这个窗口会打扰现场，请确认当前没有会议正在进行。</p>
            <p>配置文件只在<b>服务启动时读取一次</b>，改完保存后<b>需要手动重启程序才会生效</b>。</p>
            <p class="muted">若部署场景不需要这个入口，可在配置文件中设 <b>"hide_config_entry": true</b> 把它隐藏。</p>
            <div class="modal-target" id="cfgModalPath"></div>
            <div class="modal-actions">
                <button type="button" class="btn btn-ghost btn-sm" onclick="closeCfgModal()">取消</button>
                <button type="button" class="btn btn-primary btn-sm" id="cfgOkBtn" onclick="confirmOpenConfig()">确认打开</button>
            </div>
        </div>
    </div>
    {{end}}

    <script>
        var APP = {{.AppJSON}};

        function pad(n) { return (n < 10 ? '0' : '') + n; }

        // formatForInput 把 Date 格式化成输入框可接受的字面量
        // native 模式需要 "YYYY-MM-DDTHH:MM"，文本模式用 "YYYY-MM-DD HH:MM"
        function formatForInput(d, mode) {
            var s = d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
            var m = mode || APP.timeMode;
            return m === 'native' ? s.replace(' ', 'T') : s;
        }

        // parseTextDateTime 解析人工输入的文本时间，支持三种写法：
        //   2026-09-28 14:30 / 2026/9/28 14:30 / 14:30（表示今天）
        function parseTextDateTime(v) {
            if (!v) { return null; }
            var t = String(v).trim();
            var full = t.match(/^(\d{4})[-\/](\d{1,2})[-\/](\d{1,2})[ T](\d{1,2}):(\d{2})$/);
            if (full) {
                return new Date(+full[1], +full[2] - 1, +full[3], +full[4], +full[5], 0, 0);
            }
            var hm = t.match(/^(\d{1,2}):(\d{2})$/);
            if (hm) {
                var now = new Date();
                return new Date(now.getFullYear(), now.getMonth(), now.getDate(), +hm[1], +hm[2], 0, 0);
            }
            return null;
        }

        function parseStart(v) {
            if (!v) { return null; }
            if (APP.timeMode === 'native') {
                var d0 = new Date(v);
                return isNaN(d0.getTime()) ? null : d0;
            }
            return parseTextDateTime(v);
        }

        function showBanner(msg, kind) {
            var el = document.getElementById('banner');
            el.className = 'banner ' + (kind || 'info');
            el.textContent = msg;
            el.hidden = false;
        }

        function hideBanner() {
            var el = document.getElementById('banner');
            el.hidden = true;
        }

        // 表单顶部三个字段互斥联动：会议类型 / 会议号 / 自定义链接
        //   自定义  → 隐藏「会议号」，显示「自定义链接」
        //   其他    → 显示「会议号」，隐藏「自定义链接」
        // 后端与之对齐：自定义时 meeting_no 一律置空，不记录会议号
        function syncTypeFields() {
            var type = document.getElementById('meetingType').value;
            var isCustom = (type === 'custom');
            document.getElementById('customURLGroup').style.display = isCustom ? 'block' : 'none';
            document.getElementById('meetingNoGroup').style.display = isCustom ? 'none' : 'block';
        }

        // setMeetingType 把「会议类型」下拉框切到指定类型
        // 目的：让 config.json 的 default_meeting_type 真正生效（此前该配置项未被前端使用）
        // 传空值或非法值时保持当前选中项不变，避免出现空白下拉框
        function setMeetingType(t) {
            var sel = document.getElementById('meetingType');
            if (!t) { return; }
            for (var i = 0; i < sel.options.length; i++) {
                if (sel.options[i].value === t) { sel.value = t; return; }
            }
        }

        function setNow() {
            document.getElementById('startTime').value = formatForInput(new Date());
        }

        function currentPayload(triggerType) {
            var type = document.getElementById('meetingType').value;
            var startRaw = document.getElementById('startTime').value;
            var start = parseStart(startRaw);
            if (!start) { throw new Error('请填写正确的开始时间'); }
            var endRaw = document.getElementById('endTime').value;
            var end = endRaw ? parseStart(endRaw) : null;
            if (endRaw && !end) { throw new Error('结束时间格式不正确'); }
            if (end && end.getTime() < start.getTime()) { throw new Error('结束时间不能早于开始时间'); }
            var payload = {
                meeting_no: (type === 'custom') ? '' : document.getElementById('meetingNo').value,
                meeting_type: type,
                start_time: start.toISOString(),
                end_time: end ? end.toISOString() : '',
                meeting_name: document.getElementById('meetingName').value,
                applicant: document.getElementById('applicant').value,
                trigger_type: triggerType || 'scheduled'
            };
            if (type === 'custom') {
                payload.custom_url = document.getElementById('customURL').value;
            }
            return payload;
        }

        function resetForm() {
            document.getElementById('meetingForm').reset();
            document.getElementById('meetingNo').value = APP.defaultNo;
            setMeetingType(APP.defaultType);
            document.getElementById('startTime').value = '';
            document.getElementById('endTime').value = '';
            syncTypeFields();
        }

        function postMeeting(payload, btn, onDone) {
            if (btn) { btn.disabled = true; }
            hideBanner();
            fetch('/api/meetings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            }).then(function (res) {
                return res.text().then(function (text) {
                    return { status: res.status, text: text };
                });
            }).then(function (r) {
                if (r.status === 201) {
                    showBanner('已加入待执行队列。', 'ok');
                    resetForm();
                } else if (r.status === 409) {
                    showBanner('这条任务已经在队列里了（同类型、同会议号、时间相近），未重复创建。可在下方队列中查看或删除。', 'warn');
                } else {
                    showBanner('添加失败：' + (r.text || ('HTTP ' + r.status)), 'err');
                }
                if (onDone) { onDone(r.status === 201); }
                loadMeetings();
            }).catch(function (e) {
                showBanner('请求失败：' + e.message, 'err');
                if (onDone) { onDone(false); }
            }).then(function () {
                if (btn) { btn.disabled = false; }
            });
        }

        document.getElementById('meetingForm').addEventListener('submit', function (e) {
            e.preventDefault();
            var payload;
            try { payload = currentPayload('scheduled'); } catch (err) { showBanner(err.message, 'err'); return; }
            postMeeting(payload, document.getElementById('submitBtn'));
        });

        // ---- 预设按钮：自动填充 + 二次确认 ----
        var pendingPreset = null;

        function runPreset(i) {
            var p = APP.presets[i];
            if (!p) { return; }
            document.getElementById('meetingType').value = p.meeting_type;
            syncTypeFields();
            if (p.meeting_type === 'custom') {
                document.getElementById('customURL').value = p.custom_url || '';
                document.getElementById('meetingNo').value = '';
            } else {
                document.getElementById('meetingNo').value = p.meeting_no || '';
                document.getElementById('customURL').value = '';
            }
            document.getElementById('meetingName').value = p.meeting_name || '';
            setNow();
            pendingPreset = i;
            renderPresetConfirm(p);
        }

        function renderPresetConfirm(p) {
            var bar = document.getElementById('presetConfirm');
            var target = (p.meeting_type === 'custom') ? (p.custom_url || '') : (p.meeting_no || '');
            bar.innerHTML = '<div>将<b>立即启动</b>：' + escapeHtml(p.label) + '</div>' +
                '<div class="muted">会议类型：' + escapeHtml(p.meeting_type) + '　目标：' + escapeHtml(target) + '</div>' +
                '<div class="muted">时间：当前时刻（表单已自动填充，可先修改再启动）</div>' +
                '<div class="confirm-actions">' +
                '<button type="button" class="btn btn-go" id="presetOk">确认启动</button>' +
                '<button type="button" class="btn btn-ghost" id="presetCancel">取消</button>' +
                '</div>';
            bar.hidden = false;
            document.getElementById('presetOk').onclick = confirmPresetRun;
            document.getElementById('presetCancel').onclick = cancelPreset;
        }

        function confirmPresetRun() {
            if (pendingPreset === null) { return; }
            var btn = document.getElementById('presetOk');
            var payload;
            try { payload = currentPayload('immediate'); } catch (err) { showBanner(err.message, 'err'); return; }
            postMeeting(payload, btn, function (ok) {
                if (ok) { cancelPreset(); }
            });
        }

        function cancelPreset() {
            pendingPreset = null;
            document.getElementById('presetConfirm').hidden = true;
        }

        function escapeHtml(s) {
            return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
                return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
            });
        }

        // ---- 队列 ----
        var itemCache = {};

        function loadMeetings() {
            fetch('/api/meetings').then(function (res) { return res.json(); }).then(function (data) {
                var list = document.getElementById('meetingList');
                itemCache = {};
                if (!data || data.length === 0) {
                    list.innerHTML = '<p class="muted">暂无待执行会议</p>';
                    return;
                }
                var html = '';
                data.forEach(function (m) {
                    itemCache[m.id] = m;
                    html += '<div class="meeting-item">';
                    html += '<h3>' + escapeHtml(m.meeting_name || '未命名会议') + '</h3>';
                    html += '<p><strong>会议号:</strong> ' + escapeHtml(m.meeting_no || '（自定义链接）') + '</p>';
                    html += '<p><strong>类型:</strong> ' + escapeHtml(m.meeting_type) + '</p>';
                    html += '<p><strong>开始时间:</strong> ' + new Date(m.start_time).toLocaleString('zh-CN') + '</p>';
                    if (m.end_time) {
                        html += '<p><strong>结束时间:</strong> ' + new Date(m.end_time).toLocaleString('zh-CN') + '</p>';
                    }
                    if (m.applicant) {
                        html += '<p><strong>申请人:</strong> ' + escapeHtml(m.applicant) + '</p>';
                    }
                    html += '<span class="status ' + escapeHtml(m.status) + '">' + statusText(m.status) + '</span>';
                    html += '<div class="item-actions"><button class="btn btn-danger btn-sm" onclick="deleteMeeting(\'' + m.id + '\')">取出到编辑区</button></div>';
                    html += '</div>';
                });
                list.innerHTML = html;
            }).catch(function () {});
        }

        function statusText(s) {
            if (s === 'pending') { return '等待执行'; }
            if (s === 'running') { return '执行中'; }
            if (s === 'completed') { return '已执行'; }
            if (s === 'missed') { return '已错过'; }
            return s;
        }

        // 取出到编辑区：删除该任务并把内容回填到表单，相当于"修改"（轨迹为取消 + 重建，可追溯）
        function deleteMeeting(id) {
            var item = itemCache[id];
            fetch('/api/meetings?id=' + encodeURIComponent(id), { method: 'DELETE' }).then(function () {
                loadMeetings();
                if (!item) { return; }
                document.getElementById('meetingType').value = item.meeting_type;
                syncTypeFields();
                if (item.meeting_type === 'custom') {
                    document.getElementById('customURL').value = item.custom_url || '';
                    document.getElementById('meetingNo').value = '';
                } else {
                    document.getElementById('meetingNo').value = item.meeting_no || '';
                }
                document.getElementById('startTime').value = formatForInput(new Date(item.start_time));
                // 结束时间与开始时间使用同一种控件，回填格式保持一致
                document.getElementById('endTime').value = item.end_time ? formatForInput(new Date(item.end_time)) : '';
                document.getElementById('meetingName').value = item.meeting_name || '';
                document.getElementById('applicant').value = item.applicant || '';
                showBanner('已取出到编辑区，修改后重新提交即可。', 'info');
                window.scrollTo({ top: 0, behavior: 'smooth' });
            });
        }

        // ---- 最近一次执行 ----
        // 数据来自本机执行日志的最后一条；没有记录时整块隐藏
        // 口径：这只代表「客户端已被唤起」，不代表「人已确认入会」
        function loadLastDone() {
            fetch('/api/last').then(function (res) { return res.json(); }).then(function (d) {
                var card = document.getElementById('lastDoneCard');
                var body = document.getElementById('lastDoneBody');
                if (!card || !body) { return; }
                if (!d || !d.found || !d.entry) { card.hidden = true; return; }
                var e = d.entry;
                var isCustom = (e.meeting_type === 'custom');
                var targetLabel = isCustom ? '链接' : '会议号';
                var targetValue = isCustom ? (e.custom_url || '（未记录）') : (e.meeting_no || '（无）');
                var html = '';
                html += '<div><strong>类型：</strong>' + escapeHtml(typeText(e.meeting_type)) + '</div>';
                html += '<div><strong>' + targetLabel + '：</strong>' + escapeHtml(targetValue) + '</div>';
                if (e.meeting_name) { html += '<div><strong>会议名称：</strong>' + escapeHtml(e.meeting_name) + '</div>'; }
                html += '<div><strong>开始时间：</strong>' + fmtTime(e.start_time) + '</div>';
                html += '<div><strong>执行时间：</strong>' + fmtTime(e.executed_time) + '</div>';
                html += '<div><span class="res ' + escapeHtml(e.result || '') + '">' + escapeHtml(resultText(e.result)) + '</span></div>';
                body.innerHTML = html;
                card.hidden = false;
            }).catch(function () {});
        }

        function typeText(t) {
            var map = {
                quanshi: '全时云会议',
                tencent: '腾讯会议',
                zoom: 'Zoom会议',
                teams: 'Teams会议',
                xylink: '小鱼易连',
                custom: '自定义链接'
            };
            return map[t] || t;
        }

        // fmtTime 时间展示：解析不了就原样回显，避免出现 Invalid Date
        function fmtTime(s) {
            if (!s) { return '—'; }
            var d = new Date(s);
            return isNaN(d.getTime()) ? escapeHtml(s) : d.toLocaleString('zh-CN');
        }

        function resultText(k) {
            var map = {
                success: '成功唤起',
                failed: '唤起失败',
                missed: '已错过（超时未执行）',
                skipped_running: '跳过（客户端已在运行）',
                cancelled_by_user: '现场取消'
            };
            return map[k] || k;
        }

        // ---- 默认配置（必须在网页侧先二次确认）----
        // 为什么不允许"点一下就直接弹"：该操作会在会议室电脑本机弹出记事本窗口，
        // 会议进行中会打扰现场。所以先在网页里明确告知 + 二次确认，确认后才发起请求。
        //
        // 目标路径不写在页面里，而是每次点开时向服务端实时取（/api/config/info）：
        // 保证「弹窗上显示的路径」与「随后真正打开的文件」永远是同一个。
        function askOpenConfig() {
            var el = document.getElementById('cfgModalPath');
            el.textContent = '目标文件：读取中…';
            document.getElementById('cfgModal').hidden = false;
            fetch('/api/config/info').then(function (res) { return res.json(); }).then(function (d) {
                if (d && d.path) {
                    el.textContent = '目标文件：' + d.path +
                        (d.exists === false ? '（尚不存在，确认后将自动新建模板）' : '');
                } else {
                    el.textContent = '目标文件：（与程序同级的 config.json）';
                }
            }).catch(function () {
                el.textContent = '目标文件：（与程序同级的 config.json）';
            });
        }

        function closeCfgModal() {
            document.getElementById('cfgModal').hidden = true;
        }

        function confirmOpenConfig() {
            var btn = document.getElementById('cfgOkBtn');
            btn.disabled = true;
            closeCfgModal();
            fetch('/api/config/open', { method: 'POST' }).then(function (res) {
                return res.json().then(function (body) { return { status: res.status, body: body }; });
            }).then(function (r) {
                if (r.status === 200) {
                    showBanner('已在会议室终端上打开配置文件：' + r.body.path + (r.body.created ? '（新建了模板）' : '') + '。修改后请重启服务生效。', 'ok');
                } else {
                    showBanner('打开配置失败：' + (r.body.error || ('HTTP ' + r.status)), 'err');
                }
            }).catch(function (e) {
                showBanner('打开配置失败：' + e.message, 'err');
            }).then(function () { btn.disabled = false; });
        }

        // 显示使用说明弹窗
        function showHelp() {
            alert(
                '【使用说明】\n\n' +
                '1. 预约入会：\n' +
                '   填写会议号与开始时间后提交，到点由本机自动启动对应会议客户端\n' +
                '   请确保电脑上已安装全时云/腾讯会议/Zoom/Teams/小鱼易连客户端\n\n' +
                '2. 一键快捷启动：\n' +
                '   点击上方的预设按钮，表单会自动填充并弹出确认，确认后立即启动\n' +
                '   适合在会议室现场把线下会议转为线上会议的场景\n\n' +
                '3. 当前时刻按钮：\n' +
                '   点「当前时刻」把开始时间填为此刻的时间；\n' +
                '   本系统不会预设"当前时间"作为字段默认值，必须由人主动点击，避免误触\n\n' +
                '4. 会议类型：\n' +
                '   全时云(tang://)、腾讯会议(wemeet://)、Zoom(https://zoom.us/j/)、\n' +
                '   Teams(msteams://)、小鱼易连(xylink://joinMeeting)、自定义(仅http/https链接)\n' +
                '   选「自定义」时「会议号」字段会隐藏，此时只使用自定义链接，\n' +
                '   记录时会议号一栏留空\n' +
                '   字段多、格式不固定的复杂链接请选「自定义」直接粘贴完整链接\n\n' +
                '5. 同分钟多任务：\n' +
                '   同一分钟内的多个任务会依次执行\n\n' +
                '6. 防误触：\n' +
                '   提交后按钮会短暂禁用；服务端会识别重复任务并拒绝重复创建\n\n' +
                '7. 取出到编辑区：\n' +
                '   点队列里的「取出到编辑区」可删除该任务并把内容回填到表单，改完重新提交\n\n' +
                '8. 运行中会议检测：\n' +
                '   执行前会检查会议客户端是否已在运行，按终端配置的策略处理\n\n' +
                '9. 持久化存储：\n' +
                '   任务自动保存到程序目录，关机重启不丢失\n' +
                '   已执行的任务记录在 tasks_log.jsonl 中\n\n' +
                '10. 重启恢复：\n' +
                '   开机时过期任务（5分钟内）会自动补执行\n' +
                '   超过5分钟的过期任务标记为已错过，不再执行\n\n' +
                '11. 修改默认配置：\n' +
                '   点击页面右上角「修改默认配置」，会在会议室终端本机上打开同级 config.json\n' +
                '   为避免打扰正在进行的会议，点击后会先弹出网页确认框，确认后才真的弹窗\n' +
                '   可配置默认会议号、预设按钮等\n' +
                '   配置文件只在服务启动时读取一次，修改后需要手动重启程序才会生效\n' +
                '   该入口可用配置项 hide_config_entry 关闭\n' +
                '   该文件可以删除，删除后程序使用内置默认值正常运行\n\n' +
                '12. 最近一次执行：\n' +
                '   队列下方显示最近一次已执行的会议（类型、会议号、开始时间、执行时间与结果）\n' +
                '   数据取自本机执行日志，只表示客户端已被唤起，不等于已确认入会'
            );
        }

        // 初始化加载，每5秒刷新一次
        setMeetingType(APP.defaultType);
        syncTypeFields();
        loadMeetings();
        loadLastDone();
        setInterval(loadMeetings, 5000);
        setInterval(loadLastDone, 5000);
       </script>
</body>
</html>
`))
