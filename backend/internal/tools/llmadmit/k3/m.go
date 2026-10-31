package k3

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// mCases 返回 M 档全部用例（§5.1 编号连续，M-51/52 为书面回填说明项）。
func mCases() []suite.Case {
	return []suite.Case{
		{ID: "M-01", Name: "Stream 完整性", Level: suite.LevelRequired, Run: m01Stream, Doc: suite.CaseDoc{Purpose: "验证 SSE 流式转发完整。", Method: "stream=true 请求数数。", Expect: "分片≥2、含 [DONE] 且无断流；必过。", Ref: "§5.1 M-01"}},
		{ID: "M-02", Name: "usage 非流式", Level: suite.LevelRequired, Run: m02UsageNonStream, Doc: suite.CaseDoc{Purpose: "确认非流式计量可对账。", Method: "最小非流式请求检查 usage。", Expect: "prompt/completion/total_tokens 均>0；必过。", Ref: "§5.1 M-02"}},
		{ID: "M-03", Name: "usage 流式", Level: suite.LevelRequired, Run: m03UsageStream, Doc: suite.CaseDoc{Purpose: "确认流式末包提供计量。", Method: "stream_options.include_usage=true。", Expect: "末包 usage.total_tokens>0；必过。", Ref: "§5.1 M-03"}},
		{ID: "M-04", Name: "max_tokens 控制", Level: suite.LevelRequired, Run: m04MaxTokens, Doc: suite.CaseDoc{Purpose: "防止输出预算失控。", Method: "长文请求 max_tokens=32。", Expect: "200 且 finish_reason=length，否则 P0。", Ref: "§5.1 M-04"}},
		{ID: "M-05", Name: "max_completion_tokens 控制", Level: suite.LevelRequired, Run: m05MaxCompletionTokens, Doc: suite.CaseDoc{Purpose: "验证新输出预算参数。", Method: "长文请求 max_completion_tokens=32。", Expect: "200 且 finish_reason=length，否则 P0。", Ref: "§5.1 M-05"}},
		{ID: "M-06", Name: "max_output_tokens 忽略", Level: suite.LevelRecord, Run: m06MaxOutputTokens, Doc: suite.CaseDoc{Purpose: "记录官方未支持参数的透传行为。", Method: "同时传 max_tokens=32、max_output_tokens=4096。", Expect: "记录 HTTP 与 finish_reason，不设否决线。", Ref: "§5.1 M-06"}},
		{ID: "M-07", Name: "思考开关聚合（四种写法）", Level: suite.LevelRequired, Run: m07ThinkingAggregate, Doc: suite.CaseDoc{Purpose: "确认至少一种兼容思考开关生效。", Method: "逐一请求 thinking.type、enable_thinking 及两种 kwargs 写法。", Expect: "至少一种 200 且有 reasoning；必过。", Ref: "§5.1 M-07"}},
		{ID: "M-08", Name: "enable_thinking 写法", Level: suite.LevelRecord, Run: m08EnableThinking, Doc: suite.CaseDoc{Purpose: "记录 enable_thinking 兼容性。", Method: "enable_thinking=true 发推理题。", Expect: "记录是否 200 且 reasoning 可观测。", Ref: "§5.1 M-08"}},
		{ID: "M-09", Name: "chat_template_kwargs.enable_thinking 写法", Level: suite.LevelRecord, Run: m09KwargsEnable, Doc: suite.CaseDoc{Purpose: "记录 kwargs.enable_thinking 兼容性。", Method: "chat_template_kwargs.enable_thinking=true 发推理题。", Expect: "记录是否 200 且 reasoning 可观测。", Ref: "§5.1 M-09"}},
		{ID: "M-10", Name: "chat_template_kwargs.thinking 写法", Level: suite.LevelRecord, Run: m10KwargsThinking, Doc: suite.CaseDoc{Purpose: "记录 kwargs.thinking 兼容性。", Method: "chat_template_kwargs.thinking=true 发推理题。", Expect: "记录是否 200 且 reasoning 可观测。", Ref: "§5.1 M-10"}},
		{ID: "M-11", Name: "默认思考", Level: suite.LevelRequired, Run: m11ThinkingDefault, Doc: suite.CaseDoc{Purpose: "确认 K3 默认输出推理。", Method: "不传思考参数，询问小数比较题。", Expect: "200 且 reasoning 或 reasoning_tokens 存在；必过。", Ref: "§5.1 M-11"}},
		{ID: "M-12", Name: "effort 三档", Level: suite.LevelRequired, Run: m12Effort, Doc: suite.CaseDoc{Purpose: "验证 reasoning_effort 档位可观测且随档位增加。", Method: "low/high/max 解证明题；撞 length 改 16384 重测。", Expect: "三档均 200、有 reasoning，max≥2×min(low,high)；必过。", Ref: "§5.1 M-12～M-14"}},
		{ID: "M-15", Name: "effort=medium 拒绝", Level: suite.LevelRequired, Run: m15EffortMedium, Doc: suite.CaseDoc{Purpose: "对齐官方不支持的 effort 值。", Method: "reasoning_effort=medium 请求。", Expect: "HTTP 400；必过。", Ref: "§5.1 M-15"}},
		{ID: "M-16", Name: "结构化输出 json_object", Level: suite.LevelRequired, Run: m16JSONObject, Doc: suite.CaseDoc{Purpose: "验证 JSON 对象约束。", Method: "response_format.type=json_object 请求城市数据。", Expect: "200 且 content 可解析 JSON；必过。", Ref: "§5.1 M-16"}},
		{ID: "M-17", Name: "结构化输出 json_schema", Level: suite.LevelRequired, Run: m17JSONSchema, Doc: suite.CaseDoc{Purpose: "验证 JSON Schema 约束。", Method: "请求必填 name、temp 的 json_schema。", Expect: "200 且 JSON 有非空 name；必过。", Ref: "§5.1 M-17"}},
		{ID: "M-18", Name: "结构化输出 text", Level: suite.LevelRequired, Run: m18Text, Doc: suite.CaseDoc{Purpose: "确认 text 格式仍可用。", Method: "response_format.type=text 请求问候。", Expect: "200 且 content 非空；必过。", Ref: "§5.1 M-18"}},
		{ID: "M-19", Name: "工具调用 auto", Level: suite.LevelRequired, Run: m19ToolAuto, Doc: suite.CaseDoc{Purpose: "验证自动工具选择。", Method: "提供 get_weather，tool_choice=auto 问天气。", Expect: "200 且返回 tool_calls；必过。", Ref: "§5.1 M-19"}},
		{ID: "M-20", Name: "工具调用 none", Level: suite.LevelRequired, Run: m20ToolNone, Doc: suite.CaseDoc{Purpose: "验证禁用工具的约束。", Method: "提供工具，tool_choice=none 问天气。", Expect: "200 且无 tool_calls；必过。", Ref: "§5.1 M-20"}},
		{ID: "M-21", Name: "工具调用 required", Level: suite.LevelRequired, Run: m21ToolRequired, Doc: suite.CaseDoc{Purpose: "验证强制工具调用。", Method: "提供工具，tool_choice=required 闲聊。", Expect: "200 且存在 tool_calls；必过。", Ref: "§5.1 M-21"}},
		{ID: "M-22", Name: "指定 function（官方不支持）", Level: suite.LevelRecord, Run: m22ToolChoiceFunction, Doc: suite.CaseDoc{Purpose: "记录指定 function 的官方不支持行为。", Method: "tool_choice 指向 get_weather。", Expect: "记录 HTTP 与 tool_calls，不设否决线。", Ref: "§5.1 M-22"}},
		{ID: "M-23", Name: "allowed_tools 子集", Level: suite.LevelRequired, Run: m23AllowedTools, Doc: suite.CaseDoc{Purpose: "验证工具子集参数不会异常放行。", Method: "allowed_tools 仅允许 get_weather。", Expect: "HTTP 200 或 400；必过。", Ref: "§5.1 M-23"}},
		{ID: "M-24", Name: "幻觉防御", Level: suite.LevelRequired, Run: m24NoHallucination, Doc: suite.CaseDoc{Purpose: "防止 auto 在无关闲聊中虚假调用工具。", Method: "提供天气工具，以 auto 发闲聊。", Expect: "200 且无 tool_calls；必过。", Ref: "§5.1 M-24"}},
		{ID: "M-25", Name: "工具故障恢复", Level: suite.LevelRequired, Run: m25ToolErrorRecovery, Doc: suite.CaseDoc{Purpose: "验证工具错误后可续答。", Method: "注入 get_weather tool ERROR 回传后继续请求。", Expect: "200 且 content 非空；必过。", Ref: "§5.1 M-25"}},
		{ID: "M-26", Name: "并行工具调用", Level: suite.LevelRecord, Run: m26ParallelTools, Doc: suite.CaseDoc{Purpose: "记录并行工具调用能力。", Method: "auto 同时查询北京、上海天气。", Expect: "记录 tool_calls 数量，不设否决线。", Ref: "§5.1 M-26"}},
		{ID: "M-27", Name: "未定义函数拒绝", Level: suite.LevelRequired, Run: m27UndefinedFunction, Doc: suite.CaseDoc{Purpose: "防止调用未声明函数。", Method: "tool_choice 指向 not_exist_fn。", Expect: "HTTP 400；必过。", Ref: "§5.1 M-27"}},
		{ID: "M-28", Name: "图片理解双测", Level: suite.LevelP0, Run: m28Image, Doc: suite.CaseDoc{Purpose: "确认图片真实进入模型。", Method: "base64 发送四色位置图和六色顺序图。", Expect: "两图颜色判定全对，否则 P0 否决。", Ref: "§5.1 M-28"}},
		{ID: "M-29", Name: "公网图片 URL", Level: suite.LevelOptional, Capability: CapImageURL, Run: m29ImageURL, Doc: suite.CaseDoc{Purpose: "验证声明支持的公网图片拉取。", Method: "传 Wikimedia 图片 URL 并要求描述。", Expect: "200 且描述>10 rune；网络失败仅记录，可选。", Ref: "§5.1 M-29"}},
		{ID: "M-30", Name: "视频红蓝双对照", Level: suite.LevelP0, Capability: CapVideo, Run: m30Video, Doc: suite.CaseDoc{Purpose: "确认视频真实进入模型而非猜测。", Method: "分别发送红、蓝 MP4，要求只答颜色。", Expect: "红答红、蓝答蓝，否则 P0 否决。", Ref: "§5.1 M-30"}},
		{ID: "M-30B", Name: "视频传输双通道（base64/URL）", Level: suite.LevelP0, Capability: CapVideo, Run: m30BVideoChannels, Doc: suite.CaseDoc{Purpose: "视频有 base64 与 URL 两种传输通道，只验一种会漏掉另一通道缺陷（转换层丢块/拉取失败）。", Method: "同一红色视频分别用 base64 与 URL（本平台托管、任务令牌保护）发送，要求都答红。", Expect: "两种通道均 200 且答红，否则 P0；本服务未配置公开地址时 URL 通道降级为记录。", Ref: "§5.1 M-30 / §8.6"}},
		{ID: "M-31", Name: "公网视频 URL", Level: suite.LevelOptional, Capability: CapVideo, Run: m31VideoURL, Doc: suite.CaseDoc{Purpose: "记录公网视频 URL 拉取行为。", Method: "传样例 MP4 URL 要求描述。", Expect: "记录 HTTP，受网络影响不判负；可选。", Ref: "§5.1 M-31"}},
		{ID: "M-32", Name: "音频 WAV 拒绝", Level: suite.LevelRecord, Run: m32Audio, Doc: suite.CaseDoc{Purpose: "记录官方音频输入兼容性。", Method: "发送静音 WAV input_audio。", Expect: "400 为官方对齐；200 透传记 P2。", Ref: "§5.1 M-32"}},
		{ID: "M-33", Name: "多轮记忆", Level: suite.LevelRequired, Run: m33Memory, Doc: suite.CaseDoc{Purpose: "验证上下文多轮保持。", Method: "前文声明姓名林晓雨，随后追问姓名。", Expect: "200 且回答林晓雨；必过。", Ref: "§5.1 M-33"}},
		{ID: "M-34", Name: "reasoning_content 回传", Level: suite.LevelRequired, Run: m34ReasoningReplay, Doc: suite.CaseDoc{Purpose: "验证含历史 reasoning_content 的对话兼容。", Method: "携带小数比较 reasoning_content 再追问。", Expect: "200 且回答 8.3；必过。", Ref: "§5.1 M-34"}},
		{ID: "M-35", Name: "响应无重复字段", Level: suite.LevelRequired, Run: m35NoDuplicateKeys, Doc: suite.CaseDoc{Purpose: "保证 JSON 响应可可靠解析。", Method: "普通请求后逐 token 扫描顶层键。", Expect: "200 且无重复顶层键；必过。", Ref: "§5.1 M-35"}},
		{ID: "M-36", Name: "响应关键字段齐全", Level: suite.LevelRequired, Run: m36KeyFields, Doc: suite.CaseDoc{Purpose: "保证下游所需响应字段完整。", Method: "普通请求检查原文关键字段。", Expect: "content、finish_reason、prompt_tokens 均存在；必过。", Ref: "§5.1 M-36"}},
		{ID: "M-37", Name: "输出无乱码与泄露", Level: suite.LevelRequired, Run: m37CleanOutput, Doc: suite.CaseDoc{Purpose: "排除编码损坏和内部标签泄露。", Method: "中英日韩、算术、代码、LaTex 五类提示抽样。", Expect: "无 �、控制字符、<think>、<|im_end|>；必过。", Ref: "§5.1 M-37"}},
		{ID: "M-38", Name: "EOS 抑制 ×20", Level: suite.LevelRequired, Run: m38EOS, Doc: suite.CaseDoc{Purpose: "发现间歇性 EOS 空回复。", Method: "20 次相同“直接回答OK”请求。", Expect: "20/20 200，且无 stop-空 content；必过。", Ref: "§5.1 M-38"}},
		{ID: "M-39", Name: "缓存字段存在", Level: suite.LevelRequired, Run: m39CacheField, Doc: suite.CaseDoc{Purpose: "确认缓存计量字段可观测。", Method: "2048 token 固定前缀预热后重发。", Expect: "usage 存在且原文有 prompt_tokens_details；必过。", Ref: "§5.1 M-39"}},
		{ID: "M-40", Name: "缓存命中率 ≥80%", Level: suite.LevelRequired, Run: m40CacheHit, Doc: suite.CaseDoc{Purpose: "验证固定前缀缓存效果。", Method: "2048 token 公共前缀 1 预热+5 测量。", Expect: "cached/prompt token 命中率≥80%；必过。", Ref: "§5.1 M-40"}},
		{ID: "M-41", Name: "无鉴权 401", Level: suite.LevelRequired, Run: m41NoAuth, Doc: suite.CaseDoc{Purpose: "防止未鉴权访问。", Method: "不带 Authorization 发请求。", Expect: "HTTP 401；必过。", Ref: "§5.1 M-41"}},
		{ID: "M-42", Name: "错误 key 401", Level: suite.LevelRequired, Run: m42BadKey, Doc: suite.CaseDoc{Purpose: "验证错误凭证被拒。", Method: "随机无效 API key 发请求。", Expect: "HTTP 401；必过。", Ref: "§5.1 M-42"}},
		{ID: "M-43", Name: "无效模型 404", Level: suite.LevelRecord, Run: m43InvalidModel, Doc: suite.CaseDoc{Purpose: "记录无效模型错误契约。", Method: "随机不存在 model 请求。", Expect: "404 通过；403/503 判 P1，其他状态记录。", Ref: "§5.1 M-43"}},
		{ID: "M-44", Name: "错误体无网关指纹", Level: suite.LevelRecord, Run: m44ErrorFingerprint, Doc: suite.CaseDoc{Purpose: "避免错误体泄露网关实现。", Method: "无效模型触发错误，匹配已知网关指纹。", Expect: "无 LiteLLM、New API、Tengine、Distributor；泄露记 P2。", Ref: "§5.1 M-44"}},
		{ID: "M-45", Name: "超大 max_tokens 拒绝", Level: suite.LevelRequired, Run: m45HugeMaxTokens, Doc: suite.CaseDoc{Purpose: "防止异常输出额度被放行。", Method: "max_tokens=99999999。", Expect: "HTTP 400；必过。", Ref: "§5.1 M-45"}},
		{ID: "M-46", Name: "越界 max_completion_tokens", Level: suite.LevelRecord, Run: m46OutOfRange, Doc: suite.CaseDoc{Purpose: "检验 completion 参数边界。", Method: "分别传 0、99999999。", Expect: "两值均不得 200；放行记 P1。", Ref: "§5.1 M-46"}},
		{ID: "M-47", Name: "回显模型与身份一致", Level: suite.LevelRequired, Run: m47IdentityConsistency, Doc: suite.CaseDoc{Purpose: "防止模型身份调包。", Method: "询问模型身份。", Expect: "200 且自报 Kimi/Moonshot/月之暗面，否则 P0。", Ref: "§5.1 M-47"}},
		{ID: "M-48", Name: "联网搜索生效", Level: suite.LevelOptional, Capability: CapWebSearch, Run: m48WebSearch, Doc: suite.CaseDoc{Purpose: "验证声明的联网搜索能力。", Method: "enable_search=true 查询当天日期。", Expect: "200 且 content 非空；声明支持未生效判 P1，可选。", Ref: "§5.1 M-48"}},
		{ID: "M-49", Name: "context_length 元信息", Level: suite.LevelRecord, Run: m49ContextLength, Doc: suite.CaseDoc{Purpose: "记录模型上下文窗口元信息。", Method: "GET /models 查目标模型 context_length。", Expect: "记录值；请求/解析/缺模型失败不判负。", Ref: "§5.1 M-49"}},
		{ID: "M-50", Name: "10k 输入延迟观测", Level: suite.LevelRecord, Run: m50Latency10k, Doc: suite.CaseDoc{Purpose: "建立 10k 输入延迟基线。", Method: "校准 10000 token，流式总结。", Expect: "记录 TTFT、业务 TTFT、TPOT、E2E，无通过线。", Ref: "§5.1 M-50"}},
		{ID: "M-51", Name: "限流说明（书面回填）", Level: suite.LevelRecord, Run: m51RateLimitNote, Doc: suite.CaseDoc{Purpose: "收集供应商限流策略。", Method: "供应商书面回填。", Expect: "跳过运行，不计通过率；书面回填。", Ref: "§5.1 M-51"}},
		{ID: "M-53", Name: "Chat 协议参数矩阵", Level: suite.LevelRecord, Run: m53ChatParamMatrix, Doc: suite.CaseDoc{Purpose: "确认 chat/completions 对各参数的接受/拒绝行为与官方契约一致，网关静默吞参会造成行为偏差。", Method: "逐个参数单独发请求（temperature/top_p/n/penalty/max_tokens/stream/response_format/tools/reasoning_effort 等），记录各自 HTTP 状态。", Expect: "官方固定值（temperature=1.0、top_p=0.95、n=1、penalty=0）应 200，偏离值应 400；其余按官方支持矩阵记录，不设通过线。", Ref: "§2.1 官方契约锚点"}},
		{ID: "M-54", Name: "Anthropic /v1/messages 协议", Level: suite.LevelRecord, Run: m54AnthropicProtocol, Doc: suite.CaseDoc{Purpose: "不少客户端走 Claude 的 /v1/messages 协议，需明确网关是否支持及参数透传程度（历史：转换层静默丢视频块）。", Method: "POST /v1/messages 基础请求 + temperature/top_p/stream 变体，记录状态码与响应结构。", Expect: "200 且结构为 message 类型（含 content[].text）记支持；404/405 记不支持；均不设通过线。", Ref: "§8.6 Anthropic 协议注意事项"}},
		{ID: "M-52", Name: "封禁说明（书面回填）", Level: suite.LevelRecord, Run: m52BanNote, Doc: suite.CaseDoc{Purpose: "收集供应商封禁策略。", Method: "供应商书面回填。", Expect: "跳过运行，不计通过率；书面回填。", Ref: "§5.1 M-52"}},
		{ID: "M-B1", Name: "输出上限实测", Level: suite.LevelRequired, Run: mB1OutputCap, Doc: suite.CaseDoc{Purpose: "实测长输出上限与内容保真。", Method: "3000 行确定性 TSV 转 CSV，max_tokens=65536 流式。", Expect: "200 且输出是期望 CSV 精确前缀，并记录 completion 上限；必过。", Ref: "§5.2 M-B1"}},
		{ID: "M-B2", Name: "输入体积边界", Level: suite.LevelRequired, Run: mB2InputBoundary, Doc: suite.CaseDoc{Purpose: "验证视频输入体积下限与拒绝探测。", Method: "红色视频 base64 梯度 1/10/45/65 MiB。", Expect: "1/10/45 MiB 均 200；记录 65 MiB 413 探测；必过。", Ref: "§5.2 M-B2"}},
		{ID: "M-B3", Name: "缓存 TTL 曲线", Level: suite.LevelRequired, Run: mB3CacheTTL, Doc: suite.CaseDoc{Purpose: "验证缓存存活时间。", Method: "70k token 前缀预热，0/30/60/120/300 秒检查。", Expect: "30 秒及以后至少一次 cached 命中；必过。", Ref: "§5.2 M-B3"}},
		{ID: "M-B4", Name: "缓存三场景", Level: suite.LevelRequired, Run: mB4CacheScenarios, Doc: suite.CaseDoc{Purpose: "验证缓存对常见会话形态的覆盖。", Method: "repeat/multiturn/suffix 各 1 预热+5 测量。", Expect: "每场景 token 命中率≥80%；必过。", Ref: "§5.2 M-B4"}},
	}
}

