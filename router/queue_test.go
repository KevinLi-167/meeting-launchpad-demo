package router

import (
	"testing"
	"time"

	"meeting-launchpad/model"
)

func newMeeting(id string, no string, start time.Time) *model.Meeting {
	return &model.Meeting{
		ID:            id,
		StartTime:     start,
		MeetingNo:     no,
		MeetingType:   model.MeetingTypeQuanshi,
		Status:        model.StatusPending,
		ScheduledTime: time.Now(),
		TriggerType:   model.TriggerScheduled,
	}
}

// TestAddIfAbsentRejectsDuplicate 幂等：同类型 + 同会议号 + 时间相近 → 只保留一条
func TestAddIfAbsentRejectsDuplicate(t *testing.T) {
	var q MeetingQueue
	base := time.Date(2026, 9, 28, 14, 0, 0, 0, time.Local)

	if _, created := q.AddIfAbsent(newMeeting("a", "89339802", base)); !created {
		t.Fatal("第一条应创建成功")
	}
	if _, created := q.AddIfAbsent(newMeeting("b", "89339802", base.Add(30*time.Second))); created {
		t.Fatal("时间窗内的同号任务应被判为重复")
	}
	if got := len(q.GetAll()); got != 1 {
		t.Fatalf("队列长度应为 1，实际 %d", got)
	}

	if _, created := q.AddIfAbsent(newMeeting("c", "89339802", base.Add(10*time.Minute))); !created {
		t.Fatal("超出时间窗的同号任务应允许创建")
	}
	if _, created := q.AddIfAbsent(newMeeting("d", "88990011", base)); !created {
		t.Fatal("不同会议号应允许创建")
	}
	if got := len(q.GetAll()); got != 3 {
		t.Fatalf("队列长度应为 3，实际 %d", got)
	}
}

// TestAddIfAbsentIgnoresNonPending 非待执行状态的任务不参与去重判断
func TestAddIfAbsentIgnoresNonPending(t *testing.T) {
	var q MeetingQueue
	base := time.Date(2026, 9, 28, 14, 0, 0, 0, time.Local)

	done := newMeeting("a", "89339802", base)
	done.Status = model.StatusCompleted
	q.AddIfAbsent(done)

	if _, created := q.AddIfAbsent(newMeeting("b", "89339802", base)); !created {
		t.Fatal("已完成的任务不应阻止新任务创建")
	}
}

// TestTakeDueMarksRunning 到点任务只被取出一次，且状态先置为 running
func TestTakeDueMarksRunning(t *testing.T) {
	var q MeetingQueue
	base := time.Date(2026, 9, 28, 14, 0, 0, 0, time.Local)
	q.AddIfAbsent(newMeeting("due", "89339802", base))
	q.AddIfAbsent(newMeeting("future", "88990011", base.Add(time.Hour)))

	due := q.TakeDue(base.Add(time.Second))
	if len(due) != 1 || due[0].ID != "due" {
		t.Fatalf("应取出 1 条到点任务，实际 %+v", due)
	}
	if again := q.TakeDue(base.Add(2 * time.Second)); len(again) != 0 {
		t.Fatalf("同一任务不应被重复取出，实际 %+v", again)
	}
	if q.Get("due").Status != model.StatusRunning {
		t.Fatalf("取出后状态应为 running")
	}
}

// TestRemoveManyRemovesAll 批量删除应一次移除全部指定任务
func TestRemoveManyRemovesAll(t *testing.T) {
	var q MeetingQueue
	base := time.Date(2026, 9, 28, 14, 0, 0, 0, time.Local)
	q.AddIfAbsent(newMeeting("a", "89339802", base))
	q.AddIfAbsent(newMeeting("b", "88990011", base))
	q.AddIfAbsent(newMeeting("c", "88990022", base))

	q.RemoveMany([]string{"a", "c"})
	left := q.GetAll()
	if len(left) != 1 || left[0].ID != "b" {
		t.Fatalf("批量删除结果不正确: %+v", left)
	}

	q.RemoveMany(nil) // 空入参不应出错
	if len(q.GetAll()) != 1 {
		t.Fatalf("空入参不应改变队列")
	}
}

// TestGetAllReturnsCopies 返回的必须是副本，外部改动不能影响内部状态
func TestGetAllReturnsCopies(t *testing.T) {
	var q MeetingQueue
	base := time.Date(2026, 9, 28, 14, 0, 0, 0, time.Local)
	q.AddIfAbsent(newMeeting("a", "89339802", base))

	snapshot := q.GetAll()
	snapshot[0].Status = model.StatusCompleted
	snapshot[0].MeetingNo = "0000000000"

	inner := q.Get("a")
	if inner.Status != model.StatusPending {
		t.Fatalf("内部状态被外部修改: %q", inner.Status)
	}
	if inner.MeetingNo != "89339802" {
		t.Fatalf("内部数据被外部修改: %q", inner.MeetingNo)
	}
}
