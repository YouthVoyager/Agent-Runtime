package task

import "strings"

type Status string

const (
	StatusQueued          Status = "QUEUED"
	StatusRunning         Status = "RUNNING"
	StatusWaitingApproval Status = "WAITING_APPROVAL"
	StatusPaused          Status = "PAUSED"
	StatusCanceling       Status = "CANCELING"
	StatusCanceled        Status = "CANCELED"
	StatusSucceeded       Status = "SUCCEEDED"
	StatusFailed          Status = "FAILED"
)

// ParseStatus 标准化并校验任务状态字符串。
func ParseStatus(raw string) (Status, bool) {
	status := Status(strings.ToUpper(strings.TrimSpace(raw)))
	switch status {
	case StatusQueued,
		StatusRunning,
		StatusWaitingApproval,
		StatusPaused,
		StatusCanceling,
		StatusCanceled,
		StatusSucceeded,
		StatusFailed:
		return status, true
	default:
		return "", false
	}
}

// StatusForAPI 返回面向 API 的任务状态文本，未知状态会保留标准化后的原值。
func StatusForAPI(status string) string {
	parsed, ok := ParseStatus(status)
	if !ok {
		return strings.ToUpper(strings.TrimSpace(status))
	}
	return string(parsed)
}