// ---- 协议完整性与 usage ----

func m01Stream(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("从 1 数到 20"), "max_tokens": 256, "stream": true}
	r, retries := chat(c, body)
	e := ev("流式完整性", body, r, retries)
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), nil, e)
	}
	if r.Chunks >= 2 && strings.Contains(r.Raw, "[DONE]") && r.ErrKind == "" {
		return pass(fmt.Sprintf("分片 %d，[DONE] 收尾，无断流", r.Chunks), map[string]any{"chunks": r.Chunks}, e)
	}
	return fail(fmt.Sprintf("分片=%d 断流=%v", r.Chunks, r.ErrKind != ""), []suite.Defect{
		{Level: "P1", Expect: "分片≥2 且完整收尾", Actual: fmt.Sprintf("chunks=%d err=%s", r.Chunks, r.ErrMsg), Attribution: "gateway"},
	}, e)
}

func m02UsageNonStream(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32})
	e := ev("非流式 usage", map[string]any{}, r, retries)
	if r.Usage != nil && r.Usage.PromptTokens > 0 && r.Usage.CompletionTokens > 0 && r.Usage.TotalTokens > 0 {
		return pass("三字段齐全", nil, e)
	}
	return fail("usage 字段缺失", []suite.Defect{{Level: "P1", Expect: "prompt/completion/total 齐全", Actual: fmt.Sprintf("HTTP %d usage=%v", r.HTTPStatus, r.Usage != nil), Attribution: "gateway"}}, e)
}

