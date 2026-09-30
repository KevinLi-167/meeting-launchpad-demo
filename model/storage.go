package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 持久化存储相关常量
const (
	tasksFileName    = "tasks.json"      // 待执行任务文件（与内存同步，覆写模式）
	tasksLogFileName = "tasks_log.jsonl" // 已执行记录文件（追加写入，每行一个JSON）
)

// PersistedTask 持久化存储的任务结构
// 与 Meeting 结构对应，但时间使用字符串存储避免时区问题
type PersistedTask struct {
	ID            string `json:"id"`
	StartTime     string `json:"start_time"`         // RFC3339格式
	EndTime       string `json:"end_time,omitempty"` // RFC3339格式（可选）
	MeetingNo     string `json:"meeting_no"`
	MeetingType   string `json:"meeting_type"`
	CustomURL     string `json:"custom_url,omitempty"`
	MeetingName   string `json:"meeting_name,omitempty"`
	Applicant     string `json:"applicant,omitempty"`
	Status        string `json:"status"`                 // pending / running / completed / missed
	ScheduledTime string `json:"scheduled_time"`         // 任务登记时刻（审计）
	ClientIP      string `json:"client_ip,omitempty"`    // 发起方 IP（审计）
	TriggerType   string `json:"trigger_type,omitempty"` // scheduled / immediate（审计）
}

// TaskLog 已执行任务的日志记录
// 除业务字段外，额外记录发起方 IP、登记时刻、触发方式与执行时的客户端运行状态，用于内部追溯
type TaskLog struct {
	ID            string `json:"id"`
	MeetingNo     string `json:"meeting_no"`
	MeetingType   string `json:"meeting_type"`
	CustomURL     string `json:"custom_url,omitempty"` // 自定义链接（仅 custom 类型，用于溯源"当时开的是哪个会"）
	MeetingName   string `json:"meeting_name,omitempty"`
	Applicant     string `json:"applicant,omitempty"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time,omitempty"`
	ScheduledTime string `json:"scheduled_time"`           // 任务登记时刻
	ExecutedTime  string `json:"executed_time"`            // 实际执行（判定）时间
	Result        string `json:"result"`                   // success / failed / missed / skipped_running / cancelled_by_user
	RoomNo        int    `json:"room_no"`                  // 终端编号
	ClientIP      string `json:"client_ip,omitempty"`      // 发起方 IP
	TriggerType   string `json:"trigger_type,omitempty"`   // scheduled / immediate
	ClientRunning bool   `json:"client_running,omitempty"` // 执行时是否检测到客户端已在运行
}

// 持久化存储的全局锁
var storageMutex sync.Mutex

// 数据目录（解析一次后固定）
var (
	dataDirOnce sync.Once
	dataDirPath string
)

// DataDir 返回运行时数据文件所在目录：优先取可执行文件目录
// 若旧版本的数据文件残留在"当前工作目录"（任务计划程序「起始于」配错时会这样），
// 自动迁移到可执行文件目录，保证升级后历史记录不丢
func DataDir() string {
	dataDirOnce.Do(func() {
		exeDir := ExecutableDir()
		dataDirPath = exeDir
		migrateLegacyData(exeDir)
	})
	return dataDirPath
}

// migrateLegacyData 将工作目录下的历史数据文件迁移到 exe 目录（仅当目标不存在时）
func migrateLegacyData(exeDir string) {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		return
	}
	absExe, e1 := filepath.Abs(exeDir)
	absCwd, e2 := filepath.Abs(cwd)
	if e1 != nil || e2 != nil || absExe == absCwd {
		return
	}
	for _, name := range []string{tasksFileName, tasksLogFileName} {
		dst := filepath.Join(absExe, name)
		if _, err := os.Stat(dst); err == nil {
			continue // 目标已存在，不做任何处理
		}
		src := filepath.Join(absCwd, name)
		data, err := os.ReadFile(src)
		if err != nil || len(data) == 0 {
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			log.Printf("[存储] 迁移历史文件失败 %s -> %s: %v", src, dst, err)
			continue
		}
		log.Printf("[存储] 已将历史数据文件迁移到程序目录: %s", name)
	}
}

// tasksPath 返回 tasks.json 的完整路径
func tasksPath() string { return filepath.Join(DataDir(), tasksFileName) }

// tasksLogPath 返回 tasks_log.jsonl 的完整路径
func tasksLogPath() string { return filepath.Join(DataDir(), tasksLogFileName) }

// MeetingToPersistedTask 将内存中的 Meeting 转为持久化任务结构
func MeetingToPersistedTask(m *Meeting) PersistedTask {
	pt := PersistedTask{
		ID:            m.ID,
		StartTime:     m.StartTime.Format(time.RFC3339),
		MeetingNo:     m.MeetingNo,
		MeetingType:   string(m.MeetingType),
		CustomURL:     m.CustomURL,
		MeetingName:   m.MeetingName,
		Applicant:     m.Applicant,
		Status:        m.Status,
		ScheduledTime: formatTime(m.ScheduledTime),
		ClientIP:      m.ClientIP,
		TriggerType:   m.TriggerType,
	}
	if m.EndTime != nil {
		pt.EndTime = m.EndTime.Format(time.RFC3339)
	}
	return pt
}

