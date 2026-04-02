package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// Task 任务数据
type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"` // pending / in_progress / completed / stopped
	Description string `json:"description,omitempty"`
}

// TaskStore 内存任务存储
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[int]*Task
	nextID atomic.Int32
}

// 全局任务存储
var globalTaskStore = &TaskStore{
	tasks: make(map[int]*Task),
}

func init() {
	globalTaskStore.nextID.Store(1)
}

// TaskTool 任务管理工具
type TaskTool struct{}

type taskInput struct {
	Action      string `json:"action"` // create / get / list / update / stop
	ID          int    `json:"id,omitempty"`
	Title       string `json:"title,omitempty"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
}

func (t *TaskTool) Name() string        { return "Task" }
func (t *TaskTool) Description() string {
	return "任务管理：创建、查询、更新、停止任务。actions: create/get/list/update/stop"
}
func (t *TaskTool) IsReadOnly() bool    { return false }
func (t *TaskTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":      map[string]any{"type": "string", "description": "操作：create/get/list/update/stop"},
			"id":          map[string]any{"type": "integer", "description": "任务 ID（get/update/stop 时必填）"},
			"title":       map[string]any{"type": "string", "description": "任务标题（create 时必填）"},
			"status":      map[string]any{"type": "string", "description": "任务状态（update 时使用）"},
			"description": map[string]any{"type": "string", "description": "任务描述"},
		},
		"required": []string{"action"},
	}
}

func (t *TaskTool) Execute(_ context.Context, input json.RawMessage) (*tool.Result, error) {
	var in taskInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	store := globalTaskStore

	switch in.Action {
	case "create":
		if in.Title == "" {
			return &tool.Result{Content: "title is required for create", IsError: true}, nil
		}
		id := int(store.nextID.Add(1) - 1)
		task := &Task{
			ID:          id,
			Title:       in.Title,
			Status:      "pending",
			Description: in.Description,
		}
		store.mu.Lock()
		store.tasks[id] = task
		store.mu.Unlock()

		data, _ := json.MarshalIndent(task, "", "  ")
		return &tool.Result{Content: fmt.Sprintf("created task:\n%s", string(data))}, nil

	case "get":
		store.mu.RLock()
		task, ok := store.tasks[in.ID]
		store.mu.RUnlock()
		if !ok {
			return &tool.Result{Content: fmt.Sprintf("task #%d not found", in.ID), IsError: true}, nil
		}
		data, _ := json.MarshalIndent(task, "", "  ")
		return &tool.Result{Content: string(data)}, nil

	case "list":
		store.mu.RLock()
		defer store.mu.RUnlock()
		if len(store.tasks) == 0 {
			return &tool.Result{Content: "no tasks"}, nil
		}
		var sb strings.Builder
		for _, task := range store.tasks {
			sb.WriteString(fmt.Sprintf("#%d [%s] %s\n", task.ID, task.Status, task.Title))
		}
		return &tool.Result{Content: sb.String()}, nil

	case "update":
		store.mu.Lock()
		task, ok := store.tasks[in.ID]
		if !ok {
			store.mu.Unlock()
			return &tool.Result{Content: fmt.Sprintf("task #%d not found", in.ID), IsError: true}, nil
		}
		if in.Title != "" {
			task.Title = in.Title
		}
		if in.Status != "" {
			task.Status = in.Status
		}
		if in.Description != "" {
			task.Description = in.Description
		}
		store.mu.Unlock()
		data, _ := json.MarshalIndent(task, "", "  ")
		return &tool.Result{Content: fmt.Sprintf("updated task:\n%s", string(data))}, nil

	case "stop":
		store.mu.Lock()
		task, ok := store.tasks[in.ID]
		if !ok {
			store.mu.Unlock()
			return &tool.Result{Content: fmt.Sprintf("task #%d not found", in.ID), IsError: true}, nil
		}
		task.Status = "stopped"
		store.mu.Unlock()
		return &tool.Result{Content: fmt.Sprintf("task #%d stopped", in.ID)}, nil

	default:
		return &tool.Result{Content: fmt.Sprintf("unknown action: %s", in.Action), IsError: true}, nil
	}
}