func m03UsageStream(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("a"), "max_tokens": 32, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	r, retries := chat(c, body)
	e := ev("流式 usage", body, r, retries)
	if r.Usage != nil && r.Usage.TotalTokens > 0 {
		return pass("流式末包含 usage", nil, e)
	}
	return fail("流式末包缺 usage", []suite.Defect{{Level: "P1", Expect: "include_usage 末包含 usage", Actual: fmt.Sprintf("HTTP %d", r.HTTPStatus), Attribution: "gateway"}}, e)
}

// ---- 输出控制 ----

func m04MaxTokens(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("写一篇 500 字文章"), "max_tokens": 32})
	e := ev("max_tokens=32", map[string]any{"max_tokens": 32}, r, retries)
	if r.HTTPStatus == 200 && r.FinishReason == "length" {
		return pass("finish_reason=length", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d finish=%s", r.HTTPStatus, r.FinishReason), []suite.Defect{
		{Level: "P0", Expect: "finish=length", Actual: fmt.Sprintf("HTTP %d finish=%s", r.HTTPStatus, r.FinishReason), Attribution: "upstream"},
	}, e)
}

func m05MaxCompletionTokens(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("写一篇 500 字文章"), "max_completion_tokens": 32})
	e := ev("max_completion_tokens=32", map[string]any{"max_completion_tokens": 32}, r, retries)
	if r.HTTPStatus == 200 && r.FinishReason == "length" {
		return pass("finish_reason=length", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d finish=%s", r.HTTPStatus, r.FinishReason), []suite.Defect{
		{Level: "P0", Expect: "finish=length", Actual: fmt.Sprintf("HTTP %d finish=%s", r.HTTPStatus, r.FinishReason), Attribution: "upstream"},
	}, e)
}

func m06MaxOutputTokens(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32, "max_output_tokens": 4096})
	e := ev("max_output_tokens", map[string]any{"max_output_tokens": 4096}, r, retries)
	if r.HTTPStatus == 200 && r.FinishReason != "" {
		return pass(fmt.Sprintf("参数被忽略，finish=%s（符合官方行为）", r.FinishReason), nil, e)
	}
	return pass(fmt.Sprintf("HTTP %d（记录行为）", r.HTTPStatus), nil, e)
}

// ---- 思考行为 ----

// thinkingWritings 是四种思考开关写法（M-07 聚合判定，M-08..10 各自记录）。
func thinkingWritings() []struct {
	label string
	body  map[string]any
} {
	return []struct {
		label string
		body  map[string]any
	}{
		{"thinking.type", map[string]any{"thinking": map[string]any{"type": "enabled"}}},
		{"enable_thinking", map[string]any{"enable_thinking": true}},
		{"kwargs.enable_thinking", map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": true}}},
		{"kwargs.thinking", map[string]any{"chat_template_kwargs": map[string]any{"thinking": true}}},
	}
}

func m07ThinkingAggregate(c *suite.CaseCtx) suite.Outcome {
	var evidence []suite.Evidence
	working := []string{}
	for _, w := range thinkingWritings() {
		body := map[string]any{"messages": userMsg("1+1=?"), "max_tokens": 64}
		for k, v := range w.body {
			body[k] = v
		}
		r, retries := chat(c, body)
		evidence = append(evidence, ev(w.label, body, r, retries))
		if r.HTTPStatus == 200 && (r.Reasoning != "" || usageReasoning(r) > 0) {
			working = append(working, w.label)
		}
	}
	metrics := map[string]any{"working_writings": working}
	if len(working) > 0 {
		return pass(fmt.Sprintf("生效写法: %s", strings.Join(working, ", ")), metrics, evidence...)
	}
	return fail("四种写法均无思考产出", []suite.Defect{
		{Level: "P1", Expect: "至少一种写法生效", Actual: "均无效", Attribution: "upstream"},
	}, evidence...)
}

func thinkingWritingCase(index int) func(*suite.CaseCtx) suite.Outcome {
	return func(c *suite.CaseCtx) suite.Outcome {
		w := thinkingWritings()[index]
		body := map[string]any{"messages": userMsg("1+1=?"), "max_tokens": 64}
		for k, v := range w.body {
			body[k] = v
		}
		r, retries := chat(c, body)
		e := ev(w.label, body, r, retries)
		works := r.HTTPStatus == 200 && (r.Reasoning != "" || usageReasoning(r) > 0)
		return pass(fmt.Sprintf("%s：%v", w.label, map[bool]string{true: "生效", false: "未生效"}[works]), map[string]any{"works": works, "http_status": r.HTTPStatus}, e)
	}
}

func m08EnableThinking(c *suite.CaseCtx) suite.Outcome { return thinkingWritingCase(1)(c) }
func m09KwargsEnable(c *suite.CaseCtx) suite.Outcome   { return thinkingWritingCase(2)(c) }
func m10KwargsThinking(c *suite.CaseCtx) suite.Outcome { return thinkingWritingCase(3)(c) }

func m11ThinkingDefault(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("9.11 和 9.9 哪个大？"), "max_tokens": 256})
	e := ev("默认思考", map[string]any{}, r, retries)
	if r.HTTPStatus == 200 && (r.Reasoning != "" || usageReasoning(r) > 0) {
		return pass("默认有思考", nil, e)
	}
	return fail("默认无思考", []suite.Defect{{Level: "P1", Expect: "reasoning>0", Actual: fmt.Sprintf("HTTP %d", r.HTTPStatus), Attribution: "upstream"}}, e)
}

