package llmgateway

// PromptTemplate 描述一个受版本管理的 prompt。
type PromptTemplate struct {
	Version string
	System  string
}

// promptRegistry 保存全部 prompt 版本,新版本追加不覆盖,便于审计和回滚。
var promptRegistry = map[string]PromptTemplate{
	"local-v1": {
		Version: "local-v1",
		System:  "你是 StableAgent 执行器,按任务目标生成确定性的多 step 工具调用计划。",
	},
}

// activePromptVersion 指定当前生效的 prompt 版本。
const activePromptVersion = "local-v1"

// ActivePrompt 返回当前生效的 prompt 模板。
func ActivePrompt() PromptTemplate {
	return promptRegistry[activePromptVersion]
}

// PromptByVersion 按版本查找 prompt,用于回放历史请求。
func PromptByVersion(version string) (PromptTemplate, bool) {
	prompt, ok := promptRegistry[version]
	return prompt, ok
}
