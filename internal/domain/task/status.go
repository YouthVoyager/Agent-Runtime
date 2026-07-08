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
	StatusRetryableFailed Status = "RETRYABLE_FAILED"
	StatusStoppedByLimit  Status = "STOPPED_BY_LIMIT"
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
		StatusFailed,
		StatusRetryableFailed,
		StatusStoppedByLimit:
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

// CanResume 判断任务状态是否允许 resume,设计方案 §5.4 只允许失败、暂停和超限停止。
func CanResume(status Status) bool {
	switch status {
	case StatusFailed, StatusRetryableFailed, StatusPaused, StatusStoppedByLimit:
		return true
	default:
		return false
	}
}