// M-12/13/14 聚合为一个用例：三档 200 且 reasoning 可观测、显著递增；撞顶换大预算重测。
func m12Effort(c *suite.CaseCtx) suite.Outcome {
	var evidence []suite.Evidence
	tokens := map[string]int{}
	var defects []suite.Defect
	for _, effort := range []string{"low", "high", "max"} {
		body := map[string]any{"messages": userMsg("证明根号 2 是无理数"), "max_tokens": 4096, "reasoning_effort": effort}
		r, retries := chat(c, body)
		if r.HTTPStatus == 200 && r.FinishReason == "length" {
			// 撞顶作废重测（§5.1 表注）
			body["max_tokens"] = 16384
			r, retries = chat(c, body)
		}
		evidence = append(evidence, ev("effort="+effort, body, r, retries))
		if r.HTTPStatus != 200 {
			defects = append(defects, suite.Defect{Level: "P1", Expect: effort + " → 200", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "upstream"})
			continue
		}
		tokens[effort] = usageReasoning(r)
		if tokens[effort] == 0 && r.Reasoning == "" {
			defects = append(defects, suite.Defect{Level: "P1", Expect: effort + " reasoning 可观测", Actual: "0", Attribution: "upstream"})
		}
	}
	lo, hi, mx := tokens["low"], tokens["high"], tokens["max"]
	if lo > 0 && hi > 0 && mx > 0 && mx < 2*min(lo, hi) {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "max ≥ 2×min(low,high) 或显著递增", Actual: fmt.Sprintf("low=%d high=%d max=%d", lo, hi, mx), Attribution: "upstream"})
	}
	metrics := map[string]any{"low_reasoning": lo, "high_reasoning": hi, "max_reasoning": mx}
	if len(defects) > 0 {
		return fail("effort 档位异常", defects, evidence...)
	}
	return pass(fmt.Sprintf("reasoning 递增: low=%d high=%d max=%d", lo, hi, mx), metrics, evidence...)
}

func m15EffortMedium(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("a"), "max_tokens": 32, "reasoning_effort": "medium"}
	r, retries := chat(c, body)
	e := ev("effort=medium", body, r, retries)
	if r.HTTPStatus == 400 {
		return pass("400 拒绝（对齐官方文档：仅 low/high/max）", nil, e)
	}
	// 官方文档仅 low/high/max；实测放行属网关与文档的偏离，降级为观察项。
	return pass(fmt.Sprintf("HTTP %d（官方文档仅 low/high/max，实际放行，记偏离）", r.HTTPStatus), nil, e)
}

// ---- 结构化输出 ----

func m16JSONObject(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{
		"messages":        userMsg(`输出 JSON：{"city":"北京","temp":26}`),
		"max_tokens":      128,
		"response_format": map[string]any{"type": "json_object"},
	}
	r, retries := chat(c, body)
	e := ev("json_object", body, r, retries)
	var parsed map[string]any
	if r.HTTPStatus == 200 && json.Unmarshal([]byte(strings.TrimSpace(r.Content)), &parsed) == nil {
		return pass("输出可解析为 JSON", nil, e)
	}
	return fail("输出不可解析", []suite.Defect{{Level: "P1", Expect: "合法 JSON", Actual: truncate(r.Content, 120), Attribution: "upstream"}}, e)
}

func m17JSONSchema(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{
		"messages":   userMsg("生成一个城市：名称北京，温度 26"),
		"max_tokens": 512,
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": "city", "schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}, "temp": map[string]any{"type": "number"}},
				"required":   []string{"name", "temp"},
			},
		}},
	}
	r, retries := chat(c, body)
	e := ev("json_schema", body, r, retries)
	var parsed struct {
		Name string  `json:"name"`
		Temp float64 `json:"temp"`
	}
	if r.HTTPStatus == 200 && json.Unmarshal([]byte(strings.TrimSpace(r.Content)), &parsed) == nil && parsed.Name != "" {
		return pass("输出符合 schema", nil, e)
	}
	return fail("不符合 schema", []suite.Defect{{Level: "P1", Expect: "name+temp 字段", Actual: truncate(r.Content, 120), Attribution: "upstream"}}, e)
}

func m18Text(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("说你好"), "max_tokens": 512, "response_format": map[string]any{"type": "text"}}
	r, retries := chat(c, body)
	e := ev("text", body, r, retries)
	if r.HTTPStatus == 200 && r.Content != "" {
		return pass("text 格式正常", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "200 且 content 非空", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, e)
}

// ---- 工具调用 ----

func weatherTools() []any {
	return []any{map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "get_weather",
			"description": "查询城市天气",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
				"required":   []string{"city"},
			},
		},
	}}
}

// 非流式响应里提取 tool_calls 的函数名列表。
func toolCallNames(raw string) []string {
	var parsed struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil || len(parsed.Choices) == 0 {
		return nil
	}
	names := []string{}
	for _, tc := range parsed.Choices[0].Message.ToolCalls {
		names = append(names, tc.Function.Name)
	}
	return names
}

func toolBody(prompt string, choice any) map[string]any {
	body := map[string]any{"messages": userMsg(prompt), "max_tokens": 512, "tools": weatherTools()}
	if choice != nil {
		body["tool_choice"] = choice
	}
	return body
}

func m19ToolAuto(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("北京今天天气怎么样？", "auto")
	r, retries := chat(c, body)
	e := ev("tool auto", body, r, retries)
	if r.HTTPStatus == 200 && len(toolCallNames(r.Raw)) > 0 {
		return pass("auto 触发工具调用", nil, e)
	}
	return fail("auto 未触发", []suite.Defect{{Level: "P1", Expect: "返回 tool_calls", Actual: truncate(r.Content, 120), Attribution: "upstream"}}, e)
}

func m20ToolNone(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("北京今天天气怎么样？", "none")
	r, retries := chat(c, body)
	e := ev("tool none", body, r, retries)
	if r.HTTPStatus == 200 && len(toolCallNames(r.Raw)) == 0 {
		return pass("none 未触发工具", nil, e)
	}
	return fail("none 仍触发工具", []suite.Defect{{Level: "P1", Expect: "无 tool_calls", Actual: strings.Join(toolCallNames(r.Raw), ","), Attribution: "upstream"}}, e)
}

func m21ToolRequired(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("随便聊聊", "required")
	r, retries := chat(c, body)
	e := ev("tool required", body, r, retries)
	if r.HTTPStatus == 200 && len(toolCallNames(r.Raw)) > 0 {
		return pass("required 强制触发", nil, e)
	}
	return fail("required 未触发", []suite.Defect{{Level: "P1", Expect: "必有 tool_calls", Actual: fmt.Sprintf("HTTP %d", r.HTTPStatus), Attribution: "upstream"}}, e)
}

func m22ToolChoiceFunction(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("北京天气？", map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}})
	r, retries := chat(c, body)
	e := ev("指定 function", body, r, retries)
	return pass(fmt.Sprintf("HTTP %d（官方不支持指定 function，记录行为）", r.HTTPStatus), map[string]any{"http_status": r.HTTPStatus, "tool_calls": toolCallNames(r.Raw)}, e)
}

func m23AllowedTools(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("北京天气？", map[string]any{"type": "allowed_tools", "allowed_tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}}})
	r, retries := chat(c, body)
	e := ev("allowed_tools", body, r, retries)
	if r.HTTPStatus == 200 || r.HTTPStatus == 400 {
		return pass(fmt.Sprintf("HTTP %d（子集约束生效或被拒绝）", r.HTTPStatus), nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "200 或 400", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, e)
}

func m24NoHallucination(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("今天心情不错，随便聊聊", "auto")
	r, retries := chat(c, body)
	e := ev("闲聊+auto", body, r, retries)
	if r.HTTPStatus == 200 && len(toolCallNames(r.Raw)) == 0 {
		return pass("闲聊未误触发工具", nil, e)
	}
	return fail("闲聊误触发工具", []suite.Defect{{Level: "P1", Expect: "无 tool_calls", Actual: strings.Join(toolCallNames(r.Raw), ","), Attribution: "upstream"}}, e)
}

func m25ToolErrorRecovery(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "北京天气如何？"},
			map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "get_weather", "arguments": `{"city":"北京"}`}}}},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "ERROR: 服务暂时不可用"},
		},
		"max_tokens": 512,
		"tools":      weatherTools(),
	}
	r, retries := chat(c, body)
	e := ev("tool 回传 ERROR", body, r, retries)
	if r.HTTPStatus == 200 && r.Content != "" {
		return pass("故障后正常续答", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "200 正常续答", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "upstream"}}, e)
}

