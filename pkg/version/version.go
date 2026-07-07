package version

// 这些变量由构建命令通过 -ldflags 注入；本地 go run 时使用默认值。
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)
