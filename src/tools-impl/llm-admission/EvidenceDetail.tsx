import { useEffect, useState } from "react";
import { addToast, Button, Card, CardBody, Chip, Spinner } from "@heroui/react";

import { caseEvidence, type CaseEvidence, type CaseRow } from "./api";

function copyText(text: string) {
  if (!text) {
    addToast({ title: "内容为空，没有可复制的内容", color: "warning" });
    return;
  }
  const fallback = () => {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand("copy");
      addToast({ title: "已复制" });
    } catch {
      addToast({ title: "复制失败，请手动选择文本", color: "danger" });
    } finally {
      document.body.removeChild(ta);
    }
  };
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(text).then(() => addToast({ title: "已复制" })).catch(fallback);
  } else {
    fallback();
  }
}

function CopyButton({ text, label }: { text: string; label: string }) {
  return (
    <Button size="sm" variant="flat" onPress={() => copyText(text)}>
      {label}
    </Button>
  );
}

function formatHeaders(headers?: Record<string, string>): string {
  if (!headers || Object.keys(headers).length === 0) return "（无）";
  return Object.entries(headers).sort(([a], [b]) => a.localeCompare(b)).map(([k, v]) => `${k}: ${v}`).join("\n");
}

export default function EvidenceDetail({ runId, row }: { runId: string; row: CaseRow }) {
  const [data, setData] = useState<CaseEvidence | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError("");
    caseEvidence(runId, row.case_id)
      .then((res) => { if (alive) setData(res); })
      .catch((e) => { if (alive) setError((e as Error).message); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [runId, row.case_id]);

  const allText = data
    ? [
        `用例：${data.case_id} ${row.name}`,
        `结论：${data.status} ${data.summary}`,
        `解释：${data.explanation}`,
        ...data.evidence.map((e) => `\n===== ${e.label} =====\n[请求头]\n${formatHeaders(e.request_headers)}\n[请求]\n${e.request}\n[返回头]\n${formatHeaders(e.response_headers)}\n[返回]\n${e.response}`),
      ].join("\n")
    : "";

  return (
    <Card shadow="none" className="w-full rounded-none border-x-0 border-b-0 border-l-2 border-default-200">
      <CardBody className="gap-3 py-3 pl-3 sm:pl-4">
        {loading && <div className="flex items-center gap-2 text-sm text-default-500"><Spinner size="sm" /> 正在加载请求和返回…</div>}
        {error && <p className="text-sm text-danger">加载失败：{error}</p>}
        {data && (
          <>
            <div>
              <p className="text-sm font-semibold">发生了什么</p>
              <p className="mt-1 text-sm text-default-600">{data.explanation}</p>
              {data.doc.expect && <p className="mt-1 text-xs text-default-400">判据：{data.doc.expect}</p>}
            </div>
            <div className="flex flex-wrap gap-2">
              <CopyButton text={allText} label="复制全部" />
              {data.evidence.map((item, i) => (
                <div key={`${item.label}-${i}`} className="flex flex-wrap gap-2">
                  <CopyButton text={item.request} label={`复制请求 ${i + 1}`} />
                  <CopyButton text={item.response} label={`复制返回 ${i + 1}`} />
                </div>
              ))}
            </div>
            <div className="flex flex-col gap-3">
              {data.evidence.length === 0 && <p className="text-sm text-default-400">该用例没有保存请求/返回证据。</p>}
              {data.evidence.map((item, i) => (
                <div key={`${item.label}-${i}`} className="rounded-medium border border-default-200">
                  <div className="flex flex-wrap items-center gap-2 border-b border-default-200 bg-default-50 px-3 py-2">
                    <span className="text-sm font-medium">{item.label || `请求 ${i + 1}`}</span>
                    <Chip size="sm" variant="flat">HTTP {item.http_status}</Chip>
                    {item.finish_reason && <Chip size="sm" variant="flat">finish: {item.finish_reason}</Chip>}
                    {item.duration_ms ? <span className="text-xs text-default-400">{item.duration_ms}ms</span> : null}
                  </div>
                  <div className="grid gap-2 border-b border-default-100 p-3 lg:grid-cols-2">
                    <details>
                      <summary className="cursor-pointer text-xs font-semibold text-default-500">请求头</summary>
                      <pre className="mt-1 max-h-48 overflow-auto rounded bg-default-50 p-2 text-xs whitespace-pre-wrap break-all">{formatHeaders(item.request_headers)}</pre>
                    </details>
                    <details>
                      <summary className="cursor-pointer text-xs font-semibold text-default-500">返回头</summary>
                      <pre className="mt-1 max-h-48 overflow-auto rounded bg-default-50 p-2 text-xs whitespace-pre-wrap break-all">{formatHeaders(item.response_headers)}</pre>
                    </details>
                  </div>
                  <div className="grid gap-2 p-3 lg:grid-cols-2">
                    <div>
                      <p className="mb-1 text-xs font-semibold text-default-500">请求（脱敏）</p>
                      <pre className="max-h-96 overflow-auto rounded bg-default-50 p-2 text-xs whitespace-pre-wrap break-all">{item.request || "—"}</pre>
                    </div>
                    <div>
                      <p className="mb-1 text-xs font-semibold text-default-500">返回（脱敏）</p>
                      <pre className="max-h-96 overflow-auto rounded bg-default-50 p-2 text-xs whitespace-pre-wrap break-all">{item.response || "—"}</pre>
                    </div>
                  </div>
                </div>
              ))}
            </div>
            {data.defects.length > 0 && (
              <div className="text-xs">
                {data.defects.map((d, i) => (
                  <p key={i} className={d.level === "P2" ? "text-warning" : "text-danger"}>
                    [{d.level}] 期望 {d.expect}；实际 {d.actual}
                    {d.level === "P2" ? "（格式/兼容性问题，不影响已观测到的缓存效果）" : ""}
                  </p>
                ))}
              </div>
            )}
          </>
        )}
      </CardBody>
    </Card>
  );
}
