package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

// JSON-RPC 消息结构

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// StdioTransport 通过子进程 stdin/stdout 的 MCP 传输
type StdioTransport struct {
	command string
	args    []string
	env     map[string]string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	mu       sync.Mutex
	nextID   atomic.Int64
	pending  map[int64]chan *jsonrpcResponse
	pendMu   sync.Mutex
	closed   bool
}

// NewStdioTransport 创建 stdio 传输
func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport {
	return &StdioTransport{
		command: command,
		args:    args,
		env:     env,
		pending: make(map[int64]chan *jsonrpcResponse),
	}
}

// Start 启动子进程
func (t *StdioTransport) Start(ctx context.Context) error {
	t.cmd = exec.CommandContext(ctx, t.command, t.args...)

	// 设置环境变量
	t.cmd.Env = os.Environ()
	for k, v := range t.env {
		t.cmd.Env = append(t.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取 stdin 失败: %w", err)
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取 stdout 失败: %w", err)
	}

	// stderr 丢弃（或者可以记录日志）
	t.cmd.Stderr = io.Discard

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %w", err)
	}

	// 启动读取 goroutine
	go t.readLoop()

	return nil
}

// SendRequest 发送 JSON-RPC 请求并等待响应
func (t *StdioTransport) SendRequest(ctx context.Context, method string, params any) (any, error) {
	id := t.nextID.Add(1)

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// 注册 pending channel
	ch := make(chan *jsonrpcResponse, 1)
	t.pendMu.Lock()
	t.pending[id] = ch
	t.pendMu.Unlock()

	defer func() {
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
	}()

	// 发送请求
	if err := t.send(req); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp == nil {
			return nil, fmt.Errorf("连接已关闭")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		var result any
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

// SendNotification 发送 JSON-RPC 通知（不需要响应）
func (t *StdioTransport) SendNotification(method string, params any) error {
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return t.send(req)
}

// Close 关闭连接
func (t *StdioTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}
	t.closed = true

	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}

	// 关闭所有 pending channels
	t.pendMu.Lock()
	for id, ch := range t.pending {
		close(ch)
		delete(t.pending, id)
	}
	t.pendMu.Unlock()
}

// send 序列化并写入 stdin
func (t *StdioTransport) send(msg any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("连接已关闭")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// JSON-RPC over stdio: 每条消息一行
	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}

// readLoop 持续读取 stdout 中的 JSON-RPC 响应
func (t *StdioTransport) readLoop() {
	defer func() {
		// 防止向已关闭的 channel 发送导致 panic
		if r := recover(); r != nil {
			// 静默恢复，连接已关闭
		}
	}()

	scanner := bufio.NewScanner(t.stdout)
	// 允许单行最大 10MB
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp jsonrpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // 忽略无法解析的行
		}

		// 分发到 pending channel（非阻塞，防止 Close 竞争）
		if resp.ID > 0 {
			t.pendMu.Lock()
			ch, ok := t.pending[resp.ID]
			t.pendMu.Unlock()
			if ok {
				select {
				case ch <- &resp:
				default:
				}
			}
		}
		// 通知类消息（无 ID）暂不处理
	}
}
