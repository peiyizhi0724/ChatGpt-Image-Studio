"use client";

import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Download, RefreshCw, ShieldCheck, TriangleAlert, XCircle } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { STARTUP_CHECK_COMPLETED_KEY } from "@/constants/startup-check";
import { downloadDiagnosticsExport, fetchStartupCheck, type StartupCheckResponse } from "@/lib/api";

function statusBadgeVariant(status: string): "success" | "warning" | "danger" | "secondary" {
  if (status === "pass") {
    return "success";
  }
  if (status === "warn") {
    return "warning";
  }
  if (status === "fail") {
    return "danger";
  }
  return "secondary";
}

function statusLabel(status: string) {
  if (status === "pass") {
    return "通过";
  }
  if (status === "warn") {
    return "警告";
  }
  if (status === "fail") {
    return "失败";
  }
  return status || "未知";
}

export default function StartupCheckPage() {
  const navigate = useNavigate();
  const [isLoading, setIsLoading] = useState(true);
  const [isDownloading, setIsDownloading] = useState(false);
  const [result, setResult] = useState<StartupCheckResponse | null>(null);

  const runCheck = async () => {
    setIsLoading(true);
    try {
      const data = await fetchStartupCheck();
      setResult(data);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "启动体检失败");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void runCheck();
  }, []);

  const headerIcon = useMemo(() => {
    if (!result || result.overall === "pass") {
      return <ShieldCheck className="size-5" />;
    }
    if (result.overall === "warn") {
      return <TriangleAlert className="size-5" />;
    }
    return <XCircle className="size-5" />;
  }, [result]);

  const handleDownloadDiagnostics = async () => {
    setIsDownloading(true);
    try {
      const { blob, fileName } = await downloadDiagnosticsExport();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = fileName;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
      toast.success("诊断包已下载");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "诊断包下载失败");
    } finally {
      setIsDownloading(false);
    }
  };

  const completeAndContinue = () => {
    try {
      localStorage.setItem(STARTUP_CHECK_COMPLETED_KEY, "1");
    } catch {
      // ignore localStorage errors
    }
    navigate("/image", { replace: true });
  };

  return (
    <section className="h-full overflow-y-auto">
      <div className="mx-auto flex max-w-[1440px] flex-col gap-6 px-1 py-1">
        <div className="rounded-[30px] border border-stone-200 bg-white px-5 py-5 shadow-[0_14px_40px_rgba(15,23,42,0.05)] sm:px-6">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0">
              <div className="flex items-start gap-4">
                <div className="inline-flex size-12 shrink-0 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                  {headerIcon}
                </div>
                <div className="min-w-0">
                  <h1 className="text-2xl font-semibold tracking-tight text-stone-950">首次启动体检</h1>
                  <p className="mt-2 max-w-[860px] text-sm leading-7 text-stone-500">
                    检查服务、代理、官方链路、CPA 与账号可用性，避免启动后空白页或调用失败。
                  </p>
                  {result ? (
                    <p className="mt-2 text-xs text-stone-500">{result.summaryText}</p>
                  ) : null}
                </div>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none"
                onClick={() => void runCheck()}
                disabled={isLoading}
              >
                <RefreshCw className={isLoading ? "size-4 animate-spin" : "size-4"} />
                重新检测
              </Button>
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none"
                onClick={() => void handleDownloadDiagnostics()}
                disabled={isDownloading}
              >
                <Download className={isDownloading ? "size-4 animate-pulse" : "size-4"} />
                导出诊断包
              </Button>
              <Button
                type="button"
                className="h-10 rounded-full bg-stone-950 px-4 text-white hover:bg-stone-800"
                onClick={completeAndContinue}
              >
                进入工作台
              </Button>
            </div>
          </div>
        </div>

        <Card className="border-stone-200 bg-white shadow-[0_18px_48px_rgba(15,23,42,0.05)]">
          <CardContent className="space-y-4 p-6">
            {result?.checks?.map((item) => (
              <div key={item.key} className="rounded-2xl border border-stone-200 bg-stone-50/60 p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="text-sm font-semibold text-stone-900">{item.label}</div>
                  <div className="flex items-center gap-2">
                    <Badge variant={statusBadgeVariant(item.status)}>{statusLabel(item.status)}</Badge>
                    <span className="text-xs text-stone-400">{item.durationMs} ms</span>
                  </div>
                </div>
                <div className="mt-2 text-sm text-stone-600">{item.detail}</div>
                {item.hint ? <div className="mt-1 text-xs text-stone-500">建议：{item.hint}</div> : null}
              </div>
            ))}
            {!isLoading && (!result || result.checks.length === 0) ? (
              <div className="rounded-2xl border border-stone-200 bg-stone-50/60 p-4 text-sm text-stone-500">
                暂无检测结果，请点击“重新检测”。
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