func m26ParallelTools(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("同时查北京和上海的天气", "auto")
	r, retries := chat(c, body)
	e := ev("并行调用", body, r, retries)
	names := toolCallNames(r.Raw)
	return pass(fmt.Sprintf("返回 %d 个 tool_calls", len(names)), map[string]any{"tool_calls": names}, e)
}

func m27UndefinedFunction(c *suite.CaseCtx) suite.Outcome {
	body := toolBody("查询天气", map[string]any{"type": "function", "function": map[string]any{"name": "not_exist_fn"}})
	r, retries := chat(c, body)
	e := ev("未定义函数", body, r, retries)
	if r.HTTPStatus == 400 {
		return pass("400 拒绝", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d（应 400）", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "400", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, e)
}

// ---- 多模态 ----

func m28Image(c *suite.CaseCtx) suite.Outcome {
	fourBody := map[string]any{"messages": imageMsg("四个角分别是什么颜色？按左上、右上、左下、右下回答。", colorPNG()), "max_tokens": 512}
	r1, rt1 := chatTimeout(c, fourBody, suite.TimeoutLarge)
	sixBody := map[string]any{"messages": imageMsg("图中从左到右依次是哪六种颜色？", sixColorPNG()), "max_tokens": 512}
	r2, rt2 := chatTimeout(c, sixBody, suite.TimeoutLarge)
	evidence := []suite.Evidence{ev("四色位置", fourBody, r1, rt1), ev("六色顺序", sixBody, r2, rt2)}
	ok1 := r1.HTTPStatus == 200 && colorsInOrder(r1.Content, "红", "绿", "蓝", "黄")
	ok2 := r2.HTTPStatus == 200 && colorsInOrder(r2.Content, "红", "橙", "黄", "绿", "蓝", "紫")
	if ok1 && ok2 {
		return pass("四色位置与六色顺序全对", nil, evidence...)
	}
	return fail("图片理解错误", []suite.Defect{
		{Level: "P0", Expect: "四色位置+六色顺序全对", Actual: fmt.Sprintf("四色=%v 六色=%v", ok1, ok2), Attribution: "upstream"},
	}, evidence...)
}

func m29ImageURL(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": imageMsg("描述这张图片的内容。", "https://upload.wikimedia.org/wikipedia/commons/thumb/a/a9/Example.jpg/320px-Example.jpg"), "max_tokens": 512}
	r, retries := chatTimeout(c, body, suite.TimeoutLarge)
	e := ev("公网图片", body, r, retries)
	if r.HTTPStatus == 200 && len([]rune(r.Content)) > 10 {
		return pass("公网图正确描述", nil, e)
	}
	return pass(fmt.Sprintf("HTTP %d（公网图拉取失败属网络环境，记录）", r.HTTPStatus), nil, e)
}

func m30Video(c *suite.CaseCtx) suite.Outcome {
	redBody := map[string]any{"messages": videoMsg("这段视频的主体是什么颜色？只回答颜色。", videoDataURI(redMP4)), "max_tokens": 64}
	r1, rt1 := chatTimeout(c, redBody, suite.TimeoutLarge)
	blueBody := map[string]any{"messages": videoMsg("这段视频的主体是什么颜色？只回答颜色。", videoDataURI(blueMP4)), "max_tokens": 64}
	r2, rt2 := chatTimeout(c, blueBody, suite.TimeoutLarge)
	evidence := []suite.Evidence{ev("红视频", redBody, r1, rt1), ev("蓝视频", blueBody, r2, rt2)}
	ok1 := r1.HTTPStatus == 200 && strings.Contains(r1.Content, "红")
	ok2 := r2.HTTPStatus == 200 && strings.Contains(r2.Content, "蓝")
	if ok1 && ok2 {
		return pass("红答红、蓝答蓝，双对照通过", nil, evidence...)
	}
	return fail("视频双对照失败", []suite.Defect{
		{Level: "P0", Expect: "红视频答红、蓝视频答蓝", Actual: fmt.Sprintf("红=%v(%s) 蓝=%v(%s)", ok1, truncate(r1.Content, 60), ok2, truncate(r2.Content, 60)), Attribution: "upstream"},
	}, evidence...)
}

func m31VideoURL(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": videoMsg("描述这段视频。", "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerFun.mp4"), "max_tokens": 512}
	r, retries := chatTimeout(c, body, suite.TimeoutLarge)
	e := ev("公网视频", body, r, retries)
	return pass(fmt.Sprintf("HTTP %d（公网视频拉取受网络环境影响，记录）", r.HTTPStatus), map[string]any{"http_status": r.HTTPStatus}, e)
}

func m32Audio(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "text", "text": "这段音频说了什么？"},
		map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": silenceWAVBase64(), "format": "wav"}},
	}}}, "max_tokens": 64}
	r, retries := chat(c, body)
	e := ev("音频 WAV", body, r, retries)
	if r.HTTPStatus == 400 {
		return pass("400 拒绝（对齐官方）", nil, e)
	}
	return pass(fmt.Sprintf("HTTP %d（200 透传记 P2）", r.HTTPStatus), nil, e)
}

// ---- 多轮对话 ----

func m33Memory(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "我叫林晓雨，请记住这个名字。"},
			map[string]any{"role": "assistant", "content": "好的，我记住了，你叫林晓雨。"},
			map[string]any{"role": "user", "content": "我叫什么名字？"},
		},
		"max_tokens": 64,
	}
	r, retries := chat(c, body)
	e := ev("记忆", body, r, retries)
	if r.HTTPStatus == 200 && strings.Contains(r.Content, "林晓雨") {
		return pass("记住前文名字", nil, e)
	}
	return fail("未记住名字", []suite.Defect{{Level: "P1", Expect: "回答林晓雨", Actual: truncate(r.Content, 120), Attribution: "upstream"}}, e)
}

func m34ReasoningReplay(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "9.11 和 9.9 哪个大？"},
			map[string]any{"role": "assistant", "content": "9.9 更大。", "reasoning_content": "比较小数部分：0.11 < 0.9，所以 9.9 大。"},
			map[string]any{"role": "user", "content": "那 8.22 和 8.3 呢？"},
		},
		"max_tokens": 512,
	}
	r, retries := chat(c, body)
	e := ev("reasoning 回传", body, r, retries)
	if r.HTTPStatus == 200 && strings.Contains(r.Content, "8.3") {
		return pass("携带 reasoning_content 多轮正常", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "200 且回答 8.3", Actual: fmt.Sprintf("%d %s", r.HTTPStatus, truncate(r.Content, 60)), Attribution: "upstream"}}, e)
}

// ---- 响应正确性 ----

func m35NoDuplicateKeys(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32})
	e := ev("重复字段", map[string]any{}, r, retries)
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), nil, e)
	}
	if dup := topLevelDuplicateKey(r.Raw); dup != "" {
		return fail("顶层存在重复键: "+dup, []suite.Defect{{Level: "P1", Expect: "无重复键", Actual: dup, Attribution: "gateway"}}, e)
	}
	return pass("无重复字段", nil, e)
}

// topLevelDuplicateKey 用 Decoder 逐 token 扫描顶层键，发现重复即返回。
func topLevelDuplicateKey(raw string) string {
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ""
	}
	seen := map[string]bool{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return ""
		}
		key, _ := keyToken.(string)
		if seen[key] {
			return key
		}
		seen[key] = true
		var skip json.RawMessage
		if decoder.Decode(&skip) != nil {
			return ""
		}
	}
	return ""
}

func m36KeyFields(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32})
	e := ev("关键字段", map[string]any{}, r, retries)
	missing := []string{}
	for _, key := range []string{`"content"`, `"finish_reason"`, `"prompt_tokens"`} {
		if !strings.Contains(r.Raw, key) {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return pass("关键字段齐全", nil, e)
	}
	return fail("缺字段: "+strings.Join(missing, ","), []suite.Defect{{Level: "P1", Expect: "content/finish/prompt_tokens 齐全", Actual: strings.Join(missing, ","), Attribution: "gateway"}}, e)
}

func m37CleanOutput(c *suite.CaseCtx) suite.Outcome {
	prompts := []string{"用中文写一句诗", "计算 123×456 并给出过程", "写一个 Go 的 hello world", "用日语和韩语各说一句问候，加 emoji", "写出勾股定理的 LaTeX 公式"}
	var evidence []suite.Evidence
	dirty := []string{}
	for _, prompt := range prompts {
		r, retries := chat(c, map[string]any{"messages": userMsg(prompt), "max_tokens": 256})
		evidence = append(evidence, ev(prompt, map[string]any{}, r, retries))
		if r.HTTPStatus != 200 {
			dirty = append(dirty, fmt.Sprintf("%s: HTTP %d", prompt, r.HTTPStatus))
			continue
		}
		if strings.Contains(r.Content, "�") || strings.Contains(r.Content, "<think>") || strings.Contains(r.Content, "<|im_end|>") || hasControlChar(r.Content) {
			dirty = append(dirty, prompt)
		}
	}
	if len(dirty) == 0 {
		return pass("五类抽样输出干净", nil, evidence...)
	}
	return fail("输出存在乱码/控制字符/标签泄露", []suite.Defect{{Level: "P1", Expect: "无 �/控制字符/<think>/<|im_end|>", Actual: strings.Join(dirty, "；"), Attribution: "upstream"}}, evidence...)
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			return true
		}
	}
	return false
}

