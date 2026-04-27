"use client";

import { useEffect, useMemo, useState } from "react";
import { Copy, Download, HardDrive, LoaderCircle, Sparkles, Upload } from "lucide-react";
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
    return;
  }

  try {
    await navigator.clipboard.writeText(value);
    toast.success("提示词已复制");
  } catch {
    toast.error("复制失败");
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
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-start gap-4">
            <span className="flex size-12 shrink-0 items-center justify-center rounded-2xl bg-amber-100 text-amber-700">
              <HardDrive className="size-5" />
            </span>
            <div className="min-w-0">
              <h1 className="text-xl font-semibold tracking-tight text-stone-950">我的作品</h1>
              <p className="mt-2 text-sm leading-6 text-stone-600">
                这里展示的是当前浏览器保存的本地作品记录。清理浏览器缓存、更换设备或更换浏览器后，这些数据不会自动同步回来。
              </p>
              <div className="mt-3 inline-flex rounded-full border border-amber-200 bg-amber-50 px-3 py-1.5 text-xs font-medium text-amber-700">
                本页数据仅保存在本地，请将重要作品发布到作品广场或自行备份
              </div>
            </div>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">本地作品数</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{works.length}</div>
            <div className="mt-2 text-sm text-stone-500">来自当前浏览器保存的历史会话</div>
          </div>
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">已发布到广场</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{publishedCount}</div>
            <div className="mt-2 text-sm text-stone-500">发布后会保存到服务器，可供多人浏览互动</div>
          </div>
        </div>
      </div>

      <div className="min-h-0 overflow-hidden rounded-[30px] border border-stone-200 bg-white shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
        <div className="flex items-center justify-between border-b border-stone-100 px-6 py-5">
          <div>
            <h2 className="text-lg font-semibold tracking-tight text-stone-950">本地作品列表</h2>
            <p className="mt-1 text-sm text-stone-500">点击卡片可查看大图、复制提示词或发布到作品广场。</p>
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
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
              {works.map((work) => {
                const published = Boolean(records[work.key]);
                const publishing = publishingKey === work.key;
                return (
                  <div
                    key={work.key}
                    role="button"
                    tabIndex={0}
                    className="group overflow-hidden rounded-[26px] border border-stone-200 bg-[#fafaf8] text-left shadow-[0_10px_30px_rgba(15,23,42,0.05)] transition hover:-translate-y-0.5 hover:shadow-[0_16px_36px_rgba(15,23,42,0.09)]"
                    onClick={() => setSelectedWork(work)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        setSelectedWork(work);
                      }
                    }}
                  >
                    <div className="relative overflow-hidden bg-stone-100">
                      <Image
                        src={work.imageUrl}
                        alt={work.title}
                        className="block aspect-square w-full object-cover transition duration-300 group-hover:scale-[1.02]"
                      />
                      <div className="absolute top-3 left-3 flex flex-wrap gap-2">
                        <span className="rounded-full bg-white/92 px-3 py-1 text-xs font-medium text-stone-700 shadow-sm">
                          {work.modeLabel}
                        </span>
                        {published ? (
                          <span className="rounded-full bg-emerald-100 px-3 py-1 text-xs font-medium text-emerald-700">
                            已发布
                          </span>
                        ) : null}
                      </div>
                    </div>

                    <div className="space-y-3 px-4 py-4">
                      <div>
                        <div className="line-clamp-1 text-sm font-semibold tracking-tight text-stone-950">{work.title}</div>
                        <div className="mt-1 text-xs text-stone-500">{formatDate(work.createdAt)}</div>
                      </div>

                      <div className="line-clamp-3 text-sm leading-6 text-stone-600">{work.prompt}</div>

                      <div className="flex flex-wrap items-center gap-2 border-t border-stone-200 pt-3">
                        <Button
                          type="button"
                          variant="outline"
                          className={cn(
                            "h-9 rounded-full px-3",
                            published && "border-emerald-200 bg-emerald-50 text-emerald-700 hover:bg-emerald-100",
                          )}
                          disabled={published || publishing}
                          onClick={(event) => {
                            event.stopPropagation();
                            void handlePublish(work);
                          }}
                        >
                          {publishing ? <LoaderCircle className="size-4 animate-spin" /> : <Upload className="size-4" />}
                          {published ? "已发布" : "发布"}
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          className="h-9 rounded-full px-3"
                          onClick={(event) => {
                            event.stopPropagation();
                            void copyText(work.prompt);
                          }}
                        >
                          <Copy className="size-4" />
                          复制提示词
                        </Button>
                        <a
                          href={work.imageUrl}
                          download={buildDownloadName(work.createdAt, work.imageId)}
                          className="inline-flex h-9 items-center gap-2 rounded-full border border-stone-200 bg-white px-3 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
                          onClick={(event) => event.stopPropagation()}
                        >
                          <Download className="size-4" />
                          下载
                        </a>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <Dialog open={Boolean(selectedWork)} onOpenChange={(open) => (!open ? setSelectedWork(null) : null)}>
        <DialogContent className="w-[min(96vw,1040px)] overflow-hidden rounded-[30px] border border-stone-200 bg-white p-0">
          {selectedWork ? (
            <div className="grid max-h-[88vh] min-h-0 lg:grid-cols-[minmax(0,1fr)_320px]">
              <div className="min-h-0 overflow-auto bg-[#f7f7f4] p-5">
                <div className="overflow-hidden rounded-[24px] border border-stone-200 bg-white shadow-sm">
                  <Image src={selectedWork.imageUrl} alt={selectedWork.title} className="block h-auto w-full object-contain" />
                </div>
              </div>

              <div className="flex min-h-0 flex-col border-t border-stone-100 p-6 lg:border-t-0 lg:border-l">
                <DialogHeader className="gap-3">
                  <DialogTitle className="text-2xl tracking-tight text-stone-950">{selectedWork.title}</DialogTitle>
                  <DialogDescription className="text-sm leading-6 text-stone-500">
                    本作品源自本地历史记录。若需长期保存并让其他用户查看、评论、点赞，请发布到作品广场。
                  </DialogDescription>
                </DialogHeader>

                <div className="mt-4 flex flex-wrap gap-2 text-xs text-stone-500">
                  <span className="rounded-full bg-stone-100 px-3 py-1.5">{selectedWork.modeLabel}</span>
                  <span className="rounded-full bg-stone-100 px-3 py-1.5">{selectedWork.model}</span>
                  {selectedWork.size ? <span className="rounded-full bg-stone-100 px-3 py-1.5">{selectedWork.size}</span> : null}
                  <span className="rounded-full bg-stone-100 px-3 py-1.5">{formatDate(selectedWork.createdAt)}</span>
                </div>

                <div className="mt-5 rounded-[24px] bg-stone-50 px-4 py-4 text-sm leading-7 text-stone-700">
                  {selectedWork.prompt}
                </div>

                <div className="mt-5 flex flex-wrap gap-2">
                  <Button
                    className="rounded-full bg-stone-950 text-white hover:bg-stone-800"
                    disabled={Boolean(records[selectedWork.key]) || publishingKey === selectedWork.key}
                    onClick={() => void handlePublish(selectedWork)}
                  >
                    {publishingKey === selectedWork.key ? <LoaderCircle className="size-4 animate-spin" /> : <Sparkles className="size-4" />}
                    {records[selectedWork.key] ? "已发布到作品广场" : "发布到作品广场"}
                  </Button>
                  <Button variant="outline" className="rounded-full" onClick={() => void copyText(selectedWork.prompt)}>
                    <Copy className="size-4" />
                    复制提示词
                  </Button>
                  <a
                    href={selectedWork.imageUrl}
                    download={buildDownloadName(selectedWork.createdAt, selectedWork.imageId)}
                    className="inline-flex h-10 items-center gap-2 rounded-full border border-stone-200 bg-white px-4 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
                  >
                    <Download className="size-4" />
                    下载图片
                  </a>
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}
