package server

import (
	"sort"
	"sync"
	"time"
)

type externalTask struct {
	ID        string
	RelID     string
	Source    string
	Type      string
	Message   string
	Status    string
	SubTasks  int
	Retryable bool
	DetailURL string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	extTaskMu   sync.Mutex
	extTaskRepo = make(map[string]*externalTask)
)

func upsertExternalTask(t externalTask) {
	if t.ID == "" {
		return
	}
	extTaskMu.Lock()
	defer extTaskMu.Unlock()
	cp := t
	if exist, ok := extTaskRepo[t.ID]; ok {
		*exist = cp
		return
	}
	extTaskRepo[t.ID] = &cp
}

func listExternalTasks(limit int) []*externalTask {
	extTaskMu.Lock()
	defer extTaskMu.Unlock()
	out := make([]*externalTask, 0, len(extTaskRepo))
	for _, t := range extTaskRepo {
		if t != nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit <= 0 {
		limit = 200
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

