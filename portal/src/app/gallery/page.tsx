"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Check,
  Copy,
  Download,
  Heart,
  ImageIcon,
  LoaderCircle,
  Maximize2,
  MessageCircle,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { AppImage as Image } from "@/components/app-image";
import { PortalAvatar } from "@/components/portal-avatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  createPortalGalleryComment,
  deletePortalGalleryWork,
  fetchPortalGalleryWork,
  fetchPortalGalleryWorks,
  togglePortalGalleryLike,
  type PortalGalleryComment,
  type PortalGalleryWork,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { usePortalSession } from "@/store/session";

type GallerySort = "latest" | "likes" | "comments";

const sortOptions: Array<{ value: GallerySort; label: string; description: string }> = [
  { value: "latest", label: "最新发布", description: "优先展示刚发布到服务器的作品" },
  { value: "likes", label: "最多点赞", description: "优先展示最受欢迎的作品" },
  { value: "comments", label: "评论最多", description: "优先展示讨论最活跃的作品" },
];

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

function maskEmail(value: string) {
  const email = String(value || "").trim();
  const atIndex = email.indexOf("@");
  if (atIndex <= 1) {
    return email;
  }
  const name = email.slice(0, atIndex);
  const domain = email.slice(atIndex);
  const visible = name.length <= 3 ? name.slice(0, 1) : name.slice(0, 2);
  return `${visible}***${domain}`;
}