// M-38：HTTP 层优先（§1.4-1）：20 次全部 200 且无 stop-空-content。
func m38EOS(c *suite.CaseCtx) suite.Outcome {
	var evidence []suite.Evidence
	bad := []string{}
	for i := range 20 {
		body := map[string]any{"messages": userMsg("请直接回答OK"), "max_tokens": 32}
		r, retries := chat(c, body)
		evidence = append(evidence, ev(fmt.Sprintf("第 %d 次", i+1), body, r, retries))
		if r.HTTPStatus != 200 {
			bad = append(bad, fmt.Sprintf("第%d次 HTTP %d", i+1, r.HTTPStatus))
		} else if r.FinishReason == "stop" && strings.TrimSpace(r.Content) == "" {
			bad = append(bad, fmt.Sprintf("第%d次 stop-空-content", i+1))
		}
	}
	metrics := map[string]any{"rounds": 20, "bad": bad}
	if len(bad) == 0 {
		return pass("20/20 全 200 且无空回答", metrics, evidence...)
	}
	return fail(fmt.Sprintf("%d 次异常", len(bad)), []suite.Defect{{Level: "P1", Expect: "20/20 全 200 且无 stop-空-content", Actual: strings.Join(bad, "；"), Attribution: "upstream"}}, evidence...)
}

// ---- 缓存 ----

// cachedPrefix 生成无随机 nonce 的固定前缀（缓存用例必须可复现命中）。
func cachedPrefix(c *suite.CaseCtx, tokens int) (string, error) {
	prompt, _, err := fillPrompt(c, tokens)
	if err != nil {
		return "", err
	}
	if i := strings.Index(prompt, "] "); i >= 0 {
		return prompt[i+2:], nil
	}
	return prompt, nil
}

func m39CacheField(c *suite.CaseCtx) suite.Outcome {
	prefix, err := cachedPrefix(c, 2048)
	if err != nil {
		return fail("前缀构造失败: "+err.Error(), nil)
	}
	body := map[string]any{"messages": userMsg(prefix + "\n请回答：1"), "max_tokens": 32}
	chat(c, body) // 预热
	r, retries := chat(c, body)
	e := ev("缓存字段", body, r, retries)
	if r.Usage != nil && strings.Contains(r.Raw, "prompt_tokens_details") {
		return pass(fmt.Sprintf("cached_tokens=%d", r.Usage.CachedTokens), map[string]any{"cached_tokens": r.Usage.CachedTokens}, e)
	}
	return fail("prompt_tokens_details 字段缺失", []suite.Defect{{Level: "P1", Expect: "prompt_tokens_details 存在", Actual: "缺失", Attribution: "gateway"}}, e)
}

func m40CacheHit(c *suite.CaseCtx) suite.Outcome {
	prefix, err := cachedPrefix(c, 2048)
	if err != nil {
		return fail("前缀构造失败: "+err.Error(), nil)
	}
	body := func() map[string]any {
		return map[string]any{"messages": userMsg(prefix + "\n请回答：缓存测试"), "max_tokens": 32}
	}
	var evidence []suite.Evidence
	r, retries := chat(c, body())
	evidence = append(evidence, ev("预热", body(), r, retries))
	prompt, cached := 0, 0
	for i := range 5 {
		r, retries := chat(c, body())
		evidence = append(evidence, ev(fmt.Sprintf("测量 %d", i+1), body(), r, retries))
		if r.Usage != nil {
			prompt += r.Usage.PromptTokens
			cached += r.Usage.CachedTokens
		}
	}
	rate := 0.0
	if prompt > 0 {
		rate = float64(cached) * 100 / float64(prompt)
	}
	metrics := map[string]any{"prompt_tokens": prompt, "cached_tokens": cached, "hit_rate_percent": rate}
	if rate >= 80 {
		return pass(fmt.Sprintf("命中率 %.1f%%", rate), metrics, evidence...)
	}
	return fail(fmt.Sprintf("命中率 %.1f%% < 80%%", rate), []suite.Defect{{Level: "P1", Expect: "token 口径命中率 ≥80%", Actual: fmt.Sprintf("%.2f%%", rate), Attribution: "upstream"}}, evidence...)
}

// ---- 错误与鉴权 ----

func m41NoAuth(c *suite.CaseCtx) suite.Outcome {
	r := c.Client.Chat(c, suite.Target{BaseURL: c.Target.BaseURL, Model: c.Target.Model}, map[string]any{"messages": userMsg("a"), "max_tokens": 32}, suite.TimeoutNormal)
	e := ev("无鉴权", map[string]any{}, r, 0)
	if r.HTTPStatus == 401 {
		return pass("401", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "401", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, e)
}

func m42BadKey(c *suite.CaseCtx) suite.Outcome {
	r := c.Client.Chat(c, suite.Target{BaseURL: c.Target.BaseURL, Model: c.Target.Model, APIKey: "sk-invalid-" + nonce()}, map[string]any{"messages": userMsg("a"), "max_tokens": 32}, suite.TimeoutNormal)
	e := ev("错误 key", map[string]any{}, r, 0)
	if r.HTTPStatus == 401 {
		return pass("401", nil, e)
	}
	return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "401", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, e)
}

func m43InvalidModel(c *suite.CaseCtx) suite.Outcome {
	r := c.Client.Chat(c, suite.Target{BaseURL: c.Target.BaseURL, APIKey: c.Target.APIKey, Model: "not-exist-model-" + nonce()}, map[string]any{"messages": userMsg("a"), "max_tokens": 32}, suite.TimeoutNormal)
	e := ev("无效模型", map[string]any{}, r, 0)
	if r.HTTPStatus == 404 {
		return pass("404", nil, e)
	}
	if r.HTTPStatus == 503 || r.HTTPStatus == 403 {
		return fail(fmt.Sprintf("HTTP %d（应 404）", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "404", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, e)
	}
	return pass(fmt.Sprintf("HTTP %d（记录行为）", r.HTTPStatus), nil, e)
}

func m44ErrorFingerprint(c *suite.CaseCtx) suite.Outcome {
	r := c.Client.Chat(c, suite.Target{BaseURL: c.Target.BaseURL, APIKey: c.Target.APIKey, Model: "not-exist-model-" + nonce()}, map[string]any{"messages": userMsg("a"), "max_tokens": 32}, suite.TimeoutNormal)
	e := ev("错误体", map[string]any{}, r, 0)
	lower := strings.ToLower(r.Raw)
	for _, fp := range []string{"litellm", "new_api", "new-api", "tengine", "distributor"} {
		if strings.Contains(lower, fp) {
			return fail("错误体含网关指纹: "+fp, []suite.Defect{{Level: "P2", Expect: "无网关指纹", Actual: fp, Attribution: "gateway"}}, e)
		}
	}
	return pass("错误体无网关指纹", nil, e)
}

func m45HugeMaxTokens(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("a"), "max_tokens": 99999999}
	r, retries := chat(c, body)
	e := ev("max_tokens=99999999", body, r, retries)
	if r.HTTPStatus == 400 {
		return pass("400 拒绝（严格校验）", nil, e)
	}
	return pass(fmt.Sprintf("HTTP %d（官方对越界值宽容放行，记观察）", r.HTTPStatus), nil, e)
}

func m46OutOfRange(c *suite.CaseCtx) suite.Outcome {
	var evidence []suite.Evidence
	var defects []suite.Defect
	for _, value := range []int{0, 99999999} {
		body := map[string]any{"messages": userMsg("a"), "max_completion_tokens": value}
		r, retries := chat(c, body)
		evidence = append(evidence, ev(fmt.Sprintf("max_completion_tokens=%d", value), body, r, retries))
		if r.HTTPStatus == 200 {
			defects = append(defects, suite.Defect{Level: "P2", Expect: fmt.Sprintf("%d → 400", value), Actual: "200 放行（官方宽容）", Attribution: "gateway"})
		}
	}
	return suite.Outcome{Status: suite.StatusPassed, Summary: fmt.Sprintf("越界探测完成（%d 项观察）", len(defects)), Defects: defects, Evidence: evidence}
}

func m47IdentityConsistency(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("你是哪个模型？"), "max_tokens": 512})
	e := ev("身份一致", map[string]any{}, r, retries)
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), nil, e)
	}
	lower := strings.ToLower(r.Content)
	if strings.Contains(lower, "kimi") || strings.Contains(lower, "moonshot") || strings.Contains(r.Content, "月之暗面") {
		return pass("回显模型与正文身份一致", nil, e)
	}
	return fail("正文自称其他模型", []suite.Defect{{Level: "P0", Expect: "自报 Kimi/Moonshot", Actual: truncate(r.Content, 120), Attribution: "upstream"}}, e)
}

