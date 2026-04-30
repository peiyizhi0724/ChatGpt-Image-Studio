"use client";

import { useEffect, useMemo, useState } from "react";
import { Check, Copy, Download, HardDrive, ImageIcon, LoaderCircle, Maximize2, Sparkles, Upload, X } from "lucide-react";
import { toast } from "sonner";

import { AppImage as Image } from "@/components/app-image";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { publishPortalGalleryWork } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  buildGalleryPublishRecordKey,
  listGalleryPublishRecords,
  saveGalleryPublishRecord,
  type GalleryPublishRecord,
} from "@/store/gallery-publish-records";
import {
  listImageConversations,
  type ImageConversation,
  type ImageConversationTurn,
  type StoredImage,
} from "@/store/image-conversations";

type MyWorkItem = {
  key: string;
  conversationId: string;
  turnId: string;
  imageId: string;
  imageUrl: string;
  title: string;
  prompt: string;
  modeLabel: string;
  model: string;
  size?: string;
  createdAt: string;
};

const modeLabelMap = {
  generate: "生成",
  edit: "编辑",
  upscale: "放大",
} as const;

function buildImageDataUrl(image: StoredImage) {
  if (image.url) {
    return image.url;
  }
  if (!image.b64_json) {
    return "";
  }
  return `data:image/png;base64,${image.b64_json}`;
}

function buildTitle(turn: ImageConversationTurn) {
  const prompt = turn.prompt.trim();
  if (!prompt) {
    return `${modeLabelMap[turn.mode]}作品`;
  }
  return prompt.length > 24 ? `${prompt.slice(0, 24)}...` : prompt;
}

function buildPrompt(turn: ImageConversationTurn) {
  const prompt = turn.prompt.trim();
  return prompt || `${modeLabelMap[turn.mode]}作品`;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatSimpleDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return `${date.getFullYear()}/${date.getMonth() + 1}/${date.getDate()}`;
}

function buildDownloadName(createdAt: string, imageId: string) {
  const date = new Date(createdAt);
  const suffix = imageId.slice(0, 8) || "image";
  if (Number.isNaN(date.getTime())) {
    return `cheilins-studio-work-${suffix}.png`;
  }
  const yyyy = String(date.getFullYear());
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const dd = String(date.getDate()).padStart(2, "0");
  const hh = String(date.getHours()).padStart(2, "0");
  const min = String(date.getMinutes()).padStart(2, "0");
  const sec = String(date.getSeconds()).padStart(2, "0");
  return `cheilins-studio-work-${yyyy}${mm}${dd}-${hh}${min}${sec}-${suffix}.png`;
}

async function copyText(text: string) {
  const value = text.trim();
  if (!value) {
    toast.warning("没有可复制的提示词");
    return false;
  }

  try {
    await navigator.clipboard.writeText(value);
    toast.success("提示词已复制");
    return true;
  } catch {
    toast.error("复制失败");
    return false;
  }
}

async function copyImage(url: string) {
  try {
    const response = await fetch(url);
    const blob = await response.blob();
    await navigator.clipboard.write([
      new ClipboardItem({
        [blob.type]: blob,
      }),
    ]);
    toast.success("图片已复制到剪贴板");
    return true;
  } catch (error) {
    console.error("Copy image failed:", error);
    toast.error("图片复制失败，请尝试右键另存为");
    return false;
  }
}

