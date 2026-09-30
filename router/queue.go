package router

import (
	"fmt"
	"log"
	"sync"
	"time"

	"meeting-launchpad/model"
)

// 幂等去重的时间窗：同类型 + 同会议号 + 开始时间相差在窗口内，视为同一次提交
const duplicateWindow = 60 * time.Second

// 补执行时间窗：开机晚于计划时间时，只有落在这个窗口内的过期任务才会补执行
const catchUpWindow = 5 * time.Minute

// MeetingQueue 任务队列
// 特点：支持持久化存储（tasks.json），关机重启后任务不丢失
type MeetingQueue struct {
	// 用一个互斥锁同时保护读写：写操作包含"改状态 + 落盘"，用 RWMutex 反而容易漏保护
	mu       sync.Mutex
	meetings []*model.Meeting
}

// queue 全局任务队列实例
var queue MeetingQueue

// AddIfAbsent 幂等新增任务
// 若已存在同类型、同会议号（或同链接）、开始时间相近的待执行任务，则不重复添加，
// 直接返回已存在的那一条 —— 这是防误触的服务端兜底，前端禁用被绕过时依然拦得住。
func (q *MeetingQueue) AddIfAbsent(m *model.Meeting) (*model.Meeting, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if existing := q.findDuplicateLocked(m); existing != nil {
		return existing.Clone(), false
	}
	q.meetings = append(q.meetings, m)
	q.sortLocked()
	q.saveLocked()
	return m.Clone(), true
}

// findDuplicateLocked 查找重复任务（调用前需持有锁）
func (q *MeetingQueue) findDuplicateLocked(m *model.Meeting) *model.Meeting {
	for _, e := range q.meetings {
		if e.Status != model.StatusPending {
			continue
		}
		if e.MeetingType != m.MeetingType {
			continue
		}
		if e.MeetingNo != m.MeetingNo || e.CustomURL != m.CustomURL {
			continue
		}
		diff := e.StartTime.Sub(m.StartTime)
		if diff < 0 {
			diff = -diff
		}
		if diff <= duplicateWindow {
			return e
		}
	}
	return nil
}

// GetAll 返回队列中所有任务的副本（避免外部持有内部指针造成并发写入）
func (q *MeetingQueue) GetAll() []*model.Meeting {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]*model.Meeting, 0, len(q.meetings))
	for _, m := range q.meetings {
		result = append(result, m.Clone())
	}
	return result
}

// Get 按 ID 返回任务副本
func (q *MeetingQueue) Get(id string) *model.Meeting {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, m := range q.meetings {
		if m.ID == id {
			return m.Clone()
		}
	}
	return nil
}

// TakeDue 取出所有已到点且待执行的任务，并在锁内把状态置为 running
// 状态先置位是为了防止同一个任务被重复触发；重启后 running 会归位为 pending
func (q *MeetingQueue) TakeDue(now time.Time) []*model.Meeting {
	q.mu.Lock()
	defer q.mu.Unlock()

	var due []*model.Meeting
	for _, m := range q.meetings {
		if m.Status == model.StatusPending && !m.StartTime.After(now) {
			m.Status = model.StatusRunning
			due = append(due, m.Clone())
		}
	}
	return due
}

// Remove 移除指定ID的任务并落盘
func (q *MeetingQueue) Remove(id string) {
	q.RemoveMany([]string{id})
}

// RemoveMany 批量移除并只落盘一次
// 修复此前"同分钟多个任务逐个删除、反复写文件"的问题
func (q *MeetingQueue) RemoveMany(ids []string) {
	if len(ids) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	drop := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		drop[id] = struct{}{}
	}
	kept := q.meetings[:0]
	for _, m := range q.meetings {
		if _, ok := drop[m.ID]; ok {
			continue
		}
		kept = append(kept, m)
	}
	q.meetings = kept
	q.saveLocked()
}

// replaceAll 用给定列表替换整个队列并落盘（用于启动时的过期任务整理）
func (q *MeetingQueue) replaceAll(list []*model.Meeting) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.meetings = list
	q.sortLocked()
	q.saveLocked()
}

// sortLocked 按开始时间升序排序（调用前需持有锁；队列规模很小，插入排序足够）
func (q *MeetingQueue) sortLocked() {
	for i := 0; i < len(q.meetings); i++ {
		for j := i + 1; j < len(q.meetings); j++ {
			if q.meetings[j].StartTime.Before(q.meetings[i].StartTime) {
				q.meetings[i], q.meetings[j] = q.meetings[j], q.meetings[i]
			}
		}
	}
}

// saveLocked 将当前队列写入 tasks.json（调用前需持有锁）
func (q *MeetingQueue) saveLocked() {
	if err := model.SaveTasks(q.meetings); err != nil {
		log.Printf("[存储] 持久化任务失败: %v", err)
	}
}

// loadFromFile 从 tasks.json 加载任务到队列
func (q *MeetingQueue) loadFromFile() {
	meetings, err := model.LoadTasks()
	if err != nil {
		log.Printf("[存储] 加载持久化任务失败: %v", err)
		return
	}
	if len(meetings) == 0 {
		return
	}
	q.replaceAll(meetings)
	log.Printf("[存储] 从文件恢复了 %d 个任务", len(meetings))
}

