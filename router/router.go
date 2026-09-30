package router

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"meeting-launchpad/model"
)

// lastResult 「最近一次执行」接口的响应结构
// Found=false 表示暂无执行记录（页面不展示该区块），不是错误
type lastResult struct {
	Found bool           `json:"found"`
	Entry *model.TaskLog `json:"entry,omitempty"`
}

// Start 启动HTTP服务器，注册路由并开始任务调度
// cfg: 当前终端生效的配置；addr: 监听地址，格式如 ":8080"
func Start(cfg model.Config, addr string) {
	// 从 tasks.json 恢复持久化的任务
	queue.loadFromFile()

	// 处理开机时发现的过期任务：只补执行时间窗内最新的一条，其余标记为 missed
	processExpiredTasks()

	// 启动后台调度，每秒检查一次队列
	startScheduler()

	// 注册HTTP路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, "404 page not found", http.StatusNotFound)
			return
		}
		HandleWebUI(w, r, cfg)
	})
	http.HandleFunc("/api/meetings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			HandleListMeetings(w, r)
		case http.MethodPost:
			HandleAddMeeting(w, r)
		case http.MethodDelete:
			HandleDeleteMeeting(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		}
	})
	http.HandleFunc("/api/last", HandleLastExecuted)
	http.HandleFunc("/api/config/info", func(w http.ResponseWriter, r *http.Request) {
		HandleConfigInfo(w, r, cfg)
	})
	http.HandleFunc("/api/config/open", func(w http.ResponseWriter, r *http.Request) {
		HandleOpenConfig(w, r, cfg)
	})

	log.Printf("[服务] 监听 %s，终端 #%d %s", addr, cfg.RoomNo, cfg.RoomName)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// HandleWebUI 输出终端操作页面
// GET /
func HandleWebUI(w http.ResponseWriter, r *http.Request, cfg model.Config) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte(renderPage(cfg))); err != nil {
		log.Printf("[UI] 输出页面失败: %v", err)
	}
}

// HandleAddMeeting 处理添加会议请求
// POST /api/meetings
func HandleAddMeeting(w http.ResponseWriter, r *http.Request) {
	var req model.MeetingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法解析")
		return
	}

	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartTime))
	if err != nil {
		writeError(w, http.StatusBadRequest, "开始时间格式不正确，需为 RFC3339")
		return
	}

	var endTime *time.Time
	if s := strings.TrimSpace(req.EndTime); s != "" {
		et, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "结束时间格式不正确，需为 RFC3339")
			return
		}
		if et.Before(startTime) {
			writeError(w, http.StatusBadRequest, "结束时间不能早于开始时间")
			return
		}
		endTime = &et
	}

	meetingType := req.MeetingType
	if meetingType == "" {
		meetingType = model.MeetingType(model.Current().DefaultMeetingType)
	}
	if !model.IsValidMeetingType(meetingType) {
		writeError(w, http.StatusBadRequest, "会议类型取值非法")
		return
	}

	// 输入校验：内置类型限定会议号字符集与长度；自定义类型只做协议白名单 + 长度上限
	meetingNo := strings.TrimSpace(req.MeetingNo)
	customURL := strings.TrimSpace(req.CustomURL)
	if meetingType == model.MeetingTypeCustom {
		if err := model.ValidateCustomURL(customURL); err != nil {
			writeError(w, http.StatusBadRequest, "自定义链接不合法："+err.Error())
			return
		}
		meetingNo = "" // 自定义通道不使用会议号
	} else {
		if err := model.ValidateMeetingNo(meetingNo); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		customURL = ""
	}
	if err := model.ValidateText("会议名称", req.MeetingName, model.MaxTextLen); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := model.ValidateText("申请人", req.Applicant, model.MaxApplicantLen); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	triggerType := req.TriggerType
	if triggerType != model.TriggerImmediate {
		triggerType = model.TriggerScheduled
	}

	meeting := &model.Meeting{
		ID:            fmt.Sprintf("%d", time.Now().UnixNano()),
		StartTime:     startTime,
		EndTime:       endTime,
		MeetingNo:     meetingNo,
		MeetingType:   meetingType,
		CustomURL:     customURL,
		MeetingName:   strings.TrimSpace(req.MeetingName),
		Applicant:     strings.TrimSpace(req.Applicant),
		Status:        model.StatusPending,
		ScheduledTime: time.Now(),
		ClientIP:      clientIP(r),
		TriggerType:   triggerType,
	}

	saved, created := queue.AddIfAbsent(meeting)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !created {
		// 幂等命中：不重复创建，直接回传已存在的那一条
		log.Printf("[服务] 重复任务已忽略: %s", saved.ID)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "已存在相同会议号、时间相近的待执行任务，未重复创建",
			"meeting": saved,
		})
		return
	}

	log.Printf("[服务] 新任务已添加并持久化: %s（%s）", saved.ID, triggerType)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(saved)
}

// HandleListMeetings 获取所有待执行的会议
// GET /api/meetings
func HandleListMeetings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(queue.GetAll()); err != nil {
		log.Printf("[服务] 输出任务列表失败: %v", err)
	}
}

// HandleDeleteMeeting 删除指定ID的会议
// DELETE /api/meetings?id=xxx
func HandleDeleteMeeting(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "缺少 id 参数")
		return
	}
	queue.Remove(id)
	log.Printf("[服务] 任务已移除: %s", id)
	w.WriteHeader(http.StatusNoContent)
}

// HandleLastExecuted 返回最近一条执行记录，供页面「最近一次执行」区块展示
// GET /api/last
// 口径说明：记录的是「任务触发记录」，不等于「已确认入会」
func HandleLastExecuted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}

	entry, found, err := model.ReadLatestTaskLog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取执行记录失败："+err.Error())
		return
	}

	result := lastResult{Found: found}
	if found {
		result.Entry = &entry
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("[服务] 输出最近执行记录失败: %v", err)
	}
}

// HandleConfigInfo 返回配置文件的真实路径与当前是否存在（不打开任何窗口）
// GET /api/config/info
//
// 用途：页面「修改默认配置」的二次确认框在展示前实时取一次路径。
// 路径若在页面渲染时就写进 HTML，一旦浏览器标签页长时间未刷新（或程序已被换新
// 重启过），弹窗里显示的就会是旧实例的路径，与随后真正打开的文件不一致。
func HandleConfigInfo(w http.ResponseWriter, r *http.Request, cfg model.Config) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}
	path, exists := model.ConfigFileInfo(cfg)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":   path,
		"exists": exists,
	})
}

// HandleOpenConfig 在服务端本机打开（必要时先创建）配置文件
// POST /api/config/open
func HandleOpenConfig(w http.ResponseWriter, r *http.Request, cfg model.Config) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}

	path, created, err := model.EnsureConfigFile(cfg)
	if err != nil {
		log.Printf("[配置] 打开配置失败: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.OpenConfigFile(path); err != nil {
		log.Printf("[配置] 调用系统默认程序失败: %v", err)
		writeError(w, http.StatusInternalServerError, "配置文件已就绪，但无法在本机打开："+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":    path,
		"created": created,
		"note":    "已在会议室终端本机打开；修改后请重启服务生效",
	})
}

// clientIP 取请求来源 IP（用于审计留痕，不在页面上展示）
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.Index(fwd, ","); i > 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeError 以纯文本方式返回错误（与既有接口保持一致的响应形态）
func writeError(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}