function flattenWorks(conversations: ImageConversation[]) {
  const items: MyWorkItem[] = [];
  conversations.forEach((conversation) => {
    (conversation.turns || []).forEach((turn) => {
      turn.images.forEach((image) => {
        const imageUrl = buildImageDataUrl(image);
        if (image.status !== "success" || !imageUrl) {
          return;
        }
        items.push({
          key: buildGalleryPublishRecordKey(conversation.id, turn.id, image.id),
          conversationId: conversation.id,
          turnId: turn.id,
          imageId: image.id,
          imageUrl,
          title: buildTitle(turn),
          prompt: buildPrompt(turn),
          modeLabel: modeLabelMap[turn.mode],
          model: turn.model,
          size: turn.size,
          createdAt: turn.createdAt,
        });
      });
    });
  });

  return items.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

export default function WorksPage() {
  const [works, setWorks] = useState<MyWorkItem[]>([]);
  const [records, setRecords] = useState<Record<string, GalleryPublishRecord>>({});
  const [isLoading, setIsLoading] = useState(true);
  const [selectedWork, setSelectedWork] = useState<MyWorkItem | null>(null);
  const [publishingKey, setPublishingKey] = useState<string | null>(null);
  const [zoomedImageUrl, setZoomedImageUrl] = useState<string | null>(null);
  const [copiedType, setCopiedType] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [conversations, publishRecords] = await Promise.all([
          listImageConversations(),
          listGalleryPublishRecords(),
        ]);
        if (cancelled) {
          return;
        }
        setWorks(flattenWorks(conversations));
        setRecords(publishRecords);
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : "读取本地作品失败");
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  const publishedCount = useMemo(
    () => works.filter((item) => Boolean(records[item.key])).length,
    [records, works],
  );

  const handlePublish = async (work: MyWorkItem) => {
    if (records[work.key]) {
      toast.message("这张图片已经发布过了");
      return;
    }

    setPublishingKey(work.key);
    try {
      const payload = await publishPortalGalleryWork({
        title: work.title,
        prompt: work.prompt,
        image_data_url: work.imageUrl.startsWith("data:") ? work.imageUrl : undefined,
        image_url: work.imageUrl.startsWith("data:") ? undefined : work.imageUrl,
        model: work.model,
        size: work.size,
      });
      const record: GalleryPublishRecord = {
        key: work.key,
        work_id: payload.item.id,
        published_at: new Date().toISOString(),
      };
      await saveGalleryPublishRecord(record);
      setRecords((current) => ({
        ...current,
        [work.key]: record,
      }));
      toast.success("已发布到作品广场");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "发布失败");
    } finally {
      setPublishingKey(null);
    }
  };

  return (
    <section className="flex h-full min-h-0 flex-col gap-3">
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_480px]">
        <div className="rounded-[24px] border border-stone-200 bg-white p-5 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-start gap-4">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-amber-100 text-amber-700">
              <HardDrive className="size-4" />
            </span>
            <div className="min-w-0">
              <h1 className="text-lg font-semibold tracking-tight text-stone-950">我的作品</h1>
              <p className="mt-1 text-sm leading-relaxed text-stone-600">
                这里展示的是当前浏览器保存的本地作品记录。清理浏览器缓存、更换设备或更换浏览器后，这些数据不会自动同步回来。
              </p>
              <div className="mt-2 inline-flex rounded-full border border-amber-200 bg-amber-50 px-3 py-1 text-[10px] font-medium text-amber-700">
                本页数据仅保存在本地，请将重要作品发布到作品广场或自行备份
              </div>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className="rounded-[24px] border border-stone-200 bg-white p-4 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-xs font-medium text-stone-500">本地作品数</div>
            <div className="mt-1 text-2xl font-semibold tracking-tight text-stone-950">{works.length}</div>
          </div>
          <div className="rounded-[24px] border border-stone-200 bg-white p-4 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-xs font-medium text-stone-500">已发布到广场</div>
            <div className="mt-1 text-2xl font-semibold tracking-tight text-stone-950">{publishedCount}</div>
          </div>
        </div>
      </div>

      <div className="min-h-0 overflow-hidden rounded-[24px] border border-stone-200 bg-white shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
        <div className="flex items-center justify-between border-b border-stone-100 px-6 py-4">
          <div>
            <h2 className="text-base font-semibold tracking-tight text-stone-950">本地作品列表</h2>
            <p className="mt-0.5 text-xs text-stone-500">点击卡片可查看大图、复制提示词或发布到作品广场。</p>
          </div>
        </div>

        <div className="hide-scrollbar h-full overflow-auto px-5 py-5">
          {isLoading ? (
            <div className="grid min-h-[320px] place-items-center text-stone-500">
              <div className="flex items-center gap-3">
                <LoaderCircle className="size-4 animate-spin" />
                正在读取本地作品...
              </div>
            </div>
          ) : works.length === 0 ? (
            <div className="grid min-h-[320px] place-items-center rounded-[26px] border border-dashed border-stone-200 bg-stone-50 px-6 text-center text-sm leading-6 text-stone-500">
              还没有本地作品。先去图片工作台生成一批图片，这里会自动读取当前浏览器保存的结果。
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
              {works.map((work) => {
                const published = Boolean(records[work.key]);
                const publishing = publishingKey === work.key;
                return (
                  <article
                    key={work.key}
                    className="group relative overflow-hidden rounded-[20px] border border-stone-200 bg-white shadow-sm transition-all hover:shadow-md"
                  >
                    <div
                      className="relative aspect-square cursor-pointer overflow-hidden bg-stone-100"
                      onClick={() => setSelectedWork(work)}
                    >
                      <Image
                        src={work.imageUrl}
                        alt={work.title}
                        className="block h-full w-full object-cover transition duration-500 group-hover:scale-105"
                      />

                      {/* Top Right Publish Status */}
                      <div className="absolute top-2.5 right-2.5 z-10">
                        <button
                          type="button"
                          className={cn(
                            "flex items-center gap-1.5 rounded-full px-2.5 py-1.5 text-[11px] font-semibold backdrop-blur-md transition",
                            published 
                              ? "bg-emerald-500/20 text-emerald-400 border border-emerald-400/20"
                              : "bg-black/40 text-white hover:bg-black/60"
                          )}
                          onClick={(e) => {
                            e.stopPropagation();
                            if (!published && !publishing) void handlePublish(work);
                          }}
                          disabled={publishing || published}
                        >
                          {publishing ? (
                            <LoaderCircle className="size-3 animate-spin" />
                          ) : (
                            <Upload className="size-3.5" />
                          )}
                          {published ? "已发布" : "发布广场"}
                        </button>
                      </div>

                      {/* Bottom Actions Overlay */}
                      <div className="absolute inset-x-0 bottom-0 z-10 flex items-center justify-center gap-2.5 bg-gradient-to-t from-black/50 to-transparent p-4 transition-opacity duration-300 lg:opacity-0 lg:group-hover:opacity-100">
                        <button
                          type="button"
                          className="flex size-9 items-center justify-center rounded-full bg-black/40 text-white backdrop-blur-md transition hover:bg-black/60"
                          onClick={(e) => {
                            e.stopPropagation();
                            setZoomedImageUrl(work.imageUrl);
                          }}
                          title="放大图片"
                        >
                          <Maximize2 className="size-4.5" />
                        </button>
                        <a
                          href={work.imageUrl}
                          download={buildDownloadName(work.createdAt, work.imageId)}
                          className="flex size-9 items-center justify-center rounded-full bg-black/40 text-white backdrop-blur-md transition hover:bg-black/60"
                          title="下载图片"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Download className="size-4.5" />
                        </a>
                        <button
                          type="button"
                          className="flex size-9 items-center justify-center rounded-full bg-black/40 text-white backdrop-blur-md transition hover:bg-black/60"
                          onClick={(e) => {
                            e.stopPropagation();
                            void copyImage(work.imageUrl);
                          }}
                          title="复制图片"
                        >
                          <ImageIcon className="size-4.5" />
                        </button>
                        <button
                          type="button"
                          className="flex size-9 items-center justify-center rounded-full bg-black/40 text-white backdrop-blur-md transition hover:bg-black/60"
                          onClick={(e) => {
                            e.stopPropagation();
                            void copyText(work.prompt);
                          }}
                          title="复制提示词"
                        >
                          <Copy className="size-4.5" />
                        </button>
                      </div>
                    </div>

                    <div className="px-4 py-3.5">
                      <div className="flex items-center gap-2">
                        <h3
                          className="cursor-pointer truncate text-[13px] font-bold tracking-tight text-stone-900 hover:text-stone-600"
                          onClick={() => setSelectedWork(work)}
                        >
                          {work.title}
                        </h3>
                      </div>

                      <div className="mt-3 grid grid-cols-2 gap-y-1.5 text-[11px]">
                        <div className="font-bold text-emerald-600">model</div>
                        <div className="truncate text-right font-medium text-indigo-400/80">{work.model || "Standard"}</div>
                        <div className="text-stone-500">{work.size || "Original"}</div>
                        <div className="text-right text-stone-400">{formatSimpleDate(work.createdAt)}</div>
                      </div>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <Dialog open={Boolean(selectedWork)} onOpenChange={(open) => (!open ? setSelectedWork(null) : null)}>
        <DialogContent className="w-[min(96vw,1180px)] max-h-[90vh] overflow-y-auto rounded-[24px] border border-stone-200 bg-white p-0 lg:overflow-hidden lg:rounded-[30px]">
          {selectedWork ? (
            <div className="flex min-h-0 flex-col lg:grid lg:max-h-[90vh] lg:grid-cols-[minmax(0,1fr)_380px]">
              <div className="flex items-center justify-center bg-[#f7f7f4] p-4 sm:p-5 lg:h-[90vh] lg:min-h-0 lg:overflow-hidden">
                <div
                  className="relative flex h-full w-full items-center justify-center cursor-zoom-in"
                  onClick={() => setZoomedImageUrl(selectedWork.imageUrl)}
                >
                  <Image
                    src={selectedWork.imageUrl}
                    alt={selectedWork.title}
                    className="block h-full max-h-full w-full max-w-full object-contain drop-shadow-md"
                  />
                </div>
              </div>

              <div className="flex min-h-0 flex-col border-t border-stone-100 p-5 sm:p-6 lg:border-t-0 lg:border-l lg:overflow-y-auto">
                <div className="space-y-4">
                  <h2 className="text-xl font-bold tracking-tight text-stone-950">{selectedWork.title}</h2>
                  
                  <div className="flex flex-wrap gap-2 text-xs text-stone-500">
                    <span className="text-[11px] text-stone-500">
                      创建时间 · {formatDate(selectedWork.createdAt)}
                    </span>
                  </div>

                  <div className="flex flex-wrap gap-2">
                    {selectedWork.model ? (
                      <span className="rounded-lg bg-stone-100 px-2.5 py-1 text-[11px] font-medium text-stone-600">
                        {selectedWork.model}
                      </span>
                    ) : null}
                    {selectedWork.size ? (
                      <span className="rounded-lg bg-stone-100 px-2.5 py-1 text-[11px] font-medium text-stone-600">
                        {selectedWork.size}
                      </span>
                    ) : null}
                    <span className="rounded-lg bg-stone-100 px-2.5 py-1 text-[11px] font-medium text-stone-600">
                      模式：{selectedWork.modeLabel}
                    </span>
                    {records[selectedWork.key] ? (
                      <span className="rounded-lg bg-emerald-50 px-2.5 py-1 text-[11px] font-medium text-emerald-600 border border-emerald-100">
                        已发布到广场
                      </span>
                    ) : null}
                  </div>

                  <div className="rounded-2xl bg-stone-50 p-4 text-sm leading-relaxed text-stone-700">
                    {selectedWork.prompt}
                  </div>

                  <div className="grid grid-cols-2 gap-2.5">
                    <Button
                      type="button"
                      className={cn(
                        "h-10 rounded-xl font-semibold",
                        records[selectedWork.key] 
                          ? "bg-emerald-600 text-white cursor-default" 
                          : "bg-stone-950 text-white hover:bg-stone-800"
                      )}
                      disabled={Boolean(records[selectedWork.key]) || publishingKey === selectedWork.key}
                      onClick={() => void handlePublish(selectedWork)}
                    >
                      {publishingKey === selectedWork.key ? (
                        <LoaderCircle className="mr-2 size-4 animate-spin" />
                      ) : (
                        <Upload className="mr-2 size-4" />
                      )}
                      {records[selectedWork.key] ? "已发布到广场" : "发布到广场"}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      className="h-10 rounded-xl border-stone-200 bg-white font-medium text-stone-700"
                      onClick={async () => {
                        const success = await copyImage(selectedWork.imageUrl);
                        if (success) {
                          setCopiedType("detail-image");
                          setTimeout(() => setCopiedType(null), 2000);
                        }
                      }}
                    >
                      {copiedType === "detail-image" ? (
                        <Check className="mr-2 size-4 text-emerald-500" />
                      ) : (
                        <ImageIcon className="mr-2 size-4" />
                      )}
                      {copiedType === "detail-image" ? "已复制图片" : "复制图片"}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      className="h-10 rounded-xl border-stone-200 bg-white font-medium text-stone-700"
                      onClick={async () => {
                        const success = await copyText(selectedWork.prompt);
                        if (success) {
                          setCopiedType("detail-prompt");
                          setTimeout(() => setCopiedType(null), 2000);
                        }
                      }}
                    >
                      {copiedType === "detail-prompt" ? (
                        <Check className="mr-2 size-4 text-emerald-500" />
                      ) : (
                        <Copy className="mr-2 size-4" />
                      )}
                      {copiedType === "detail-prompt" ? "已复制提示词" : "复制提示词"}
                    </Button>
                    <a
                      href={selectedWork.imageUrl}
                      download={buildDownloadName(selectedWork.createdAt, selectedWork.imageId)}
                      className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-stone-200 bg-white px-4 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
                    >
                      <Download className="size-4" />
                      下载图片
                    </a>
                  </div>
                </div>

                <div className="mt-8 rounded-2xl border border-amber-100 bg-amber-50/50 p-5 text-xs leading-relaxed text-amber-700">
                  <div className="flex items-center gap-2 font-bold mb-1">
                    <Sparkles className="size-3.5" />
                    提示
                  </div>
                  此作品当前仅保存在您的本地浏览器中。如果您希望永久保存或与他人分享，请使用上面的按钮将其发布到作品广场。
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(zoomedImageUrl)} onOpenChange={(open) => (!open ? setZoomedImageUrl(null) : null)}>
        <DialogContent className="max-w-[95vw] border-none bg-transparent p-0 shadow-none outline-none sm:max-w-[90vw]">
          <div className="relative flex flex-col items-center">
            {zoomedImageUrl && (
              <>
                <button
                  type="button"
                  className="absolute -top-12 right-0 flex size-10 items-center justify-center rounded-full bg-black/20 text-white transition hover:bg-black/40"
                  onClick={() => setZoomedImageUrl(null)}
                >
                  <X className="size-6" />
                </button>
                <div className="relative overflow-hidden rounded-2xl shadow-2xl">
                  <img
                    src={zoomedImageUrl}
                    alt="Zoomed"
                    className="max-h-[85vh] w-auto object-contain"
                  />

                  {/* Overlay Info for Zoomed Image */}
                  <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/80 via-black/40 to-transparent p-6 text-white">
                    <p className="line-clamp-2 text-sm leading-relaxed text-stone-200">
                      {works.find(w => w.imageUrl === zoomedImageUrl)?.prompt ||
                       (selectedWork?.imageUrl === zoomedImageUrl ? selectedWork.prompt : "")}
                    </p>
                    <div className="mt-4 flex items-center gap-3">
                      <Button
                        variant="secondary"
                        size="sm"
                        className="h-9 rounded-full bg-white/20 text-white backdrop-blur-md hover:bg-white/30"
                        onClick={async () => {
                          const success = await copyImage(zoomedImageUrl!);
                          if (success) {
                            setCopiedType("zoom-image");
                            setTimeout(() => setCopiedType(null), 2000);
                          }
                        }}
                      >
                        {copiedType === "zoom-image" ? <Check className="mr-2 size-4 text-emerald-400" /> : <ImageIcon className="mr-2 size-4" />}
                        {copiedType === "zoom-image" ? "已复制" : "复制"}
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        className="h-9 rounded-full bg-white/20 text-white backdrop-blur-md hover:bg-white/30"
                        onClick={async () => {
                          const work = works.find(w => w.imageUrl === zoomedImageUrl) || (selectedWork?.imageUrl === zoomedImageUrl ? selectedWork : null);
                          if (work) {
                            const success = await copyText(work.prompt);
                            if (success) {
                              setCopiedType("zoom-prompt");
                              setTimeout(() => setCopiedType(null), 2000);
                            }
                          }
                        }}
                      >
                        {copiedType === "zoom-prompt" ? <Check className="mr-2 size-4 text-emerald-400" /> : <Copy className="mr-2 size-4" />}
                        {copiedType === "zoom-prompt" ? "已复制" : "提示词"}
                      </Button>
                      <a
                        href={zoomedImageUrl}
                        download="download.png"
                        className="inline-flex h-9 items-center rounded-full bg-white/20 px-4 text-sm font-medium text-white backdrop-blur-md hover:bg-white/30"
                      >
                        <Download className="mr-2 size-4" />
                        下载
                      </a>
                    </div>
                  </div>
                </div>
              </>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