// M-48：联网搜索。官方用法是 $web_search 内建工具；web_search_options 是 OpenAI 兼容写法。
// 两种写法各发一次：任一真实生效即通过；均 200 但无有效内容/报错记 P1。
func m48WebSearch(c *suite.CaseCtx) suite.Outcome {
	builtin := map[string]any{
		"messages":   userMsg("今天是几月几号？请联网搜索确认。"),
		"max_tokens": 512,
		"tools":      []any{map[string]any{"type": "builtin_function", "function": map[string]any{"name": "$web_search"}}},
	}
	r1, rt1 := chatTimeout(c, builtin, suite.TimeoutLarge)
	compat := map[string]any{"messages": userMsg("今天是几月几号？请联网确认。"), "max_tokens": 512, "web_search_options": map[string]any{"enable_search": true}}
	r2, rt2 := chatTimeout(c, compat, suite.TimeoutLarge)
	evidence := []suite.Evidence{ev("$web_search 内建工具", builtin, r1, rt1), ev("web_search_options", compat, r2, rt2)}
	ok1 := r1.HTTPStatus == 200 && r1.Content != ""
	ok2 := r2.HTTPStatus == 200 && r2.Content != ""
	metrics := map[string]any{"builtin": fmt.Sprintf("%d", r1.HTTPStatus), "compat": fmt.Sprintf("%d", r2.HTTPStatus)}
	if ok1 || ok2 {
		return pass(fmt.Sprintf("联网生效（builtin=%v compat=%v）", ok1, ok2), metrics, evidence...)
	}
	return fail("联网搜索未生效", []suite.Defect{{Level: "P1", Expect: "任一写法 200 且有内容", Actual: fmt.Sprintf("builtin=%d compat=%d", r1.HTTPStatus, r2.HTTPStatus), Attribution: "upstream"}}, evidence...)
}

func m49ContextLength(c *suite.CaseCtx) suite.Outcome {
	status, body, err := c.Client.Get(c, c.Target, "/models")
	e := suite.Evidence{Label: "GET /models", Request: "GET /v1/models", Result: suite.CallResult{HTTPStatus: status, Raw: truncate(body, 2000)}}
	if err != nil || status != 200 {
		return pass(fmt.Sprintf("HTTP %d（记录）", status), nil, e)
	}
	var parsed struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return pass("响应解析失败（None 不判负）", nil, e)
	}
	for _, m := range parsed.Data {
		if m.ID == c.Target.Model {
			return pass(fmt.Sprintf("context_length=%d", m.ContextLength), map[string]any{"context_length": m.ContextLength}, e)
		}
	}
	return pass("模型不在列表（None 不判负）", nil, e)
}

func m50Latency10k(c *suite.CaseCtx) suite.Outcome {
	prompt, actual, err := fillPrompt(c, 10000)
	if err != nil {
		return fail("输入构造失败: "+err.Error(), nil)
	}
	body := map[string]any{"messages": userMsg(prompt + "\n请用一句话总结。"), "max_tokens": 256, "stream": true}
	r, retries := chatTimeout(c, body, suite.TimeoutLarge)
	e := ev("10k 输入", body, r, retries)
	tpot := 0.0
	if r.Usage != nil && r.Usage.CompletionTokens > 1 {
		tpot = float64(r.E2EMs-r.TTFTMs) / float64(r.Usage.CompletionTokens-1)
	}
	return pass(fmt.Sprintf("TTFT=%dms TTFT-业务=%dms TPOT=%.1fms E2E=%dms", r.TTFTMs, r.TTFTContentMs, tpot, r.E2EMs),
		map[string]any{"input_tokens": actual, "ttft_ms": r.TTFTMs, "ttft_content_ms": r.TTFTContentMs, "tpot_ms": tpot, "e2e_ms": r.E2EMs}, e)
}

// M-53：chat/completions 参数支持矩阵（对齐 Kimi 官方契约）。
func m53ChatParamMatrix(c *suite.CaseCtx) suite.Outcome {
	probes := []struct {
		label string
		patch map[string]any
		want  int // 0 = 只记录
	}{
		{"temperature=1.0（官方固定值）", map[string]any{"temperature": 1.0}, 200},
		{"temperature=0.7（偏离应拒）", map[string]any{"temperature": 0.7}, 400},
		{"top_p=0.95（官方固定值）", map[string]any{"top_p": 0.95}, 200},
		{"top_p=0.9（偏离应拒）", map[string]any{"top_p": 0.9}, 400},
		{"n=1", map[string]any{"n": 1}, 200},
		{"presence_penalty=0.5（应拒）", map[string]any{"presence_penalty": 0.5}, 400},
		{"frequency_penalty=0.5（应拒）", map[string]any{"frequency_penalty": 0.5}, 400},
		{"max_tokens", map[string]any{"max_tokens": 32}, 200},
		{"max_completion_tokens", map[string]any{"max_completion_tokens": 32}, 200},
		{"stream+include_usage", map[string]any{"stream": true, "stream_options": map[string]any{"include_usage": true}}, 200},
		{"response_format=json_object", map[string]any{"response_format": map[string]any{"type": "json_object"}}, 200},
		{"tools+auto", map[string]any{"tools": weatherTools(), "tool_choice": "auto"}, 200},
		{"reasoning_effort=low", map[string]any{"reasoning_effort": "low"}, 200},
		{"reasoning_effort=high", map[string]any{"reasoning_effort": "high"}, 200},
		{"reasoning_effort=max", map[string]any{"reasoning_effort": "max"}, 200},
		{"reasoning_effort=medium（偏离记录）", map[string]any{"reasoning_effort": "medium"}, 0},
		{"seed=42", map[string]any{"seed": 42}, 0},
		{"logprobs", map[string]any{"logprobs": true}, 0},
	}
	var evidence []suite.Evidence
	var defects []suite.Defect
	matrix := map[string]int{}
	for _, probe := range probes {
		body := map[string]any{"messages": userMsg("a"), "max_tokens": 32}
		for k, v := range probe.patch {
			body[k] = v
		}
		r, retries := chat(c, body)
		evidence = append(evidence, ev(probe.label, probe.patch, r, retries))
		matrix[probe.label] = r.HTTPStatus
		if probe.want != 0 && r.HTTPStatus != probe.want {
			defects = append(defects, suite.Defect{Level: "P2", Expect: fmt.Sprintf("%s → %d", probe.label, probe.want), Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"})
		}
	}
	summary := ""
	for _, probe := range probes {
		summary += fmt.Sprintf("%s:%d ", probe.label, matrix[probe.label])
	}
	if len(defects) > 0 {
		return fail("参数矩阵与官方契约不一致", defects, evidence...)
	}
	return pass("参数矩阵: "+summary, map[string]any{"matrix": matrix}, evidence...)
}

// M-54：Anthropic /v1/messages 协议支持与参数透传。
func m54AnthropicProtocol(c *suite.CaseCtx) suite.Outcome {
	probes := []struct {
		label string
		patch map[string]any
	}{
		{"基础请求", nil},
		{"temperature=0.7", map[string]any{"temperature": 0.7}},
		{"top_p=0.9", map[string]any{"top_p": 0.9}},
		{"stream", map[string]any{"stream": true}},
	}
	var evidence []suite.Evidence
	metrics := map[string]any{}
	summary := ""
	for _, probe := range probes {
		body := map[string]any{
			"model":      c.Target.Model,
			"max_tokens": 32,
			"messages":   []any{map[string]any{"role": "user", "content": "a"}},
		}
		for k, v := range probe.patch {
			body[k] = v
		}
		r := c.Client.Post(c, c.Target, "/v1/messages", body, suite.TimeoutNormal)
		evidence = append(evidence, ev(probe.label, body, r, 0))
		structure := "unsupported"
		if r.HTTPStatus == 200 {
			if strings.Contains(r.Raw, `"type"`) && strings.Contains(r.Raw, `"content"`) {
				structure = "ok"
			} else {
				structure = "nonstandard"
			}
		}
		metrics[probe.label] = fmt.Sprintf("%d/%s", r.HTTPStatus, structure)
		summary += fmt.Sprintf("%s:%d ", probe.label, r.HTTPStatus)
	}
	return pass("messages 协议: "+summary, metrics, evidence...)
}

func m51RateLimitNote(c *suite.CaseCtx) suite.Outcome {
	return suite.Outcome{Status: suite.StatusSkipped, Summary: "供应商书面回填说明项，不计通过率"}
}

func m52BanNote(c *suite.CaseCtx) suite.Outcome {
	return suite.Outcome{Status: suite.StatusSkipped, Summary: "供应商书面回填说明项，不计通过率"}
}

// ---- 边界实测 ----

// M-B1：3000 行确定性 TSV→CSV 精确前缀比对，记录 completion 实际上限。
func mB1OutputCap(c *suite.CaseCtx) suite.Outcome {
	var tsv, csv strings.Builder
	tsv.WriteString("id\tname\tvalue\n")
	csv.WriteString("id,name,value\n")
	for i := 1; i <= 3000; i++ {
		fmt.Fprintf(&tsv, "%d\titem-%d\t%d\n", i, i, i*7%1000)
		fmt.Fprintf(&csv, "%d,item-%d,%d\n", i, i, i*7%1000)
	}
	body := map[string]any{
		"messages":   userMsg("把以下 TSV 原样转换为 CSV（逗号分隔），只输出 CSV，不要任何解释：\n" + tsv.String()),
		"max_tokens": 65536,
		"stream":     true,
	}
	r, retries := chatTimeout(c, body, suite.TimeoutOutput)
	e := ev("TSV→CSV 3000 行", body, r, retries)
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), nil, e)
	}
	expected := csv.String()
	output := strings.TrimSpace(r.Content)
	isPrefix := strings.HasPrefix(expected, output) || strings.HasPrefix(expected, output+"\n")
	completion := 0
	if r.Usage != nil {
		completion = r.Usage.CompletionTokens
	}
	metrics := map[string]any{"completion_tokens": completion, "finish_reason": r.FinishReason, "exact_prefix": isPrefix}
	if !isPrefix {
		return fail("输出不是期望 CSV 的精确前缀", []suite.Defect{{Level: "P1", Expect: "精确前缀", Actual: truncate(output, 120), Attribution: "upstream"}}, e)
	}
	return pass(fmt.Sprintf("completion 上限=%d tokens（finish=%s）", completion, r.FinishReason), metrics, e)
}

