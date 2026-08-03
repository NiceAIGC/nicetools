import { useMemo, useState } from "react";
import {
  Accordion,
  AccordionItem,
  Alert,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  Chip,
  Input,
  Progress,
  Select,
  SelectItem,
} from "@heroui/react";
import {
  buildSystem,
  buildUrl,
  delay,
  sendRequest,
  type ProbeConfig,
  type RoundResult,
} from "./probe";

const defaults: ProbeConfig = {
  apiFormat: "anthropic",
  apiKey: "",
  baseUrl: "",
  model: "claude-opus-4-7",
  rounds: 5,
  waitSeconds: 3,
  concurrency: 10,
  timeoutSeconds: 60,
};

function asPositiveNumber(value: string, fallback: number): number {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : fallback;
}

export default function PromptCacheProbe() {
  const [config, setConfig] = useState(defaults);
  const [rounds, setRounds] = useState<RoundResult[]>([]);
  const [status, setStatus] = useState("等待开始");
  const [error, setError] = useState("");
  const [isRunning, setIsRunning] = useState(false);

  const total = useMemo(
    () => rounds.reduce((sum, round) => sum + round.results.length, 0),
    [rounds],
  );
  const hits = useMemo(
    () => rounds.reduce((sum, round) => sum + round.results.filter((result) => result.ok && result.read > 0).length, 0),
    [rounds],
  );
  const errors = useMemo(
    () => rounds.reduce((sum, round) => sum + round.results.filter((result) => !result.ok).length, 0),
    [rounds],
  );
  const hitRate = total ? (hits / total) * 100 : 0;

  function update<K extends keyof ProbeConfig>(key: K, value: ProbeConfig[K]) {
    setConfig((current) => ({ ...current, [key]: value }));
  }

  async function runProbe() {
    const apiKey = config.apiKey.trim();
    const baseUrl = config.baseUrl.trim();
    const model = config.model.trim();
    if (!apiKey || !baseUrl || !model) {
      setError("请填写 API Key、上游地址和模型名称。");
      return;
    }

    const normalized = {
      ...config,
      apiKey,
      baseUrl,
      model,
      rounds: asPositiveNumber(String(config.rounds), defaults.rounds),
      waitSeconds: Math.max(0, Number(config.waitSeconds) || 0),
      concurrency: Math.min(50, Math.floor(asPositiveNumber(String(config.concurrency), defaults.concurrency))),
      timeoutSeconds: asPositiveNumber(String(config.timeoutSeconds), defaults.timeoutSeconds),
    };
    const url = buildUrl(normalized.baseUrl, normalized.apiFormat);
    setConfig(normalized);
    setRounds([]);
    setError("");
    setIsRunning(true);

    try {
      for (let index = 1; index <= normalized.rounds; index += 1) {
        setStatus(`第 ${index}/${normalized.rounds} 轮：正在预热`);
        const system = buildSystem(`r${Date.now()}-${index}`);
        const warmup = await sendRequest(
          url,
          normalized.apiFormat,
          normalized.apiKey,
          normalized.model,
          system,
          "warmup",
          normalized.timeoutSeconds,
        );

        if (normalized.waitSeconds > 0) {
          setStatus(`第 ${index}/${normalized.rounds} 轮：等待 ${normalized.waitSeconds} 秒`);
          await delay(normalized.waitSeconds * 1000);
        }

        setStatus(`第 ${index}/${normalized.rounds} 轮：发送 ${normalized.concurrency} 个并发请求`);
        const results = await Promise.all(
          Array.from({ length: normalized.concurrency }, (_, requestIndex) =>
            sendRequest(
              url,
              normalized.apiFormat,
              normalized.apiKey,
              normalized.model,
              system,
              `q${requestIndex}`,
              normalized.timeoutSeconds,
            ),
          ),
        );
        setRounds((current) => [...current, { index, warmup, results }]);
      }
      setStatus("测试完成");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "测试过程中发生未知错误。");
      setStatus("测试失败");
    } finally {
      setIsRunning(false);
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2">
        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex items-center justify-between gap-3">
            <div>
              <p className="font-semibold text-foreground">测试配置</p>
              <p className="text-xs text-default-500">请求直接从浏览器发往上游 API</p>
            </div>
            <Chip color={error ? "danger" : isRunning ? "primary" : rounds.length ? "success" : "default"} variant="flat" size="sm" aria-live="polite">
              {status}
            </Chip>
          </CardHeader>
          <CardBody className="gap-4">
            <Select label="API 格式" labelPlacement="outside" selectedKeys={[config.apiFormat]} onSelectionChange={(keys) => update("apiFormat", Array.from(keys)[0] as ProbeConfig["apiFormat"])} variant="bordered">
              <SelectItem key="anthropic">Anthropic Messages API</SelectItem>
              <SelectItem key="openai">OpenAI Chat Completions API</SelectItem>
            </Select>
            <Input label="API Key" labelPlacement="outside" type="password" autoComplete="off" value={config.apiKey} onValueChange={(value) => update("apiKey", value)} variant="bordered" />
            <Input label="上游 API 地址" labelPlacement="outside" placeholder="https://xxxxxx.com" description={config.apiFormat === "anthropic" ? "自动补全为 /v1/messages" : "自动补全为 /v1/chat/completions"} value={config.baseUrl} onValueChange={(value) => update("baseUrl", value)} variant="bordered" />
            <Input label="模型" labelPlacement="outside" placeholder="例如：claude-opus-4-7 或 gpt-4o" value={config.model} onValueChange={(value) => update("model", value)} variant="bordered" />
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <Input label="轮数" labelPlacement="outside" type="number" min={1} value={String(config.rounds)} onValueChange={(value) => update("rounds", asPositiveNumber(value, defaults.rounds))} variant="bordered" />
              <Input label="等待秒数" labelPlacement="outside" type="number" min={0} step="0.5" value={String(config.waitSeconds)} onValueChange={(value) => update("waitSeconds", Math.max(0, Number(value) || 0))} variant="bordered" />
              <Input label="并发数" labelPlacement="outside" type="number" min={1} max={50} value={String(config.concurrency)} onValueChange={(value) => update("concurrency", Math.min(50, Math.floor(asPositiveNumber(value, defaults.concurrency))))} variant="bordered" />
              <Input label="超时秒数" labelPlacement="outside" type="number" min={1} value={String(config.timeoutSeconds)} onValueChange={(value) => update("timeoutSeconds", asPositiveNumber(value, defaults.timeoutSeconds))} variant="bordered" />
            </div>
          </CardBody>
          <CardFooter>
            <Button color="primary" className="w-full" isLoading={isRunning} onPress={runProbe}>
              {isRunning ? "测试进行中" : "开始缓存测试"}
            </Button>
          </CardFooter>
        </Card>

        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="font-semibold text-foreground">测试结果</p>
              <p className="text-xs text-default-500">命中读取代表该请求复用了 prompt cache</p>
            </div>
            {total > 0 && <Chip color={hitRate >= 80 ? "success" : hitRate > 0 ? "warning" : "danger"} variant="flat">总命中率 {hitRate.toFixed(0)}%</Chip>}
          </CardHeader>
          <CardBody className="gap-4">
            {total > 0 ? <>
              <Progress aria-label="缓存命中率" value={hitRate} color={hitRate >= 80 ? "success" : hitRate > 0 ? "warning" : "danger"} />
              <div className="grid grid-cols-3 gap-2 text-center text-sm">
                <div className="rounded-medium bg-default-100 p-3"><p className="text-lg font-semibold">{hits}/{total}</p><p className="text-default-500">命中读取</p></div>
                <div className="rounded-medium bg-default-100 p-3"><p className="text-lg font-semibold">{total - hits - errors}</p><p className="text-default-500">重新写入</p></div>
                <div className="rounded-medium bg-default-100 p-3"><p className="text-lg font-semibold">{errors}</p><p className="text-default-500">错误</p></div>
              </div>
              <div className="max-h-64 space-y-2 overflow-y-auto">
                {rounds.map((round) => {
                  const roundHits = round.results.filter((result) => result.ok && result.read > 0).length;
                  const roundErrors = round.results.filter((result) => !result.ok).length;
                  const warmup = !round.warmup.ok ? "错误" : round.warmup.read > 0 ? "命中" : "写入";
                  return <div key={round.index} className="rounded-medium border border-default-200 p-3 text-sm"><span className="font-medium">第 {round.index} 轮</span>：预热={warmup}，命中读取={roundHits}/{round.results.length}，重新写入={round.results.length - roundHits - roundErrors}，错误={roundErrors}</div>;
                })}
              </div>
            </> : <p className="py-16 text-center text-sm text-default-400">完成测试后，这里显示每轮的缓存命中情况。</p>}
            {error && <Alert color="danger" variant="flat" title="请求失败" description={error} />}
          </CardBody>
        </Card>
      </div>

      <Accordion variant="bordered">
        <AccordionItem key="guide" aria-label="使用说明" title="测试说明与隐私提示" subtitle="固定前缀预热后，观察并发请求的缓存复用情况">
          <div className="flex flex-col gap-3 pb-2 text-sm leading-6 text-default-600">
            <p>每轮都会生成一个新的固定 system 前缀，先发送一次预热请求，等待指定时间后再并发发送请求。Anthropic 格式以 <code>cache_read_input_tokens</code>、OpenAI 格式以 <code>prompt_tokens_details.cached_tokens</code> 大于 0 视为命中。</p>
            <p>OpenAI 格式不统一返回缓存写入 token，因此“重新写入”仅表示未报告缓存命中。</p>
            <Alert color="warning" variant="flat" title="浏览器直连限制">API Key 只保留在当前页面内存中，不会发送到本站服务器；但上游必须允许本站域名的 CORS 请求，否则浏览器会拦截请求。</Alert>
          </div>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