// startScheduler 启动后台调度：每秒检查一次队列，到点即执行
func startScheduler() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runDueTasks()
		}
	}()
}

// runDueTasks 取出到点任务并逐个执行，最后统一落盘一次
func runDueTasks() {
	due := queue.TakeDue(time.Now())
	if len(due) == 0 {
		return
	}
	finished := make([]string, 0, len(due))
	for _, m := range due {
		result, clientRunning := executeMeeting(m)
		writeTaskLog(m, result, clientRunning)
		finished = append(finished, m.ID)
	}
	queue.RemoveMany(finished)
}

// executeMeeting 执行一条任务，返回执行结果与"执行时客户端是否已在运行"
func executeMeeting(m *model.Meeting) (string, bool) {
	cfg := model.Current()
	clientRunning := false

	// 运行中会议检测：避免重复唤起打断正在进行的会议
	if cfg.ClientRunningAction != model.ClientRunningOff {
		names := cfg.ClientProcessNames[string(m.MeetingType)]
		if running, procName := model.IsAnyProcessRunning(names); running {
			clientRunning = true
			switch cfg.ClientRunningAction {
			case model.ClientRunningSkip:
				log.Printf("[执行] 检测到客户端已在运行(%s)，按 skip 策略跳过: %s", procName, m.MeetingNo)
				return model.ResultSkippedRunning, true
			case model.ClientRunningConfirm:
				question := fmt.Sprintf(
					"检测到会议客户端已在运行：%s\n\n计划入会：%s\n\n现在启动这个入会链接吗？\n（%d 秒内无响应将按默认执行）",
					procName, describeMeeting(m), cfg.ConfirmTimeoutSec,
				)
				if !model.ConfirmDialog(model.AppName, question, cfg.ConfirmTimeoutSec, true) {
					log.Printf("[执行] 现场人员选择取消: %s", m.MeetingNo)
					return model.ResultCancelledByUser, true
				}
			default: // warn：照常执行，只做记录
				log.Printf("[执行] 检测到客户端已在运行(%s)，按 warn 策略照常执行: %s", procName, m.MeetingNo)
			}
		}
	}

	target := m.ToURL()
	log.Printf("[执行] 唤起: %s", target)
	if err := model.LaunchURL(target); err != nil {
		log.Printf("[执行] 唤起失败: %v", err)
		return model.ResultFailed, clientRunning
	}
	return model.ResultSuccess, clientRunning
}

// describeMeeting 生成用于弹窗展示的简短描述
func describeMeeting(m *model.Meeting) string {
	if m.MeetingName != "" {
		return fmt.Sprintf("%s（%s %s）", m.MeetingName, m.MeetingType, m.MeetingNo)
	}
	return fmt.Sprintf("%s %s", m.MeetingType, m.MeetingNo)
}

// writeTaskLog 写入一条执行日志
func writeTaskLog(m *model.Meeting, result string, clientRunning bool) {
	entry := model.CreateTaskLog(m, result, model.Current().RoomNo, clientRunning)
	if err := model.AppendTaskLog(entry); err != nil {
		log.Printf("[存储] 写入执行日志失败: %v", err)
	}
}

// processExpiredTasks 处理启动时发现的过期任务
// 策略：只补执行"最新的一条且在时间窗内"的过期任务，其余标记为 missed 并写入日志
func processExpiredTasks() {
	now := time.Now()
	all := queue.GetAll()
	if len(all) == 0 {
		return
	}

	expired := make([]*model.Meeting, 0)
	future := make([]*model.Meeting, 0, len(all))
	for _, m := range all {
		if m.Status == model.StatusPending && !m.StartTime.After(now) {
			expired = append(expired, m)
		} else {
			future = append(future, m)
		}
	}
	if len(expired) == 0 {
		return
	}

	log.Printf("[启动] 发现 %d 个过期任务", len(expired))
	sortDescByStartTime(expired)

	latest := expired[0]
	if time.Since(latest.StartTime) <= catchUpWindow {
		result, clientRunning := executeMeeting(latest)
		writeTaskLog(latest, result, clientRunning)
		log.Printf("[启动] 补执行最新过期任务 %s（原定 %s）结果: %s",
			latest.MeetingNo, latest.StartTime.Format(time.RFC3339), result)
	} else {
		writeTaskLog(latest, model.ResultMissed, false)
		log.Printf("[启动] 最新过期任务已超出 %s 时间窗，标记为 missed: %s",
			catchUpWindow, latest.MeetingNo)
	}

	for i := 1; i < len(expired); i++ {
		m := expired[i]
		writeTaskLog(m, model.ResultMissed, false)
		log.Printf("[启动] 过期任务标记为 missed: %s（原定 %s）",
			m.MeetingNo, m.StartTime.Format(time.RFC3339))
	}

	queue.replaceAll(future)
	log.Printf("[启动] 过期任务处理完成，剩余 %d 个待执行任务", len(future))
}

// sortDescByStartTime 按开始时间降序排序（最新的在前）
func sortDescByStartTime(meetings []*model.Meeting) {
	for i := 0; i < len(meetings); i++ {
		for j := i + 1; j < len(meetings); j++ {
			if meetings[j].StartTime.After(meetings[i].StartTime) {
				meetings[i], meetings[j] = meetings[j], meetings[i]
			}
		}
	}
}