function getAuthorName(author: { user_display_name?: string; user_email: string }) {
  return String(author.user_display_name || "").trim() || maskEmail(author.user_email);
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

async function copyImage(imageUrl: string) {
  if (!imageUrl) {
    toast.warning("没有可复制的图片");
    return false;
  }
  try {
    const response = await fetch(imageUrl);
    const blob = await response.blob();
    await navigator.clipboard.write([
      new ClipboardItem({
        [blob.type || "image/png"]: blob,
      }),
    ]);
    toast.success("图片已复制");
    return true;
  } catch {
    try {
      await navigator.clipboard.writeText(imageUrl);
      toast.success("当前浏览器不支持直接复制图片，已复制图片地址");
      return true;
    } catch {
      toast.error("复制图片失败");
      return false;
    }
  }
}

function buildDownloadName(work: PortalGalleryWork) {
  const suffix = work.id.slice(0, 8) || "gallery";
  return `cheilins-gallery-${suffix}.png`;
}

export default function GalleryPage() {
  const { user: currentUser } = usePortalSession();
  const [works, setWorks] = useState<PortalGalleryWork[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [sort, setSort] = useState<GallerySort>("latest");
  const [searchInput, setSearchInput] = useState("");
  const [query, setQuery] = useState("");
  const [selectedWork, setSelectedWork] = useState<PortalGalleryWork | null>(null);
  const [comments, setComments] = useState<PortalGalleryComment[]>([]);
  const [isDetailLoading, setIsDetailLoading] = useState(false);
  const [commentContent, setCommentContent] = useState("");
  const [isCommentSubmitting, setIsCommentSubmitting] = useState(false);
  const [likingWorkId, setLikingWorkId] = useState<string | null>(null);
  const [deletingWorkId, setDeletingWorkId] = useState<string | null>(null);
  const [zoomedImageUrl, setZoomedImageUrl] = useState<string | null>(null);
  const [copiedType, setCopiedType] = useState<string | null>(null);

  const loadWorks = useCallback(async (nextSort: GallerySort, nextQuery: string, silent = false) => {
    if (!silent) {
      setIsLoading(true);
    }
    try {
      const payload = await fetchPortalGalleryWorks({
        sort: nextSort,
        query: nextQuery,
      });
      setWorks(payload.items);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取作品广场失败");
    } finally {
      if (!silent) {
        setIsLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void loadWorks(sort, query);
  }, [loadWorks, query, sort]);

  const openWork = useCallback(async (work: PortalGalleryWork) => {
    setSelectedWork(work);
    setComments([]);
    setCommentContent("");
    setIsDetailLoading(true);
    try {
      const payload = await fetchPortalGalleryWork(work.id);
      setSelectedWork(payload.item);
      setComments(payload.comments);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取作品详情失败");
    } finally {
      setIsDetailLoading(false);
    }
  }, []);

  const updateWorkCache = useCallback((workId: string, updater: (item: PortalGalleryWork) => PortalGalleryWork) => {
    setWorks((current) => current.map((item) => (item.id === workId ? updater(item) : item)));
    setSelectedWork((current) => (current && current.id === workId ? updater(current) : current));
  }, []);

  const handleToggleLike = useCallback(
    async (workId: string) => {
      setLikingWorkId(workId);
      try {
        const payload = await togglePortalGalleryLike(workId);
        updateWorkCache(workId, (item) => ({
          ...item,
          liked_by_viewer: payload.liked,
          like_count: payload.like_count,
        }));
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "点赞失败");
      } finally {
        setLikingWorkId(null);
      }
    },
    [updateWorkCache],
  );

  const handleDeleteWork = useCallback(
    async (workId: string) => {
      if (!window.confirm("确定要删除这张作品吗？删除后不可恢复。")) {
        return;
      }
      setDeletingWorkId(workId);
      try {
        await deletePortalGalleryWork(workId);
        setWorks((current) => current.filter((item) => item.id !== workId));
        if (selectedWork?.id === workId) {
          setSelectedWork(null);
        }
        toast.success("作品已删除");
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "删除失败");
      } finally {
        setDeletingWorkId(null);
      }
    },
    [selectedWork?.id],
  );

  const handleSubmitComment = useCallback(async () => {
    if (!selectedWork) {
      return;
    }
    const content = commentContent.trim();
    if (!content) {
      toast.warning("请输入评论内容");
      return;
    }

    setIsCommentSubmitting(true);
    try {
      const payload = await createPortalGalleryComment(selectedWork.id, content);
      setComments((current) => [...current, payload.item]);
      setCommentContent("");
      updateWorkCache(selectedWork.id, (item) => ({
        ...item,
        comment_count: payload.comment_count,
      }));
      toast.success("评论已发布");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "发表评论失败");
    } finally {
      setIsCommentSubmitting(false);
    }
  }, [commentContent, selectedWork, updateWorkCache]);

  const galleryStats = useMemo(() => ({
    works: works.length,
    likes: works.reduce((sum, item) => sum + item.like_count, 0),
    comments: works.reduce((sum, item) => sum + item.comment_count, 0),
  }), [works]);

  return (
    <section className="flex h-full min-h-0 flex-col gap-3">
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-start gap-4">
            <span className="flex size-12 shrink-0 items-center justify-center rounded-2xl bg-stone-950 text-white">
              <Sparkles className="size-5" />
            </span>
            <div className="min-w-0">
              <h1 className="text-xl font-semibold tracking-tight text-stone-950">作品广场</h1>
              <p className="mt-2 text-sm leading-6 text-stone-600">
                发布到这里的图片会保存到服务器，其他登录用户可以查看大图、复制提示词、下载、点赞和评论。
              </p>
            </div>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-3 xl:grid-cols-1">
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">广场作品</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{galleryStats.works}</div>
          </div>
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">累计点赞</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{galleryStats.likes}</div>
          </div>
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">累计评论</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{galleryStats.comments}</div>
          </div>
        </div>
      </div>

      <div className="min-h-0 overflow-hidden rounded-[30px] border border-stone-200 bg-white shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
        <div className="border-b border-stone-100 px-6 py-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-stone-950">公开作品流</h2>
              <p className="mt-1 text-sm text-stone-500">优先展示已经发布到服务器的共享作品，支持搜索提示词和作者。</p>
            </div>

            <form
              className="flex w-full max-w-[520px] flex-col gap-2 sm:flex-row sm:items-center"
              onSubmit={(event) => {
                event.preventDefault();
                setQuery(searchInput.trim());
              }}
            >
              <div className="relative flex-1">
                <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-stone-400" />
                <Input
                  value={searchInput}
                  onChange={(event) => setSearchInput(event.target.value)}
                  placeholder="搜索标题、提示词、作者昵称或邮箱"
                  className="h-11 rounded-2xl border-stone-200 bg-stone-50 pl-10"
                />
              </div>
              <div className="flex gap-2">
                <Button type="submit" className="h-11 flex-1 rounded-2xl bg-stone-950 text-white hover:bg-stone-800 sm:flex-none">
                  搜索
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  className="h-11 rounded-2xl px-4"
                  onClick={() => void loadWorks(sort, query)}
                >
                  <RefreshCw className="size-4" />
                </Button>
              </div>
            </form>
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            {sortOptions.map((option) => (
              <button
                key={option.value}
                type="button"
                className={cn(
                  "rounded-full px-4 py-2 text-sm font-medium transition",
                  sort === option.value
                    ? "bg-stone-950 text-white"
                    : "bg-stone-100 text-stone-600 hover:bg-stone-200 hover:text-stone-900",
                )}
                onClick={() => setSort(option.value)}
                title={option.description}
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>

        <div className="hide-scrollbar h-full overflow-auto px-5 py-5">
          {isLoading ? (
            <div className="grid min-h-[320px] place-items-center text-stone-500">
              <div className="flex items-center gap-3">
                <LoaderCircle className="size-4 animate-spin" />
                正在读取作品广场...
              </div>
            </div>
          ) : works.length === 0 ? (
            <div className="grid min-h-[320px] place-items-center rounded-[26px] border border-dashed border-stone-200 bg-stone-50 px-6 text-center text-sm leading-6 text-stone-500">
              当前还没有可展示的公开作品。先在“我的作品”或“图片工作台”中发布几张作品到广场吧。
            </div>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
              {works.map((work) => (
                <article
                  key={work.id}
                  className="group overflow-hidden rounded-[26px] border border-stone-200 bg-[#fafaf8] shadow-[0_10px_30px_rgba(15,23,42,0.05)] transition-all hover:-translate-y-0.5 hover:shadow-[0_18px_45px_rgba(15,23,42,0.09)]"
                >
                  <div
                    className="relative aspect-square cursor-pointer overflow-hidden bg-stone-100"
                    onClick={() => void openWork(work)}
                  >
                    <Image
                      src={work.image_url}
                      alt={work.title}
                      className="block h-full w-full object-cover transition duration-500 group-hover:scale-105"
                    />
                    <button
                      type="button"
                      className={cn(
                        "absolute top-3 right-3 z-10 flex items-center gap-1.5 rounded-full bg-black/45 px-2.5 py-1.5 text-[11px] font-semibold text-white backdrop-blur-md transition hover:bg-black/60",
                        work.liked_by_viewer && "text-rose-300",
                      )}
                      onClick={(event) => {
                        event.stopPropagation();
                        void handleToggleLike(work.id);
                      }}
                      disabled={likingWorkId === work.id}
                      title="点赞"
                    >
                      {likingWorkId === work.id ? (
                        <LoaderCircle className="size-3 animate-spin" />
                      ) : (
                        <Heart className={cn("size-3.5", work.liked_by_viewer && "fill-current")} />
                      )}
                      {work.like_count}
                    </button>

                    <div className="absolute inset-x-0 bottom-0 z-10 flex items-center justify-center gap-2 bg-gradient-to-t from-black/55 to-transparent p-4 transition-opacity duration-300 lg:opacity-0 lg:group-hover:opacity-100">
                      <button
                        type="button"
                        className="flex size-9 items-center justify-center rounded-full bg-black/45 text-white backdrop-blur-md transition hover:bg-black/65"
                        onClick={(event) => {
                          event.stopPropagation();
                          void openWork(work);
                        }}
                        title="查看评论"
                      >
                        <MessageCircle className="size-4" />
                      </button>
                      <button
                        type="button"
                        className="flex size-9 items-center justify-center rounded-full bg-black/45 text-white backdrop-blur-md transition hover:bg-black/65"
                        onClick={(event) => {
                          event.stopPropagation();
                          setZoomedImageUrl(work.image_url);
                        }}
                        title="放大图片"
                      >
                        <Maximize2 className="size-4" />
                      </button>
                      <a
                        href={work.image_url}
                        download={buildDownloadName(work)}
                        className="flex size-9 items-center justify-center rounded-full bg-black/45 text-white backdrop-blur-md transition hover:bg-black/65"
                        title="下载图片"
                        onClick={(event) => event.stopPropagation()}
                      >
                        <Download className="size-4" />
                      </a>
                      <button
                        type="button"
                        className="flex size-9 items-center justify-center rounded-full bg-black/45 text-white backdrop-blur-md transition hover:bg-black/65"
                        onClick={(event) => {
                          event.stopPropagation();
                          void copyImage(work.image_url);
                        }}
                        title="复制图片"
                      >
                        <ImageIcon className="size-4" />
                      </button>
                      <button
                        type="button"
                        className="flex size-9 items-center justify-center rounded-full bg-black/45 text-white backdrop-blur-md transition hover:bg-black/65"
                        onClick={(event) => {
                          event.stopPropagation();
                          void copyText(work.prompt);
                        }}
                        title="复制提示词"
                      >
                        <Copy className="size-4" />
                      </button>
                      {(currentUser?.role === "admin" || currentUser?.id === work.user_id) && (
                        <button
                          type="button"
                          className="flex size-9 items-center justify-center rounded-full bg-rose-500/85 text-white backdrop-blur-md transition hover:bg-rose-600"
                          onClick={(event) => {
                            event.stopPropagation();
                            void handleDeleteWork(work.id);
                          }}
                          disabled={deletingWorkId === work.id}
                          title="删除作品"
                        >
                          {deletingWorkId === work.id ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                        </button>
                      )}
                    </div>
                  </div>

                  <div className="space-y-3 px-4 py-4">
                    <div className="flex items-center gap-3">
                      <PortalAvatar
                        src={work.user_avatar_url}
                        name={work.user_display_name}
                        email={work.user_email}
                        className="size-10"
                      />
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-stone-900">{getAuthorName(work)}</div>
                        <div className="truncate text-xs text-stone-500">{maskEmail(work.user_email)}</div>
                      </div>
                    </div>

                    <div>
                      <button type="button" className="line-clamp-1 text-left text-sm font-semibold tracking-tight text-stone-950" onClick={() => void openWork(work)}>
                        {work.title}
                      </button>
                      <div className="mt-1 text-xs text-stone-500">{formatDate(work.created_at)}</div>
                    </div>

                    <div className="line-clamp-3 text-sm leading-6 text-stone-600">{work.prompt}</div>

                    <div className="flex items-center gap-3 text-xs text-stone-500">
                      <span className="inline-flex items-center gap-1">
                        <Heart className="size-3.5" />
                        {work.like_count}
                      </span>
                      <span className="inline-flex items-center gap-1">
                        <MessageCircle className="size-3.5" />
                        {work.comment_count}
                      </span>
                    </div>

                    <div className="flex flex-wrap items-center gap-2 border-t border-stone-200 pt-3">
                      <Button
                        type="button"
                        variant="outline"
                        className={cn(
                          "h-9 rounded-full px-3",
                          work.liked_by_viewer && "border-rose-200 bg-rose-50 text-rose-700 hover:bg-rose-100",
                        )}
                        disabled={likingWorkId === work.id}
                        onClick={() => void handleToggleLike(work.id)}
                      >
                        {likingWorkId === work.id ? <LoaderCircle className="size-4 animate-spin" /> : <Heart className="size-4" />}
                        点赞
                      </Button>
                      <Button type="button" variant="outline" className="h-9 rounded-full px-3" onClick={() => void openWork(work)}>
                        <MessageCircle className="size-4" />
                        评论
                      </Button>
                      <Button type="button" variant="outline" className="h-9 rounded-full px-3" onClick={() => void copyText(work.prompt)}>
                        <Copy className="size-4" />
                        复制提示词
                      </Button>
                      <a
                        href={work.image_url}
                        download={buildDownloadName(work)}
                        className="inline-flex h-9 items-center gap-2 rounded-full border border-stone-200 bg-white px-3 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
                      >
                        <Download className="size-4" />
                        下载
                      </a>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </div>

      <Dialog open={Boolean(selectedWork)} onOpenChange={(open) => (!open ? setSelectedWork(null) : null)}>
        <DialogContent className="w-[min(96vw,1180px)] max-h-[90vh] overflow-y-auto rounded-[24px] border border-stone-200 bg-white p-0 lg:overflow-hidden lg:rounded-[30px]">
          {selectedWork ? (
            <div className="flex min-h-0 flex-col lg:grid lg:max-h-[90vh] lg:grid-cols-[minmax(0,1fr)_380px]">
              <div className="bg-[#f7f7f4] p-4 sm:p-5 lg:min-h-0 lg:overflow-auto">
                <div className="overflow-hidden rounded-[24px] border border-stone-200 bg-white shadow-sm">
                  <Image src={selectedWork.image_url} alt={selectedWork.title} className="block h-auto w-full object-contain" />
                </div>
              </div>

              <div className="flex min-h-0 flex-col border-t border-stone-100 p-5 sm:p-6 lg:border-t-0 lg:border-l">
                <DialogHeader className="gap-3">
                  <DialogTitle className="text-2xl tracking-tight text-stone-950">{selectedWork.title}</DialogTitle>
                  <DialogDescription className="not-sr-only">
                    <span className="flex items-center gap-3 text-sm leading-6 text-stone-500">
                      <PortalAvatar
                        src={selectedWork.user_avatar_url}
                        name={selectedWork.user_display_name}
                        email={selectedWork.user_email}
                        className="size-11"
                      />
                      <span className="min-w-0">
                        <span className="block truncate font-medium text-stone-700">{getAuthorName(selectedWork)}</span>
                        <span className="block truncate text-xs text-stone-500">
                          {maskEmail(selectedWork.user_email)} · {formatDate(selectedWork.created_at)}
                        </span>
                      </span>
                    </span>
                  </DialogDescription>
                </DialogHeader>

                <div className="mt-4 flex flex-wrap gap-2 text-xs text-stone-500">
                  {selectedWork.model ? <span className="rounded-full bg-stone-100 px-3 py-1.5">{selectedWork.model}</span> : null}
                  {selectedWork.size ? <span className="rounded-full bg-stone-100 px-3 py-1.5">{selectedWork.size}</span> : null}
                  <span className="rounded-full bg-stone-100 px-3 py-1.5">点赞 {selectedWork.like_count}</span>
                  <span className="rounded-full bg-stone-100 px-3 py-1.5">评论 {selectedWork.comment_count}</span>
                </div>

                <div className="mt-5 rounded-[24px] bg-stone-50 px-4 py-4 text-sm leading-7 text-stone-700">
                  {selectedWork.prompt}
                </div>

                <div className="mt-5 grid gap-2 sm:grid-cols-2">
                  {(currentUser?.role === "admin" || currentUser?.id === selectedWork.user_id) && (
                    <Button
                      type="button"
                      variant="destructive"
                      className="h-10 rounded-xl bg-rose-600 font-semibold hover:bg-rose-700"
                      disabled={deletingWorkId === selectedWork.id}
                      onClick={() => void handleDeleteWork(selectedWork.id)}
                    >
                      {deletingWorkId === selectedWork.id ? (
                        <LoaderCircle className="mr-2 size-4 animate-spin" />
                      ) : (
                        <Trash2 className="mr-2 size-4" />
                      )}
                      删除作品
                    </Button>
                  )}
                  <Button
                    type="button"
                    className={cn(
                      "h-10 rounded-xl font-semibold",
                      selectedWork.liked_by_viewer ? "bg-rose-600 text-white hover:bg-rose-500" : "bg-stone-950 text-white hover:bg-stone-800",
                    )}
                    disabled={likingWorkId === selectedWork.id}
                    onClick={() => void handleToggleLike(selectedWork.id)}
                  >
                    {likingWorkId === selectedWork.id ? (
                      <LoaderCircle className="mr-2 size-4 animate-spin" />
                    ) : (
                      <Heart className={cn("mr-2 size-4", selectedWork.liked_by_viewer && "fill-current")} />
                    )}
                    {selectedWork.liked_by_viewer ? "已点赞" : "点赞作品"}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-10 rounded-xl border-stone-200 bg-white font-medium text-stone-700"
                    onClick={async () => {
                      const success = await copyImage(selectedWork.image_url);
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
                    href={selectedWork.image_url}
                    download={buildDownloadName(selectedWork)}
                    className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-stone-200 bg-white px-4 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
                  >
                    <Download className="size-4" />
                    下载图片
                  </a>
                </div>

                <div className="mt-6 flex min-h-0 flex-1 flex-col overflow-hidden rounded-[24px] border border-stone-200 bg-stone-50">
                  <div className="flex items-center justify-between border-b border-stone-200 px-4 py-3">
                    <span className="text-sm font-medium text-stone-700">评论区</span>
                    <span className="text-xs text-stone-400">{comments.length} 条评论</span>
                  </div>
                  <div className="min-h-0 flex-1 overflow-auto px-4 py-4">
                    {isDetailLoading ? (
                      <div className="flex items-center gap-2 text-sm text-stone-500">
                        <LoaderCircle className="size-4 animate-spin" />
                        正在读取评论...
                      </div>
                    ) : comments.length === 0 ? (
                      <div className="text-sm leading-6 text-stone-500">还没有评论，来留下第一条想法吧。</div>
                    ) : (
                      <div className="space-y-3">
                        {comments.map((comment) => (
                          <div key={comment.id} className="rounded-[20px] bg-white px-4 py-3 shadow-sm">
                            <div className="flex items-start justify-between gap-3">
                              <div className="flex min-w-0 items-center gap-3">
                                <PortalAvatar
                                  src={comment.user_avatar_url}
                                  name={comment.user_display_name}
                                  email={comment.user_email}
                                  className="size-9"
                                />
                                <div className="min-w-0 text-xs text-stone-500">
                                  <div className="truncate font-medium text-stone-700">{getAuthorName(comment)}</div>
                                  <div className="truncate">{maskEmail(comment.user_email)}</div>
                                </div>
                              </div>
                              <span className="shrink-0 text-xs text-stone-500">{formatDate(comment.created_at)}</span>
                            </div>
                            <div className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-stone-700">
                              {comment.content}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="border-t border-stone-200 px-4 py-4">
                    <Textarea
                      value={commentContent}
                      onChange={(event) => setCommentContent(event.target.value)}
                      placeholder="写下你对这张作品的看法..."
                      className="min-h-[96px] rounded-[20px] border-stone-200 bg-white"
                    />
                    <div className="mt-3 flex justify-end">
                      <Button
                        type="button"
                        className="rounded-full bg-stone-950 text-white hover:bg-stone-800"
                        disabled={isCommentSubmitting || isDetailLoading}
                        onClick={() => void handleSubmitComment()}
                      >
                        {isCommentSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : <MessageCircle className="size-4" />}
                        发表评论
                      </Button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(zoomedImageUrl)} onOpenChange={(open) => (!open ? setZoomedImageUrl(null) : null)}>
        <DialogContent className="max-h-[94vh] w-[min(96vw,1280px)] overflow-hidden rounded-[28px] border border-stone-200 bg-stone-950 p-0">
          {zoomedImageUrl ? (
            <div className="relative">
              <button
                type="button"
                className="absolute top-4 right-4 z-10 flex size-10 items-center justify-center rounded-full bg-white/15 text-white backdrop-blur-md transition hover:bg-white/25"
                onClick={() => setZoomedImageUrl(null)}
                title="关闭"
              >
                <X className="size-5" />
              </button>
              <Image src={zoomedImageUrl} alt="放大的作品图片" className="block max-h-[94vh] w-full object-contain" />
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}
