package context

import (
	"fmt"
	"os/exec"
	"strings"
)

// maxGitStatusLen Git 状态最大字符数（CC: 2000）
const maxGitStatusLen = 2000

// IsGitRepo 快速检测是否在 git 仓库中（不执行多余命令）
func IsGitRepo(workDir string) bool {
	_, err := gitExec(workDir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// GetGitStatus 采集 git 状态，格式对标 CC context.ts 的 getGitStatus()
// 返回空字符串表示不在 git 仓库中
func GetGitStatus(workDir string) string {
	if !IsGitRepo(workDir) {
		return ""
	}

	branch, _ := gitExec(workDir, "branch", "--show-current")
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "(detached HEAD)"
	}

	// 检测主分支名称
	mainBranch := detectMainBranch(workDir)

	// git user
	userName, _ := gitExec(workDir, "config", "user.name")
	userName = strings.TrimSpace(userName)

	// git status --short（--no-optional-locks 防止 index.lock 冲突）
	status, _ := gitExec(workDir, "--no-optional-locks", "status", "--short")
	status = strings.TrimSpace(status)
	if status == "" {
		status = "(clean)"
	}
	// 截断过长的 status（只截断 status 部分，保留其他字段完整）
	if len(status) > 500 {
		lines := strings.Split(status, "\n")
		if len(lines) > 15 {
			status = strings.Join(lines[:15], "\n") + fmt.Sprintf("\n... and %d more files", len(lines)-15)
		}
	}

	// 最近 5 条提交（CC: -n 5）
	log, _ := gitExec(workDir, "log", "--oneline", "-5")
	log = strings.TrimSpace(log)

	// CC 格式组装
	var sb strings.Builder
	sb.WriteString("This is the git status at the start of the conversation. ")
	sb.WriteString("Note that this status is a snapshot in time, and will not update during the conversation.\n\n")
	sb.WriteString(fmt.Sprintf("Current branch: %s\n", branch))
	sb.WriteString(fmt.Sprintf("Main branch (you will usually use this for PRs): %s\n", mainBranch))
	if userName != "" {
		sb.WriteString(fmt.Sprintf("Git user: %s\n", userName))
	}
	sb.WriteString(fmt.Sprintf("Status:\n%s\n\n", status))
	sb.WriteString(fmt.Sprintf("Recent commits:\n%s", log))

	result := sb.String()
	if len(result) > maxGitStatusLen {
		result = result[:maxGitStatusLen]
	}
	return result
}

// detectMainBranch 检测主分支名称（main 或 master）
func detectMainBranch(workDir string) string {
	if _, err := gitExec(workDir, "rev-parse", "--verify", "main"); err == nil {
		return "main"
	}
	if _, err := gitExec(workDir, "rev-parse", "--verify", "master"); err == nil {
		return "master"
	}
	branch, _ := gitExec(workDir, "branch", "--show-current")
	return strings.TrimSpace(branch)
}

// gitExec 在指定目录执行 git 命令
func gitExec(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	return string(out), err
}
