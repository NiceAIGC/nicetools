import { useEffect, useMemo, useRef, useState } from "react";
import {
  Accordion,
  AccordionItem,
  addToast,
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  Code,
  Divider,
  Input,
  NumberInput,
  Select,
  SelectItem,
  Switch,
  Tab,
  Tabs,
  Textarea,
} from "@heroui/react";

import { copyText } from "../../utils/copyText";
import {
  buildCurl,
  buildRequest,
  fetchModelList,
  maskSecret,
  prettyJson,
  resolveEndpoint,
  runConnectivityTest,
  type ApiFormat,
  type TargetConfig,
  type TestResult,
} from "./request";
import {
  clearLocalRecords,
  loadDraft,
  loadProfiles,
  profileName,
  promptPresets,
  providerPresets,
  storeDraft,
  storeProfiles,
  type SavedProfile,
} from "./providers";

type ResultTab = "text" | "raw" | "request";

const formatOptions: { key: ApiFormat; label: string }[] = [
  { key: "openai", label: "OpenAI 格式" },
  { key: "anthropic", label: "Claude 格式" },
];

function createId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export default function LlmConnectivity() {
  const [config, setConfig] = useState<TargetConfig>(loadDraft);
  const [profiles, setProfiles] = useState<SavedProfile[]>(loadProfiles);
  const [result, setResult] = useState<TestResult | null>(null);
  const [liveText, setLiveText] = useState("");
  const [running, setRunning] = useState(false);
  const [resultTab, setResultTab] = useState<ResultTab>("text");
  const [showKey, setShowKey] = useState(false);
  const [models, setModels] = useState<string[]>([]);
  const [fetchingModels, setFetchingModels] = useState(false);

  const abortRef = useRef<AbortController | null>(null);
  const preset = providerPresets.find((item) => item.baseUrl && item.baseUrl === config.baseUrl);
  const curl = useMemo(() => buildCurl(config), [config]);
  const requestPreview = useMemo(() => {
    const built = buildRequest(config);
    const headerLines = Object.entries(built.headers).map(([name, value]) => {
      const lower = name.toLowerCase();
      if (lower === "authorization") return `${name}: Bearer ${maskSecret(value.replace(/^Bearer /i, ""))}`;
      if (lower === "x-api-key") return `${name}: ${maskSecret(value)}`;
      return `${name}: ${value}`;
    });
    return {
      url: built.url,
      headers: headerLines.join("\n"),
      body: built.body ? prettyJson(built.body) : "{}",
      error: built.error,
    };
  }, [config]);
  const endpoint = resolveEndpoint(config.format, config.baseUrl);
  const savedModels = useMemo(() => {
    const set = new Set<string>();
    for (const item of models) set.add(item);
    for (const item of profiles) if (item.model) set.add(item.model);
    if (config.model) set.add(config.model);
    return [...set].slice(0, 24);
  }, [models, profiles, config.model]);

  useEffect(() => {
    storeDraft(config);
  }, [config]);

  function update(patch: Partial<TargetConfig>) {
    setConfig((previous) => ({ ...previous, ...patch }));
  }

  function changeFormat(format: ApiFormat) {
    // Claude 原生格式必须带 max_tokens
    update({ format, maxTokens: format === "anthropic" ? (config.maxTokens ?? 64) : config.maxTokens });
  }

  function applyPreset(key: string) {
    const target = providerPresets.find((item) => item.key === key);
    if (!target) return;
    update({
      format: target.format,
      baseUrl: target.baseUrl,
      model: target.model,
      extraHeaders: target.extraHeaders,
      maxTokens: target.format === "anthropic" ? (config.maxTokens ?? 64) : config.maxTokens,
    });
    addToast({ title: `已套用参考配置：${target.label}`, description: target.note, timeout: 6000 });
  }

  async function test() {
    if (running) return;
    const controller = new AbortController();
    abortRef.current = controller;
    setRunning(true);
    setLiveText("");
    setResult(null);
    setResultTab("text");

    try {
      const outcome = await runConnectivityTest(config, {
        signal: controller.signal,
        onText: (text) => setLiveText(text),
      });
      setResult(outcome);
      if (outcome.ok) {
        addToast({ title: `连通正常，耗时 ${outcome.latencyMs} ms`, color: "success" });
      } else {
        addToast({
          title: outcome.networkError ? "请求未送达服务端" : "测试未通过",
          description: outcome.error ?? undefined,
          color: "danger",
          timeout: 8000,
        });
      }
    } finally {
      abortRef.current = null;
      setRunning(false);
    }
  }

  function stop() {
    abortRef.current?.abort();
  }

  async function copyCurl() {
    const copied = await copyText(curl);
    addToast({ title: copied ? "curl 已复制" : "复制失败", color: copied ? "success" : "danger" });
  }

  function copyResultText() {
    return copyText(resultTab === "request" ? prettyJson(result?.body ?? "") : (result?.text ?? ""));
  }

  function saveProfile() {
    if (!config.baseUrl.trim()) {
      addToast({ title: "请先填写 Base URL", color: "warning" });
      return;
    }
    const entry: SavedProfile = {
      id: createId(),
      name: profileName(config),
      format: config.format,
      baseUrl: config.baseUrl.trim(),
      apiKey: config.apiKey.trim(),
      model: config.model.trim(),
      savedAt: Date.now(),
    };
    const next = [
      entry,
      ...profiles.filter(
        (item) =>
          !(item.baseUrl === entry.baseUrl && item.model === entry.model && item.format === entry.format),
      ),
    ].slice(0, 30);
    setProfiles(next);
    storeProfiles(next);
    addToast({ title: "已保存到本地记录", description: "Base URL / Key / 模型名均已记录，下次可直接载入。" });
  }

  function applyProfile(id: string) {
    const target = profiles.find((item) => item.id === id);
    if (!target) return;
    update({
      format: target.format,
      baseUrl: target.baseUrl,
      apiKey: target.apiKey,
      model: target.model,
      maxTokens: target.format === "anthropic" ? (config.maxTokens ?? 64) : config.maxTokens,
    });
    addToast({ title: `已载入配置：${target.name}` });
  }

  function deleteProfile(id: string) {
    const next = profiles.filter((item) => item.id !== id);
    setProfiles(next);
    storeProfiles(next);
    addToast({ title: "已删除该配置" });
  }

  function clearAll() {
    clearLocalRecords();
    setProfiles([]);
    setModels([]);
    setConfig(loadDraft());
    addToast({ title: "已清空本地记录", color: "warning" });
  }

  async function loadModels() {
    setFetchingModels(true);
    try {
      const outcome = await fetchModelList(config);
      if (outcome.error) {
        addToast({ title: "拉取模型列表失败", description: outcome.error, color: "danger", timeout: 8000 });
        return;
      }
      setModels(outcome.models);
      addToast({ title: `已拉取 ${outcome.models.length} 个模型`, color: "success" });
    } finally {
      setFetchingModels(false);
    }
  }

  const metrics: { label: string; value: string }[] = [];
  if (result) {
    metrics.push({ label: "耗时", value: `${result.latencyMs} ms` });
    if (result.firstTokenMs !== null) metrics.push({ label: "首字", value: `${result.firstTokenMs} ms` });
    if (config.stream) metrics.push({ label: "分片", value: String(result.chunks) });
    if (result.text) metrics.push({ label: "输出", value: `${result.text.length} 字` });
    const { inputTokens, outputTokens, totalTokens } = result.usage ?? {
      inputTokens: null,
      outputTokens: null,
      totalTokens: null,
    };
    if (inputTokens !== null) metrics.push({ label: "输入 tokens", value: String(inputTokens) });
    if (outputTokens !== null) metrics.push({ label: "输出 tokens", value: String(outputTokens) });
    if (totalTokens !== null) metrics.push({ label: "合计 tokens", value: String(totalTokens) });
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardBody className="gap-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <Tabs
              aria-label="接口格式"
              variant="solid"
              color="primary"
              size="md"
              classNames={{ panel: "hidden" }}
              selectedKey={config.format}
              onSelectionChange={(key) => changeFormat(key as ApiFormat)}
            >
              {/* 仅作为格式切换器，内容全部在下方表单里，隐藏 Tabs 自带的空面板 */}
              {formatOptions.map((option) => (
                <Tab key={option.key} title={option.label} />
              ))}
            </Tabs>
            <div className="flex flex-wrap items-center gap-2">
              <Chip size="sm" variant="flat">
                {config.format === "openai" ? "POST /chat/completions" : "POST /messages"}
              </Chip>
              <Chip size="sm" variant="flat">
                {config.format === "openai" ? "Authorization: Bearer" : "x-api-key + anthropic-version"}
              </Chip>
            </div>
          </div>

          <Select
            aria-label="参考配置"
            label="参考配置"
            labelPlacement="outside"
            className="max-w-md"
            disallowEmptySelection
            selectedKeys={[preset?.key ?? "custom"]}
            onSelectionChange={(keys) => {
              const [key] = Array.from(keys as Iterable<string>);
              if (key) applyPreset(key);
            }}
          >
            {providerPresets.map((item) => (
              <SelectItem key={item.key} textValue={item.label}>
                {item.label}
              </SelectItem>
            ))}
          </Select>
          {preset?.note ? (
            <p className="-mt-3 text-xs text-default-500">
              <span className="font-medium text-default-600">{preset.label}：</span>
              {preset.note}
            </p>
          ) : null}

          <div className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2">
            <Input
              aria-label="Base URL"
              label="Base URL"
              labelPlacement="outside"
              placeholder="https://api.openai.com/v1"
              className="md:col-span-2"
              value={config.baseUrl}
              spellCheck="false"
              onValueChange={(value) => update({ baseUrl: value })}
              description={
                endpoint ? `实际请求地址：${endpoint}` : "填入服务商 Base URL，工具会自动补全终点路径"
              }
            />
            <Input
              aria-label="API Key"
              label="API Key"
              labelPlacement="outside"
              placeholder="sk-...（本地服务可留空）"
              type={showKey ? "text" : "password"}
              value={config.apiKey}
              spellCheck="false"
              autoComplete="off"
              onValueChange={(value) => update({ apiKey: value })}
              endContent={
                <Button size="sm" variant="light" onPress={() => setShowKey((value) => !value)}>
                  {showKey ? "隐藏" : "显示"}
                </Button>
              }
            />
            <Input
              aria-label="模型名称"
              label="模型名称"
              labelPlacement="outside"
              placeholder="gpt-4o-mini / claude-3-5-haiku-latest"
              value={config.model}
              spellCheck="false"
              onValueChange={(value) => update({ model: value })}
            />
            <Input
              aria-label="测试提示词"
              label="测试提示词"
              labelPlacement="outside"
              className="md:col-span-2"
              value={config.prompt}
              spellCheck="false"
              onValueChange={(value) => update({ prompt: value })}
            />
          </div>

          <div className="flex flex-wrap gap-2">
            {promptPresets.map((item) => (
              <Button
                key={item.label}
                size="sm"
                variant={config.prompt === item.text ? "solid" : "flat"}
                color={config.prompt === item.text ? "primary" : "default"}
                onPress={() => update({ prompt: item.text })}
              >
                {item.label}
              </Button>
            ))}
          </div>

          {savedModels.length > 0 ? (
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-xs text-default-500">已记录 / 拉取到的模型：</span>
                <Button
                  size="sm"
                  variant="flat"
                  isLoading={fetchingModels}
                  onPress={loadModels}
                  isDisabled={!config.baseUrl.trim()}
                >
                  拉取模型列表
                </Button>
              </div>
              <div className="flex max-h-24 flex-wrap gap-2 overflow-y-auto">
                {savedModels.map((name) => (
                  <Button
                    key={name}
                    size="sm"
                    variant={name === config.model ? "solid" : "bordered"}
                    color={name === config.model ? "primary" : "default"}
                    onPress={() => update({ model: name })}
                  >
                    {name}
                  </Button>
                ))}
              </div>
            </div>
          ) : (
            <div>
              <Button
                size="sm"
                variant="flat"
                isLoading={fetchingModels}
                onPress={loadModels}
                isDisabled={!config.baseUrl.trim()}
              >
                拉取模型列表
              </Button>
            </div>
          )}

          <div className="flex flex-wrap items-center gap-6">
            <Switch
              aria-label="流式输出"
              isSelected={config.stream}
              onValueChange={(value) => update({ stream: value })}
            >
              <span className="text-small">流式输出（stream）</span>
            </Switch>
            <NumberInput
              aria-label="超时秒数"
              label="超时（秒）"
              labelPlacement="outside"
              className="w-[140px]"
              minValue={1}
              maxValue={300}
              value={Math.round(config.timeoutMs / 1000)}
              onValueChange={(value) => {
                if (!Number.isFinite(value)) return;
                update({ timeoutMs: Math.min(300, Math.max(1, value)) * 1000 });
              }}
            />
          </div>

          <Accordion variant="splitted" className="px-0">
            <AccordionItem
              key="advanced"
              aria-label="高级选项"
              title="高级选项"
              subtitle="最大输出 tokens、追加请求头、Claude 鉴权方式"
            >
              <div className="flex flex-col gap-4">
                <div className="flex flex-wrap items-center gap-6">
                  <Switch
                    aria-label="发送 max_tokens"
                    isSelected={config.maxTokens !== null}
                    isDisabled={config.format === "anthropic"}
                    onValueChange={(value) => update({ maxTokens: value ? 64 : null })}
                  >
                    <span className="text-small">发送 max_tokens</span>
                  </Switch>
                  {config.maxTokens !== null ? (
                    <NumberInput
                      aria-label="max_tokens"
                      label="max_tokens"
                      labelPlacement="outside"
                      className="w-[160px]"
                      minValue={1}
                      maxValue={100000}
                      value={config.maxTokens}
                      onValueChange={(value) => {
                        if (!Number.isFinite(value)) return;
                        update({ maxTokens: Math.max(1, value) });
                      }}
                    />
                  ) : null}
                  {config.format === "anthropic" ? (
                    <Switch
                      aria-label="改用 Bearer 鉴权"
                      isSelected={config.bearerForAnthropic}
                      onValueChange={(value) => update({ bearerForAnthropic: value })}
                    >
                      <span className="text-small">改用 Authorization: Bearer</span>
                    </Switch>
                  ) : null}
                </div>

                <Textarea
                  aria-label="追加请求头"
                  label="追加请求头（JSON）"
                  labelPlacement="outside"
                  minRows={2}
                  placeholder='{"X-Title": "NiceTools"}'
                  value={config.extraHeaders}
                  spellCheck="false"
                  onValueChange={(value) => update({ extraHeaders: value })}
                />
              </div>
            </AccordionItem>
          </Accordion>

          <div className="flex flex-wrap items-center gap-3">
            <Button color="primary" isLoading={running} onPress={test}>
              开始测试
            </Button>
            <Button variant="flat" color="danger" onPress={stop} isDisabled={!running}>
              停止
            </Button>
            <Button variant="flat" onPress={saveProfile}>
              保存配置
            </Button>
            <Button variant="flat" onPress={copyCurl}>
              复制 curl
            </Button>
          </div>
        </CardBody>
      </Card>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="flex items-center justify-between gap-3">
          <div>
            <p className="font-semibold text-foreground">curl 复测命令</p>
            <p className="text-xs text-default-500">
              浏览器可能被 CORS 拦截，复制到终端执行可确认真实链路{config.stream ? "（-N 关闭缓冲以观察流式输出）" : ""}
            </p>
          </div>
          <Button size="sm" variant="flat" onPress={copyCurl}>
            复制
          </Button>
        </CardHeader>
        <CardBody className="pt-0">
          <pre className="max-h-64 min-w-0 overflow-auto rounded-large bg-default-100 p-3 text-xs leading-6 text-foreground">
            {curl}
          </pre>
        </CardBody>
      </Card>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="flex flex-col items-start gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <p className="font-semibold text-foreground">测试结果</p>
            {running ? <Chip color="primary" variant="flat" size="sm">测试中…</Chip> : null}
            {result ? (
              <Chip size="sm" variant="flat" color={result.ok ? "success" : "danger"}>
                {result.status === null ? "无响应" : `HTTP ${result.status}`}
              </Chip>
            ) : null}
            {metrics.map((item) => (
              <Chip key={item.label} size="sm" variant="bordered">
                {item.label} {item.value}
              </Chip>
            ))}
          </div>
          {result?.error ? (
            <Alert color={result.ok ? "warning" : "danger"} title={result.error} className="w-full" />
          ) : null}
        </CardHeader>
        <CardBody className="gap-3 pt-0">
          <Tabs
            aria-label="结果视图"
            selectedKey={resultTab}
            onSelectionChange={(key) => setResultTab(key as ResultTab)}
          >
            <Tab
              key="text"
              title="输出文本"
              className="flex flex-col gap-2"
            >
              <pre className="max-h-96 min-w-0 overflow-auto whitespace-pre-wrap rounded-large bg-default-100 p-3 text-xs leading-6 text-foreground">
                {running ? liveText || "等待首个分片…" : result?.text || "暂无内容"}
              </pre>
            </Tab>
            <Tab key="raw" title="原始响应" className="flex flex-col gap-2">
              <pre className="max-h-96 min-w-0 overflow-auto rounded-large bg-default-100 p-3 text-xs leading-6 text-foreground">
                {result?.raw ? prettyJson(result.raw) : "暂无内容"}
              </pre>
            </Tab>
            <Tab key="request" title="请求详情" className="flex flex-col gap-2">
              <div className="flex flex-col gap-2 text-xs">
                <div className="flex flex-wrap items-center gap-2">
                  <Code size="sm">POST</Code>
                  <span className="break-all text-default-600">{result?.url || endpoint || "—"}</span>
                </div>
                <Divider />
                <pre className="max-h-96 min-w-0 overflow-auto whitespace-pre-wrap rounded-large bg-default-100 p-3 leading-6">
                  {[requestPreview.headers, "", requestPreview.body].join("\n")}
                </pre>
              </div>
            </Tab>
          </Tabs>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              size="sm"
              variant="flat"
              isDisabled={!result}
              onPress={() => {
                void copyResultText().then((copied) =>
                  addToast({ title: copied ? "已复制" : "复制失败", color: copied ? "success" : "danger" }),
                );
              }}
            >
              复制当前面板
            </Button>
            {result?.headers["content-type"] ? (
              <span className="text-xs text-default-400">content-type: {result.headers["content-type"]}</span>
            ) : null}
          </div>
        </CardBody>
      </Card>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="flex items-center justify-between gap-3">
          <div>
            <p className="font-semibold text-foreground">本地记录</p>
            <p className="text-xs text-default-500">
              保存后 Base URL / API Key / 模型名都会写入浏览器 localStorage，仅保存在本机（不会上传）；公共电脑请及时清空。
            </p>
          </div>
          <span className="shrink-0 text-xs text-default-400">{profiles.length} 条</span>
        </CardHeader>
        <CardBody className="gap-3 pt-0">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-default-500">
              点击「保存配置」记录当前 Base URL / Key / 模型名；「载入」后直接开始测试。
            </p>
            <Button
              size="sm"
              variant="flat"
              color="danger"
              onPress={clearAll}
              isDisabled={profiles.length === 0}
            >
              清空全部记录
            </Button>
          </div>

          {profiles.length === 0 ? (
            <p className="rounded-large border border-dashed border-default-300 p-3 text-xs text-default-400">
              暂无记录。填好参数后点击「保存配置」，下次打开本页会自动恢复上次填写的内容。
            </p>
          ) : (
            <ul className="flex flex-col gap-2">
              {profiles.map((item) => (
                <li
                  key={item.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-large border border-default-200 bg-default-50 p-3"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-foreground">{item.name}</p>
                    <p className="truncate text-xs text-default-500">
                      {item.format === "anthropic" ? "Claude 格式" : "OpenAI 格式"} · {item.baseUrl}
                      {item.model ? ` · 模型 ${item.model}` : ""}
                      {item.apiKey ? ` · Key ${maskSecret(item.apiKey)}` : " · 无 Key"}
                    </p>
                  </div>
                  <div className="flex shrink-0 gap-2">
                    <Button size="sm" variant="flat" color="primary" onPress={() => applyProfile(item.id)}>
                      载入
                    </Button>
                    <Button size="sm" variant="flat" color="danger" onPress={() => deleteProfile(item.id)}>
                      删除
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardBody>
      </Card>
    </div>
  );
}
