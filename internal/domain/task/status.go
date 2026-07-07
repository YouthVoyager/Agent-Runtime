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

func StatusForAPI(status string) string {
	parsed, ok := ParseStatus(status)
	if !ok {
		return strings.ToUpper(strings.TrimSpace(status))
	}
	return string(parsed)
}