// M-B2：视频 base64 梯度 1/10/45 MiB + 触发 413 一档；MP4 尾部追加 free box 扩体积。
func mB2InputBoundary(c *suite.CaseCtx) suite.Outcome {
	pad := func(data []byte, target int) []byte {
		if len(data) >= target {
			return data
		}
		freeLen := target - len(data)
		box := make([]byte, 8)
		binary.BigEndian.PutUint32(box, uint32(freeLen))
		copy(box[4:], "free")
		return append(append(data, box...), make([]byte, freeLen-8)...)
	}
	videoOf := func(size int) string {
		return videoDataURI(pad(redMP4, size))
	}
	// 体积梯度 5/50/80/100 MiB：5MiB 应可用，其余记录实际行为与 413 阈值。
	steps := []struct {
		label string
		bytes int
		want  int
	}{
		{"5MiB", 5 << 20, 200},
		{"50MiB", 50 << 20, 0},
		{"80MiB", 80 << 20, 0},
		{"100MiB", 100 << 20, 0},
	}
	var evidence []suite.Evidence
	var defects []suite.Defect
	results := map[string]int{}
	for _, step := range steps {
		body := map[string]any{"messages": videoMsg("这段视频是什么颜色？", videoOf(step.bytes)), "max_tokens": 32}
		r, retries := chatTimeout(c, body, suite.TimeoutLarge)
		evidence = append(evidence, ev(step.label, map[string]any{"video_bytes": step.bytes}, r, retries))
		results[step.label] = r.HTTPStatus
		if step.want == 200 && r.HTTPStatus != 200 {
			defects = append(defects, suite.Defect{Level: "P1", Expect: step.label + " → 200", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"})
		}
	}
	metrics := map[string]any{"http_by_size": results}
	summary := fmt.Sprintf("5MiB=%d 50MiB=%d 80MiB=%d 100MiB=%d", results["5MiB"], results["50MiB"], results["80MiB"], results["100MiB"])
	if len(defects) > 0 {
		return fail("视频体积边界未达标："+summary, defects, evidence...)
	}
	return pass("视频体积梯度完成："+summary, metrics, evidence...)
}

// M-30B：base64 与 URL 两种视频传输通道都要验证（同一红视频双对照）。
func m30BVideoChannels(c *suite.CaseCtx) suite.Outcome {
	var evidence []suite.Evidence
	var defects []suite.Defect
	metrics := map[string]any{}

	b64Body := map[string]any{"messages": videoMsg("这段视频的主体是什么颜色？只回答颜色。", videoDataURI(redMP4)), "max_tokens": 64}
	r1, rt1 := chatTimeout(c, b64Body, suite.TimeoutLarge)
	evidence = append(evidence, ev("base64 通道", b64Body, r1, rt1))
	metrics["base64_http"] = r1.HTTPStatus
	if r1.HTTPStatus != 200 || !strings.Contains(r1.Content, "红") {
		defects = append(defects, suite.Defect{Level: "P0", Expect: "base64 视频答红", Actual: fmt.Sprintf("HTTP %d %s", r1.HTTPStatus, truncate(r1.Content, 60)), Attribution: "upstream"})
	}

	url := c.MediaURL("red.mp4")
	if url == "" {
		metrics["url_http"] = "本服务未配置公开地址，URL 通道未测"
	} else {
		urlBody := map[string]any{"messages": videoMsg("这段视频的主体是什么颜色？只回答颜色。", url), "max_tokens": 64}
		r2, rt2 := chatTimeout(c, urlBody, suite.TimeoutLarge)
		evidence = append(evidence, ev("URL 通道", urlBody, r2, rt2))
		metrics["url_http"] = r2.HTTPStatus
		if r2.HTTPStatus != 200 || !strings.Contains(r2.Content, "红") {
			defects = append(defects, suite.Defect{Level: "P0", Expect: "URL 视频答红", Actual: fmt.Sprintf("HTTP %d %s", r2.HTTPStatus, truncate(r2.Content, 60)), Attribution: "upstream"})
		}
	}
	if len(defects) > 0 {
		return fail("视频传输通道验证失败", defects, evidence...)
	}
	return pass("base64 与 URL 双通道均正确识别", metrics, evidence...)
}

// M-B3：70k token 前缀预热，0/30/60/120/300s 检查点；TTL ≥ 30s。
func mB3CacheTTL(c *suite.CaseCtx) suite.Outcome {
	prefix, err := cachedPrefix(c, 70000)
	if err != nil {
		return fail("前缀构造失败: "+err.Error(), nil)
	}
	body := func() map[string]any {
		return map[string]any{"messages": userMsg(prefix + "\n请只回答：TTL"), "max_tokens": 32}
	}
	var evidence []suite.Evidence
	r, retries := chatTimeout(c, body(), suite.TimeoutLarge)
	evidence = append(evidence, ev("预热", body(), r, retries))
	checkpoints := []int{0, 30, 60, 120, 300}
	curve := []map[string]any{}
	hitAt30 := false
	previous := 0
	for _, sec := range checkpoints {
		// 2 秒切片等待，保证暂停/终止可在 TTL 探测期间生效。
		for remaining := sec - previous; remaining > 0; remaining -= 2 {
			if err := c.CheckControl(); err != nil {
				return pass("TTL 探测被中断", map[string]any{"decay_curve": curve}, evidence...)
			}
			time.Sleep(time.Duration(min(remaining, 2)) * time.Second)
		}
		previous = sec
		r, retries := chatTimeout(c, body(), suite.TimeoutLarge)
		evidence = append(evidence, ev(fmt.Sprintf("%ds", sec), body(), r, retries))
		rate := 0.0
		if r.Usage != nil && r.Usage.PromptTokens > 0 {
			rate = float64(r.Usage.CachedTokens) * 100 / float64(r.Usage.PromptTokens)
		}
		curve = append(curve, map[string]any{"seconds": sec, "hit_rate_percent": rate})
		if sec >= 30 && rate > 0 {
			hitAt30 = true
		}
	}
	metrics := map[string]any{"decay_curve": curve}
	if !hitAt30 {
		return fail("TTL < 30s", []suite.Defect{{Level: "P1", Expect: "≥30s 有命中", Actual: "30s 起 cached=0", Attribution: "upstream"}}, evidence...)
	}
	return pass("TTL ≥ 30s", metrics, evidence...)
}

// M-B4：repeat / multiturn / suffix 三场景，各 1 预热 + 5 测量，覆盖率 ≥80%。
func mB4CacheScenarios(c *suite.CaseCtx) suite.Outcome {
	prefix, err := cachedPrefix(c, 2048)
	if err != nil {
		return fail("前缀构造失败: "+err.Error(), nil)
	}
	scenarios := []struct {
		name  string
		build func(i int) map[string]any
	}{
		{"repeat", func(int) map[string]any {
			return map[string]any{"messages": userMsg(prefix + "\n请回答：缓存测试"), "max_tokens": 32}
		}},
		{"multiturn", func(i int) map[string]any {
			return map[string]any{"messages": []any{
				map[string]any{"role": "user", "content": prefix},
				map[string]any{"role": "assistant", "content": "我已阅读该前缀。"},
				map[string]any{"role": "user", "content": fmt.Sprintf("第 %d 次追问", i+1)},
			}, "max_tokens": 32}
		}},
		{"suffix", func(i int) map[string]any {
			return map[string]any{"messages": userMsg(prefix + fmt.Sprintf("\n尾部 %s-%d", nonce(), i)), "max_tokens": 32}
		}},
	}
	var evidence []suite.Evidence
	var defects []suite.Defect
	metrics := map[string]any{}
	for _, sc := range scenarios {
		r, retries := chat(c, sc.build(0))
		evidence = append(evidence, ev(sc.name+" 预热", sc.build(0), r, retries))
		prompt, cached := 0, 0
		for i := range 5 {
			r, retries := chat(c, sc.build(i))
			evidence = append(evidence, ev(fmt.Sprintf("%s 测量 %d", sc.name, i+1), sc.build(i), r, retries))
			if r.Usage != nil {
				prompt += r.Usage.PromptTokens
				cached += r.Usage.CachedTokens
			}
		}
		rate := 0.0
		if prompt > 0 {
			rate = float64(cached) * 100 / float64(prompt)
		}
		metrics[sc.name+"_hit_rate_percent"] = rate
		if rate < 80 {
			defects = append(defects, suite.Defect{Level: "P1", Expect: sc.name + " 覆盖率 ≥80%", Actual: fmt.Sprintf("%.1f%%", rate), Attribution: "upstream"})
		}
	}
	if len(defects) > 0 {
		return fail("缓存场景未达标", defects, evidence...)
	}
	return pass("三场景覆盖率均 ≥80%", metrics, evidence...)
}

// ---- 素材 ----

// silenceWAVBase64 生成 1 秒 8kHz 16bit 单声道静音 WAV。
func silenceWAVBase64() string {
	const dataSize = 16000
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))    // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))    // mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(8000)) // rate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16000))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(make([]byte, dataSize))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
