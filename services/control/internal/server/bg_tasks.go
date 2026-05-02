package server

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type bgTask struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`   // license.verify | system.report
	Status    string    `json:"status"` // running | success | failed
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	bgTaskMu   sync.Mutex
	bgTaskList []*bgTask
)

func runBGTask(kind string, fn func() (string, error)) *bgTask {
	t := &bgTask{
		ID:        uuid.NewString(),
		Type:      kind,
		Status:    "running",
		Message:   "running",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	bgTaskMu.Lock()
	bgTaskList = append(bgTaskList, t)
	// Trim list to keep only the last 200 entries
	if len(bgTaskList) > 200 {
		bgTaskList = bgTaskList[len(bgTaskList)-200:]
	}
	bgTaskMu.Unlock()

	msg, err := fn()
	bgTaskMu.Lock()
	defer bgTaskMu.Unlock()
	if err != nil {
		t.Status = "failed"
		t.Message = err.Error()
	} else {
		t.Status = "success"
		t.Message = msg
	}
	t.UpdatedAt = time.Now()
	return t
}

func listBGTasks(limit int) []*bgTask {
	bgTaskMu.Lock()
	defer bgTaskMu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	n := len(bgTaskList)
	start := 0
	if n > limit {
		start = n - limit
	}
	out := make([]*bgTask, 0, n-start)
	for _, t := range bgTaskList[start:] {
		if t != nil {
			out = append(out, t)
		}
	}
	return out
}