// PersistedTaskToMeeting 将持久化任务结构转为内存中的 Meeting
func PersistedTaskToMeeting(pt PersistedTask) (*Meeting, error) {
	startTime, err := time.Parse(time.RFC3339, pt.StartTime)
	if err != nil {
		return nil, fmt.Errorf("解析开始时间失败: %v", err)
	}

	var endTime *time.Time
	if pt.EndTime != "" {
		et, err := time.Parse(time.RFC3339, pt.EndTime)
		if err != nil {
			return nil, fmt.Errorf("解析结束时间失败: %v", err)
		}
		endTime = &et
	}

	status := pt.Status
	// 重启后把运行期瞬时状态归位，避免进程中断导致任务永久卡在 running
	if status == "" || status == StatusRunning {
		status = StatusPending
	}
	triggerType := pt.TriggerType
	if triggerType == "" {
		triggerType = TriggerScheduled
	}

	return &Meeting{
		ID:            pt.ID,
		StartTime:     startTime,
		EndTime:       endTime,
		MeetingNo:     pt.MeetingNo,
		MeetingType:   MeetingType(pt.MeetingType),
		CustomURL:     pt.CustomURL,
		MeetingName:   pt.MeetingName,
		Applicant:     pt.Applicant,
		Status:        status,
		ScheduledTime: parseTimeOrZero(pt.ScheduledTime),
		ClientIP:      pt.ClientIP,
		TriggerType:   triggerType,
	}, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseTimeOrZero(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// SaveTasks 将任务列表原子写入 tasks.json
// 使用临时文件+重命名策略防止写入过程中断导致文件损坏
func SaveTasks(tasks []*Meeting) error {
	storageMutex.Lock()
	defer storageMutex.Unlock()

	// 转换 Meeting 列表为 PersistedTask 列表
	ptList := make([]PersistedTask, 0, len(tasks))
	for _, m := range tasks {
		pt := MeetingToPersistedTask(m)
		ptList = append(ptList, pt)
	}

	data, err := json.MarshalIndent(ptList, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化任务失败: %v", err)
	}

	// 原子写入：先写临时文件，再重命名
	path := tasksPath()
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %v", err)
	}
	if err := os.Rename(tmpFile, path); err != nil {
		// 重命名失败时尝试清理临时文件
		os.Remove(tmpFile)
		return fmt.Errorf("重命名文件失败: %v", err)
	}

	return nil
}

// LoadTasks 从 tasks.json 读取待执行任务列表
// 文件不存在时返回空列表，不报错
func LoadTasks() ([]*Meeting, error) {
	storageMutex.Lock()
	defer storageMutex.Unlock()

	data, err := os.ReadFile(tasksPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在，返回空列表
		}
		return nil, fmt.Errorf("读取任务文件失败: %v", err)
	}

	// 空文件返回空列表
	if len(data) == 0 {
		return nil, nil
	}

	var ptList []PersistedTask
	if err := json.Unmarshal(data, &ptList); err != nil {
		return nil, fmt.Errorf("解析任务文件失败: %v", err)
	}

	meetings := make([]*Meeting, 0, len(ptList))
	for _, pt := range ptList {
		m, err := PersistedTaskToMeeting(pt)
		if err != nil {
			// 单条解析失败，记录但继续处理其他任务
			log.Printf("[存储] 跳过一条无法解析的任务记录: %v", err)
			continue
		}
		meetings = append(meetings, m)
	}

	return meetings, nil
}

// AppendTaskLog 追加写入一条已执行记录到 tasks_log.jsonl
// 每行一个JSON，追加模式写入
func AppendTaskLog(entry TaskLog) error {
	storageMutex.Lock()
	defer storageMutex.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化日志失败: %v", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(tasksLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("写入日志文件失败: %v", err)
	}
	return nil
}

// ReadLatestTaskLog 读取最后一条执行记录（页面「最近一次执行」用）
//
// 日志按追加写入，因此文件中最后一条可解析的记录就是最近一次执行。
// 这里顺序扫描并持续覆盖结果，避免为了取一条而把整个文件读进内存。
// ok=false 表示暂无记录（文件不存在或为空），这不是错误。
func ReadLatestTaskLog() (entry TaskLog, ok bool, err error) {
	f, err := os.Open(tasksLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return TaskLog{}, false, nil
		}
		return TaskLog{}, false, fmt.Errorf("打开日志文件失败: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e TaskLog
		if err := json.Unmarshal(line, &e); err != nil {
			continue // 跳过损坏行，不影响最新记录的判定
		}
		entry = e
		ok = true
	}
	return entry, ok, nil
}

// CreateTaskLog 从 Meeting 创建一条 TaskLog 记录
// clientRunning 表示执行时是否检测到会议客户端已在运行
func CreateTaskLog(m *Meeting, result string, roomNo int, clientRunning bool) TaskLog {
	entry := TaskLog{
		ID:            m.ID,
		MeetingNo:     m.MeetingNo,
		MeetingType:   string(m.MeetingType),
		CustomURL:     m.CustomURL,
		MeetingName:   m.MeetingName,
		Applicant:     m.Applicant,
		StartTime:     m.StartTime.Format(time.RFC3339),
		ScheduledTime: formatTime(m.ScheduledTime),
		ExecutedTime:  time.Now().Format(time.RFC3339),
		Result:        result,
		RoomNo:        roomNo,
		ClientIP:      m.ClientIP,
		TriggerType:   m.TriggerType,
		ClientRunning: clientRunning,
	}
	if m.EndTime != nil {
		entry.EndTime = m.EndTime.Format(time.RFC3339)
	}
	return entry
}
