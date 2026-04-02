package tool

import (
	"context"
	"sync"

	"github.com/xincode-ai/xin-code/internal/provider"
)

const maxConcurrentReadTools = 10

// ExecuteResult 单个工具执行结果
type ExecuteResult struct {
	ToolUseID string
	Result    *Result
}

// ExecuteBatch 批量执行工具调用
// 只读工具并发执行，写入工具顺序执行
func (r *Registry) ExecuteBatch(ctx context.Context, calls []*provider.ToolCall, checker PermissionChecker) []ExecuteResult {
	results := make([]ExecuteResult, len(calls))

	// 分组：只读 vs 写入
	type indexedCall struct {
		index int
		call  *provider.ToolCall
	}
	var readCalls, writeCalls []indexedCall

	for i, call := range calls {
		t, ok := r.Get(call.Name)
		if !ok || !t.IsReadOnly() {
			writeCalls = append(writeCalls, indexedCall{i, call})
		} else {
			readCalls = append(readCalls, indexedCall{i, call})
		}
	}

	// 只读工具并发执行
	if len(readCalls) > 0 {
		sem := make(chan struct{}, maxConcurrentReadTools)
		var wg sync.WaitGroup
		for _, ic := range readCalls {
			wg.Add(1)
			sem <- struct{}{}
			go func(ic indexedCall) {
				defer wg.Done()
				defer func() { <-sem }()
				result := r.executeWithPermission(ctx, ic.call, checker)
				results[ic.index] = ExecuteResult{
					ToolUseID: ic.call.ID,
					Result:    result,
				}
			}(ic)
		}
		wg.Wait()
	}

	// 写入工具顺序执行
	for _, ic := range writeCalls {
		result := r.executeWithPermission(ctx, ic.call, checker)
		results[ic.index] = ExecuteResult{
			ToolUseID: ic.call.ID,
			Result:    result,
		}
	}

	return results
}

func (r *Registry) executeWithPermission(ctx context.Context, call *provider.ToolCall, checker PermissionChecker) *Result {
	t, ok := r.Get(call.Name)
	if !ok {
		return &Result{Content: "unknown tool: " + call.Name, IsError: true}
	}

	// 权限检查
	if checker != nil {
		allowed, reason := checker.Check(call.Name, t.IsReadOnly())
		if !allowed {
			return &Result{Content: "permission denied: " + reason, IsError: true}
		}
	}

	result, err := t.Execute(ctx, []byte(call.Input))
	if err != nil {
		return &Result{Content: "execution error: " + err.Error(), IsError: true}
	}
	return result
}
