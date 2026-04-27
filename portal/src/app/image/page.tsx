"use client";

import { memo, useCallback, useEffect, useMemo, useRef, useState, type ClipboardEvent as ReactClipboardEvent } from "react";
import "react-medium-image-zoom/dist/styles.css";
import Zoom from "react-medium-image-zoom";
import {
  ArrowUp,
  Clock3,
  Copy,
  Download,
  History,
  ImagePlus,
  LoaderCircle,
  PanelLeftClose,
  PanelLeftOpen,
  RotateCcw,
  Sparkles,
  Trash2,
  Upload,
  Wand2,
} from "lucide-react";
import { toast } from "sonner";

import { AppImage as Image } from "@/components/app-image";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  editImage,
  fetchAccountQuota,
  fetchPortalWorkspaceBootstrap,
  generateImageWithOptions,
  publishPortalGalleryWork,
  upscaleImage,
  type Account,
  type ImageQuality,
  type PortalAccountQuotaResponse,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  buildGalleryPublishRecordKey,
  listGalleryPublishRecords,
  saveGalleryPublishRecord,
} from "@/store/gallery-publish-records";
import {
  deleteImageConversation,
  listImageConversations,
  normalizeConversation,
  saveImageConversation,
  updateImageConversation,
  type ImageConversation,
  type ImageConversationTurn,
  type ImageMode,
  type StoredImage,
  type StoredSourceImage,
} from "@/store/image-conversations";
import {
  finishImageTask,
  isImageTaskActive,
  listActiveImageTasks,
  startImageTask,
  subscribeImageTasks,
} from "@/store/image-active-tasks";

const modeOptions: Array<{
  label: string;
  value: ImageMode;
  description: string;
}> = [
  {
    label: "生成",
    value: "generate",
    description: "输入提示词生成新图，也可上传参考图辅助生成",
  },
  {
    label: "编辑",
    value: "edit",
    description: "上传图像后整体或局部改图",
  },
  {
    label: "放大",
    value: "upscale",
    description: "提升清晰度并增强细节",
  },
];

const sizeOptions = [
  { value: "1024x1024", label: "1:1 标准" },
  { value: "1536x1024", label: "3:2 横向" },
  { value: "1024x1536", label: "2:3 纵向" },
];

const qualityOptions: Array<{
  value: ImageQuality;
  label: string;
  description: string;
}> = [
  { value: "low", label: "Low", description: "低质量，适合草稿测试" },
  { value: "medium", label: "Medium", description: "均衡质量与速度" },
  { value: "high", label: "High", description: "高质量，适合最终出图" },
];

const upscaleOptions = ["2x", "4x"];

const inspirationExamples = [
  {
    id: "portrait-editorial",
    title: "电影感人物海报",
    prompt: "一位站在雨夜霓虹街头的女性主角，电影海报质感，浅景深，潮湿空气，橙蓝对比光，细节丰富",
    hint: "适合测试人物主视觉和电影感光影。",
    tone: "from-slate-950 via-indigo-900 to-amber-500",
  },
  {
    id: "interior-design",
    title: "室内空间概念图",
    prompt: "现代侘寂风客厅，清晨斜射光，原木、石材与亚麻织物，构图克制，高级家居杂志风格",
    hint: "适合空间、家居与商业提案展示。",
    tone: "from-stone-900 via-stone-700 to-neutral-300",
  },
  {
    id: "product-macro",
    title: "产品特写广告图",
    prompt: "高端护肤瓶置于雾面金属台座，微距镜头，柔雾体积光，极简高级广告视觉，背景留白",
    hint: "适合品牌视觉、电商主图和材质测试。",
    tone: "from-zinc-950 via-zinc-700 to-rose-200",
  },
  {
    id: "fantasy-landscape",
    title: "奇幻场景设定图",
    prompt: "悬浮山脉之间的古代空中神殿，金色日落云海，史诗感构图，概念设定图风格",
    hint: "适合世界观、游戏和影视概念图。",
    tone: "from-emerald-950 via-teal-700 to-amber-300",
  },
];

const modeLabelMap: Record<ImageMode, string> = {
  generate: "生成",
  edit: "编辑",
  upscale: "放大",
};

type WorkspaceConfig = {
  allowDisabledStudioAccounts: boolean;
};

type ActiveRequestState = {
  conversationId: string;
  turnId: string;
  mode: ImageMode;
  count: number;
  variant: "standard" | "selection-edit";
};

type HistorySidebarProps = {
  conversations: ImageConversation[];
  selectedConversationId: string | null;
  isLoadingHistory: boolean;
  hasActiveTasks: boolean;
  activeConversationIds: Set<string>;
  onCreateDraft: () => void;
  onClearHistory: () => Promise<void>;
  onFocusConversation: (id: string) => void;
  onDeleteConversation: (id: string) => Promise<void>;
};

type WorkspaceHeaderProps = {
  historyCollapsed: boolean;
  selectedConversationTitle?: string | null;
  onToggleHistory: () => void;
};

type ProcessingStatus = {
  title: string;
  detail: string;
};

type ConversationTurnsProps = {
  conversationId: string;
  turns: ImageConversationTurn[];
  activeRequest: ActiveRequestState | null;
  isSubmitting: boolean;
  processingStatus: ProcessingStatus | null;
  waitingDots: string;
  submitElapsedSeconds: number;
  publishedImageKeys: Set<string>;
  publishingImageKey: string | null;
  onSeedFromResult: (conversationId: string, image: StoredImage, nextMode: ImageMode) => void;
  onPublishImage: (conversationId: string, turn: ImageConversationTurn, image: StoredImage) => Promise<void>;
  onRetryTurn: (conversationId: string, turn: ImageConversationTurn) => Promise<void>;
};

type PromptComposerProps = {
  mode: ImageMode;
  imageCount: string;
  imageSize: string;
  imageQuality: ImageQuality;
  upscaleScale: string;
  availableQuota: string;
  sourceImages: StoredSourceImage[];
  imagePrompt: string;
  hasGenerateReferences: boolean;
  isSubmitting: boolean;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
  uploadInputRef: React.RefObject<HTMLInputElement | null>;
  maskInputRef: React.RefObject<HTMLInputElement | null>;
  onModeChange: (mode: ImageMode) => void;
  onImageCountChange: (value: string) => void;
  onImageSizeChange: (value: string) => void;
  onImageQualityChange: (value: ImageQuality) => void;
  onUpscaleScaleChange: (value: string) => void;
  onPromptChange: (value: string) => void;
  onPromptPaste: (event: ReactClipboardEvent<HTMLTextAreaElement>) => void;
  onRemoveSourceImage: (id: string) => void;
  onAppendFiles: (files: FileList | null, role: "image" | "mask") => Promise<void>;
  onMobileCollapsedChange: (collapsed: boolean) => void;
  onSubmit: () => Promise<void>;
};

function sortConversations(items: ImageConversation[]) {
  return [...items].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

function makeId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function formatConversationTime(value: string) {
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

function formatProcessingDuration(totalSeconds: number) {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes <= 0) {
    return `${seconds}s`;
  }
  return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
}

function buildWaitingDots(totalSeconds: number) {
  return ".".repeat((totalSeconds % 3) + 1);
}

function buildProcessingStatus(mode: ImageMode, elapsedSeconds: number, count: number) {
  if (mode === "generate") {
    if (elapsedSeconds < 4) {
      return { title: "正在提交生成请求", detail: `已进入图像生成队列，本次目标 ${count} 张` };
    }
    if (elapsedSeconds < 12) {
      return { title: "正在排队创建画面", detail: "模型正在准备构图与风格细节" };
    }
    return { title: "模型正在生成图片", detail: "通常需要 20 到 90 秒，请保持页面开启" };
  }
  if (mode === "edit") {
    if (elapsedSeconds < 4) {
      return { title: "正在提交编辑请求", detail: "请求已发送，正在准备处理素材" };
    }
    if (elapsedSeconds < 12) {
      return { title: "正在上传编辑素材", detail: "素材上传完成后会进入改图阶段" };
    }
    return { title: "模型正在编辑图片", detail: "通常需要 20 到 90 秒，请保持页面开启" };
  }
  if (elapsedSeconds < 4) {
    return { title: "正在提交放大请求", detail: "请求已发送，正在准备放大素材" };
  }
  if (elapsedSeconds < 12) {
    return { title: "正在上传待放大图片", detail: "素材上传完成后会进入增强阶段" };
  }
  return { title: "模型正在放大图片", detail: "通常需要 20 到 90 秒，请保持页面开启" };
}

function buildConversationTitle(mode: ImageMode, prompt: string, scale = "") {
  const prefix = mode === "generate" ? "生成" : mode === "edit" ? "编辑" : "放大";
  const trimmed = prompt.trim();
  if (!trimmed) {
    return scale ? `${prefix} · ${scale}` : `${prefix} · 图片任务`;
  }
  return `${prefix} · ${trimmed.slice(0, 24)}`;
}

function buildGalleryWorkPrompt(turn: ImageConversationTurn) {
  const prompt = turn.prompt.trim();
  if (prompt) {
    return prompt;
  }
  return `${modeLabelMap[turn.mode]}作品`;
}

function buildGalleryWorkTitle(turn: ImageConversationTurn) {
  const prompt = buildGalleryWorkPrompt(turn);
  return prompt.length > 24 ? `${prompt.slice(0, 24)}...` : prompt;
}

function getImageRemaining(account: Account) {
  const limit = account.limits_progress?.find((item) => item.feature_name === "image_gen");
  if (typeof limit?.remaining === "number") {
    return Math.max(0, limit.remaining);
  }
  return Math.max(0, account.quota);
}

function isImageAccountUsable(account: Account, allowDisabled: boolean) {
  const disabled = Boolean(account.disabled) || account.status === "禁用";
  return (!disabled || allowDisabled) && account.status !== "异常" && account.status !== "限流" && getImageRemaining(account) > 0;
}

function formatAvailableQuota(accounts: Account[], allowDisabled: boolean) {
  return String(accounts.filter((item) => isImageAccountUsable(item, allowDisabled)).reduce((sum, item) => sum + getImageRemaining(item), 0));
}

function mergeImageGenLimit(
  limitsProgress: Account["limits_progress"],
  remaining: number | null | undefined,
  resetAfter: string | null | undefined,
) {
  const next = Array.isArray(limitsProgress) ? [...limitsProgress] : [];
  const currentIndex = next.findIndex((item) => item.feature_name === "image_gen");
  const nextItem = {
    feature_name: "image_gen",
    remaining: typeof remaining === "number" ? remaining : undefined,
    reset_after: resetAfter || undefined,
  };

  if (currentIndex >= 0) {
    next[currentIndex] = {
      ...next[currentIndex],
      ...nextItem,
    };
    return next;
  }

  next.push(nextItem);
  return next;
}

function applyQuotaResultToAccount(account: Account, quota: PortalAccountQuotaResponse): Account {
  return {
    ...account,
    status: quota.status,
    type: quota.type,
    quota: quota.quota,
    restoreAt: quota.image_gen_reset_after || account.restoreAt,
    limits_progress: mergeImageGenLimit(account.limits_progress, quota.image_gen_remaining, quota.image_gen_reset_after),
  };
}

function createLoadingImages(count: number, turnId: string): StoredImage[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `${turnId}-${index + 1}`,
    status: "loading",
  }));
}

function buildConversationBase(conversationId: string, draftTurn: ImageConversationTurn): ImageConversation {
  return {
    id: conversationId,
    title: draftTurn.title,
    mode: draftTurn.mode,
    prompt: draftTurn.prompt,
    model: draftTurn.model,
    count: draftTurn.count,
    size: draftTurn.size,
    quality: draftTurn.quality,
    scale: draftTurn.scale,
    sourceImages: draftTurn.sourceImages,
    images: draftTurn.images,
    createdAt: draftTurn.createdAt,
    status: draftTurn.status,
    error: draftTurn.error,
    turns: [draftTurn],
  };
}

function mergeResultImages(
  turnId: string,
  items: Array<{
    url?: string;
    b64_json?: string;
    revised_prompt?: string;
    file_id?: string;
    gen_id?: string;
    conversation_id?: string;
    parent_message_id?: string;
    source_account_id?: string;
  }>,
  expected: number,
) {
  const results: StoredImage[] = items.map((item, index) =>
    item.b64_json || item.url
      ? {
          id: `${turnId}-${index + 1}`,
          status: "success",
          b64_json: item.b64_json,
          url: item.url,
          revised_prompt: item.revised_prompt,
          file_id: item.file_id,
          gen_id: item.gen_id,
          conversation_id: item.conversation_id,
          parent_message_id: item.parent_message_id,
          source_account_id: item.source_account_id,
        }
      : {
          id: `${turnId}-${index + 1}`,
          status: "error",
          error: "接口没有返回图片数据",
        },
  );

  while (results.length < expected) {
    results.push({
      id: `${turnId}-${results.length + 1}`,
      status: "error",
      error: "接口返回的图片数量不足",
    });
  }

  return results;
}

function countFailures(images: StoredImage[]) {
  return images.filter((image) => image.status === "error").length;
}

function buildImageDataUrl(image: StoredImage) {
  if (image.url) {
    return image.url;
  }
  if (!image.b64_json) {
    return "";
  }
  return `data:image/png;base64,${image.b64_json}`;
}

function buildSourceImageUrl(source: StoredSourceImage) {
  return String(source.dataUrl || source.url || "").trim();
}

function buildConversationPreviewSource(conversation: ImageConversation) {
  const latestSuccessfulImage = conversation.images.find(
    (image) => image.status === "success" && (image.b64_json || image.url),
  );
  if (latestSuccessfulImage) {
    return buildImageDataUrl(latestSuccessfulImage);
  }
  const firstSourceImage = conversation.sourceImages?.find((item) => item.role === "image");
  return firstSourceImage ? buildSourceImageUrl(firstSourceImage) : "";
}

function buildConversationSourceLabel(source: StoredSourceImage) {
  return source.role === "mask" ? "选区 / 遮罩" : "源图";
}

async function fileToDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(new Error(`读取 ${file.name} 失败`));
    reader.readAsDataURL(file);
  });
}

async function dataUrlToFile(dataUrl: string, fileName: string) {
  const response = await fetch(dataUrl);
  const blob = await response.blob();
  return new File([blob], fileName, { type: blob.type || "image/png" });
}

function formatTurnSizeLabel(size?: string) {
  return String(size || "")
    .trim()
    .replace("x", " X ");
}

function buildDownloadName(createdAt: string, turnId: string, index: number) {
  const date = new Date(createdAt);
  const safeIndex = String(index + 1).padStart(2, "0");
  if (Number.isNaN(date.getTime())) {
    return `cheilins-studio-${turnId.slice(0, 8)}-${safeIndex}.png`;
  }

  const yyyy = String(date.getFullYear());
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const dd = String(date.getDate()).padStart(2, "0");
  const hh = String(date.getHours()).padStart(2, "0");
  const min = String(date.getMinutes()).padStart(2, "0");
  const sec = String(date.getSeconds()).padStart(2, "0");
  return `cheilins-studio-${yyyy}${mm}${dd}-${hh}${min}${sec}-${safeIndex}.png`;
}

async function copyPromptToClipboard(prompt: string) {
  const text = prompt.trim();
  if (!text) {
    toast.warning("没有可复制的提示词");
    return;
  }

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      const input = document.createElement("textarea");
      input.value = text;
      input.setAttribute("readonly", "");
      input.style.position = "fixed";
      input.style.left = "-9999px";
      document.body.appendChild(input);
      input.select();
      document.execCommand("copy");
      document.body.removeChild(input);
    }
    toast.success("提示词已复制");
  } catch {
    toast.error("复制失败");
  }
}

async function normalizeConversationHistory(items: ImageConversation[]) {
  const normalized = items.map((item) => {
    let changed = false;
    const conversation = normalizeConversation(item);
    const turns = (conversation.turns || []).map((turn) => {
      if (turn.status !== "generating" || isImageTaskActive(conversation.id, turn.id)) {
        return turn;
      }

      changed = true;
      const errorMessage = turn.images.some((image) => image.status === "success")
        ? turn.error || "任务已中断"
        : "页面已刷新，任务已中断";

      return {
        ...turn,
        status: "error" as const,
        error: errorMessage,
        images: turn.images.map((image) =>
          image.status === "loading"
            ? {
                ...image,
                status: "error" as const,
                error: "页面已刷新，任务已中断",
              }
            : image,
        ),
      };
    });

    return {
      changed,
      conversation: normalizeConversation({
        ...conversation,
        turns,
      }),
    };
  });

  await Promise.all(
    normalized
      .filter((item) => item.changed)
      .map((item) => saveImageConversation(item.conversation)),
  );

  return normalized.map((item) => item.conversation);
}

const HistorySidebar = memo(function HistorySidebar({
  conversations,
  selectedConversationId,
  isLoadingHistory,
  hasActiveTasks,
  activeConversationIds,
  onCreateDraft,
  onClearHistory,
  onFocusConversation,
  onDeleteConversation,
}: HistorySidebarProps) {
  return (
    <aside className="order-2 max-h-[36vh] overflow-hidden rounded-[28px] border border-stone-200 bg-[#f8f8f7] shadow-[0_8px_30px_rgba(15,23,42,0.04)] lg:order-none lg:max-h-none lg:min-h-0">
      <div className="flex h-full min-h-0 flex-col">
        <div className="border-b border-stone-200/80 px-4 py-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-stone-900">
                历史记录
              </h2>
            </div>
            <span className="rounded-full bg-white px-3 py-1 text-xs font-medium text-stone-500 shadow-sm">
              {conversations.length}
            </span>
          </div>
          <div className="mt-4 flex items-center gap-2">
            <Button className="h-11 flex-1 rounded-2xl bg-stone-950 text-white hover:bg-stone-800" onClick={onCreateDraft}>
              <Wand2 className="size-4" />
              新建对话
            </Button>
            <Button
              variant="outline"
              className="h-11 rounded-2xl border-stone-200 bg-white px-3 text-stone-600 hover:bg-stone-50"
              onClick={() => void onClearHistory()}
              disabled={conversations.length === 0 || hasActiveTasks}
              title={hasActiveTasks ? "有任务运行中时不能清空历史" : "清空历史记录"}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3">
          {isLoadingHistory ? (
            <div className="flex items-center gap-2 rounded-2xl px-3 py-3 text-sm text-stone-500">
              <LoaderCircle className="size-4 animate-spin" />
              正在读取会话记录
            </div>
          ) : conversations.length === 0 ? (
            <div className="px-3 py-4 text-sm leading-6 text-stone-500">
              还没有历史记录。创建第一条图片任务后，会在这里保留缩略图和提示词摘要。
            </div>
          ) : (
            <div className="space-y-2">
              {conversations.map((conversation) => {
                const active = conversation.id === selectedConversationId;
                const isDeletingDisabled = activeConversationIds.has(conversation.id);
                const previewSrc = buildConversationPreviewSource(conversation);
                return (
                  <div
                    key={conversation.id}
                    className={cn(
                      "group rounded-[22px] border p-2 transition",
                      active
                        ? "border-stone-200 bg-white shadow-sm"
                        : "border-transparent bg-transparent hover:border-stone-200/80 hover:bg-white/70",
                    )}
                  >
                    <div className="flex items-center gap-3">
                      <button
                        type="button"
                        onClick={() => onFocusConversation(conversation.id)}
                        className="flex min-w-0 flex-1 items-center gap-3 text-left"
                      >
                        <div className="flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-stone-100">
                          {previewSrc ? (
                            <Image src={previewSrc} alt={conversation.title} width={56} height={56} unoptimized className="h-full w-full object-cover" />
                          ) : (
                            <History className="size-4 text-stone-400" />
                          )}
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="rounded-full bg-stone-100 px-2 py-0.5 text-[11px] font-medium text-stone-500">
                              {modeLabelMap[conversation.mode]}
                            </span>
                            <span className="truncate text-xs text-stone-400">
                              {formatConversationTime(conversation.createdAt)}
                            </span>
                          </div>
                          <div className="mt-2 truncate text-sm font-medium text-stone-800">
                            {conversation.title}
                          </div>
                          <div className="mt-1 line-clamp-2 text-xs leading-5 text-stone-500">
                            {conversation.prompt || "无额外提示词"}
                          </div>
                        </div>
                      </button>
                      <button
                        type="button"
                        onClick={() => void onDeleteConversation(conversation.id)}
                        disabled={isDeletingDisabled}
                        title={isDeletingDisabled ? "当前会话仍在处理中，暂时不能删除" : "删除会话"}
                        className="inline-flex size-8 shrink-0 items-center justify-center rounded-xl text-stone-400 opacity-100 transition hover:bg-stone-100 hover:text-rose-500 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-stone-400 lg:opacity-0 lg:group-hover:opacity-100 lg:disabled:opacity-40"
                        aria-label="删除会话"
                      >
                        <Trash2 className="size-4" />
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </aside>
  );
});

function WorkspaceHeader({
  historyCollapsed,
  selectedConversationTitle,
  onToggleHistory,
}: WorkspaceHeaderProps) {
  return (
    <div className="hidden border-b border-stone-200/80 px-5 py-4 sm:px-6 lg:block">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            className="h-10 rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none"
            onClick={onToggleHistory}
          >
            {historyCollapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
            {historyCollapsed ? "展开历史" : "收起历史"}
          </Button>
          <h1 className="text-xl font-semibold tracking-tight text-stone-950 sm:text-[22px]">
            图片工作台
          </h1>
          {selectedConversationTitle ? (
            <span className="truncate rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-600">
              {selectedConversationTitle}
            </span>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function EmptyState({
  onApplyPromptExample,
}: {
  onApplyPromptExample: (example: (typeof inspirationExamples)[number]) => void;
}) {
  return (
    <div className="mx-auto flex max-w-[1120px] flex-col gap-8 px-4 py-8 sm:px-6">
      <div className="max-w-[760px]">
        <div className="inline-flex size-14 items-center justify-center rounded-[20px] bg-stone-950 text-white shadow-sm">
          <Sparkles className="size-5" />
        </div>
        <h1 className="mt-6 text-3xl font-semibold tracking-tight text-stone-950 lg:text-5xl">
          从一个提示词，开始完整的图像工作流。
        </h1>
      </div>

      <div className="hide-scrollbar flex gap-3 overflow-x-auto pb-1 md:grid md:grid-cols-2 md:overflow-visible xl:grid-cols-4">
        {inspirationExamples.map((example) => (
          <button
            key={example.id}
            type="button"
            onClick={() => onApplyPromptExample(example)}
            className="w-[220px] shrink-0 overflow-hidden rounded-[22px] border border-stone-200 bg-white text-left transition hover:-translate-y-0.5 hover:border-stone-300 hover:shadow-sm md:w-auto"
          >
            <div className={cn("h-[4.5rem] bg-gradient-to-br md:h-20", example.tone)} />
            <div className="space-y-2 px-4 py-3.5">
              <div className="flex items-center gap-2 text-[11px] text-stone-500">
                <span className="rounded-full bg-stone-100 px-2 py-0.5 font-medium">Prompt</span>
              </div>
              <div className="text-sm font-semibold tracking-tight text-stone-900">
                {example.title}
              </div>
              <div className="line-clamp-2 text-sm leading-6 text-stone-600">
                {example.prompt}
              </div>
              <div className="border-t border-stone-100 pt-2 text-xs leading-5 text-stone-500">
                {example.hint}
              </div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

const ConversationTurns = memo(function ConversationTurns({
  conversationId,
  turns,
  activeRequest,
  isSubmitting,
  processingStatus,
  waitingDots,
  submitElapsedSeconds,
  publishedImageKeys,
  publishingImageKey,
  onSeedFromResult,
  onPublishImage,
  onRetryTurn,
}: ConversationTurnsProps) {
  return (
    <div className="mx-auto flex w-full max-w-[1120px] flex-col gap-8 px-4 pt-0 pb-8 sm:px-6 sm:py-8">
      {turns.map((turn) => {
        const turnProcessing = Boolean(
          isSubmitting &&
            activeRequest &&
            activeRequest.conversationId === conversationId &&
            activeRequest.turnId === turn.id,
        );

        return (
          <div key={turn.id} className="space-y-4">
            <div className="flex justify-end">
              <div className="flex w-full max-w-[78%] flex-col items-end gap-4">
                {turn.sourceImages && turn.sourceImages.length > 0 ? (
                  <div className="flex flex-wrap justify-end gap-2.5">
                    {turn.sourceImages.map((source) => (
                      <div key={source.id} className="w-[136px] overflow-hidden rounded-[20px] border border-stone-200 bg-white shadow-sm">
                        <div className="border-b border-stone-100 px-3 py-2 text-left text-[11px] font-medium text-stone-500">
                          {buildConversationSourceLabel(source)}
                        </div>
                        <Zoom>
                          <Image
                            src={buildSourceImageUrl(source)}
                            alt={source.name}
                            width={220}
                            height={160}
                            unoptimized
                            className="block h-24 w-full cursor-zoom-in bg-stone-50 object-contain"
                          />
                        </Zoom>
                      </div>
                    ))}
                  </div>
                ) : null}
                <div className="group flex max-w-full flex-col items-start gap-1.5">
                  <div className="min-w-0 whitespace-pre-wrap break-words rounded-[28px] bg-[#f2f2f1] px-5 py-4 text-[15px] leading-7 text-stone-800 shadow-[inset_0_1px_0_rgba(255,255,255,0.75)]">
                    {turn.prompt || "无额外提示词"}
                  </div>
                  <button
                    type="button"
                    onClick={() => void copyPromptToClipboard(turn.prompt || "")}
                    className="inline-flex h-7 shrink-0 items-center gap-1 rounded-full border border-stone-200 bg-white px-2.5 text-xs font-medium text-stone-500 opacity-0 shadow-sm transition hover:bg-stone-100 hover:text-stone-900 focus-visible:opacity-100 focus-visible:outline-none group-hover:opacity-100"
                    title="复制提示词"
                    aria-label="复制提示词"
                  >
                    <Copy className="size-3.5" />
                    复制
                  </button>
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <div className="flex items-center gap-3 px-1">
                <span className="flex size-9 items-center justify-center rounded-2xl bg-stone-950 text-white">
                  <Sparkles className="size-4" />
                </span>
                <div>
                  <div className="text-sm font-semibold tracking-tight text-stone-900">
                    Cheilins Studio
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2 px-1 text-xs text-stone-500">
                <span className="rounded-full bg-stone-100 px-3 py-1.5">{modeLabelMap[turn.mode]}</span>
                <span className="rounded-full bg-stone-100 px-3 py-1.5">{turn.model}</span>
                <span className="rounded-full bg-stone-100 px-3 py-1.5">{turn.count} 张</span>
                {turn.size ? <span className="rounded-full bg-stone-100 px-3 py-1.5">{formatTurnSizeLabel(turn.size)}</span> : null}
                {turn.quality ? <span className="rounded-full bg-stone-100 px-3 py-1.5">Quality {turn.quality}</span> : null}
                {turn.scale ? <span className="rounded-full bg-stone-100 px-3 py-1.5">{turn.scale}</span> : null}
                <span className="rounded-full bg-stone-100 px-3 py-1.5">
                  <Clock3 className="mr-1 inline size-3.5" />
                  {formatConversationTime(turn.createdAt)}
                </span>
              </div>

              {turn.images.length > 0 ? (
                <div className={cn("grid gap-4", turn.images.length === 1 ? "grid-cols-1" : "grid-cols-1 lg:grid-cols-2")}>
                  {turn.images.map((image, index) => {
                    const imageDataUrl = buildImageDataUrl(image);
                    const downloadName = buildDownloadName(turn.createdAt, turn.id, index);
                    const publishRecordKey = buildGalleryPublishRecordKey(conversationId, turn.id, image.id);
                    const published = publishedImageKeys.has(publishRecordKey);
                    const publishing = publishingImageKey === publishRecordKey;
                    return (
                      <div
                        key={image.id}
                        className={cn(
                          "overflow-hidden rounded-[22px] border border-stone-200 bg-white shadow-sm",
                          turn.images.length === 1 && "w-fit max-w-full justify-self-start",
                        )}
                      >
                        {image.status === "success" && imageDataUrl ? (
                          <div>
                            <Zoom>
                              <Image
                                src={imageDataUrl}
                                alt={`Generated result ${index + 1}`}
                                width={1024}
                                height={1024}
                                unoptimized
                                className="block h-auto max-h-[360px] w-auto max-w-full cursor-zoom-in"
                              />
                            </Zoom>
                            <div className="flex flex-wrap items-center gap-2 border-t border-stone-100 px-4 py-3">
                              <button
                                type="button"
                                className="inline-flex size-9 items-center justify-center rounded-full border border-stone-200 bg-white text-stone-600 transition hover:bg-stone-100 hover:text-stone-900"
                                onClick={() => onSeedFromResult(conversationId, image, "edit")}
                                title="引用到编辑"
                                aria-label="引用到编辑"
                              >
                                <Copy className="size-4" />
                              </button>
                              <button
                                type="button"
                                className="inline-flex size-9 items-center justify-center rounded-full border border-stone-200 bg-white text-stone-600 transition hover:bg-stone-100 hover:text-stone-900"
                                onClick={() => onSeedFromResult(conversationId, image, "upscale")}
                                title="引用到放大"
                                aria-label="引用到放大"
                              >
                                <Sparkles className="size-4" />
                              </button>
                              <a
                                href={imageDataUrl}
                                download={downloadName}
                                className="inline-flex size-9 items-center justify-center rounded-full border border-stone-200 bg-white text-stone-600 transition hover:bg-stone-100 hover:text-stone-900"
                                title="下载"
                                aria-label="下载"
                              >
                                <Download className="size-4" />
                              </a>
                              <button
                                type="button"
                                className={cn(
                                  "inline-flex size-9 items-center justify-center rounded-full border transition",
                                  published
                                    ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                                    : "border-stone-200 bg-white text-stone-600 hover:bg-stone-100 hover:text-stone-900",
                                )}
                                onClick={() => void onPublishImage(conversationId, turn, image)}
                                title={published ? "已发布到作品广场" : publishing ? "正在发布" : "发布到作品广场"}
                                aria-label={published ? "已发布到作品广场" : "发布到作品广场"}
                                disabled={published || publishing}
                              >
                                {publishing ? <LoaderCircle className="size-4 animate-spin" /> : <Upload className="size-4" />}
                              </button>
                            </div>
                          </div>
                        ) : image.status === "error" ? (
                          <div className="flex min-h-[320px] flex-col">
                            <div className="flex flex-1 items-center justify-center whitespace-pre-line bg-rose-50 px-6 py-8 text-center text-sm leading-7 text-rose-600">
                              {image.error || "处理失败"}
                            </div>
                            <div className="flex flex-wrap items-center gap-2 border-t border-stone-100 px-4 py-3">
                              <button
                                type="button"
                                className="inline-flex size-9 items-center justify-center rounded-full border border-stone-200 bg-white text-rose-600 transition hover:bg-rose-50 hover:text-rose-700 disabled:cursor-not-allowed disabled:opacity-60"
                                onClick={() => void onRetryTurn(conversationId, turn)}
                                disabled={isSubmitting}
                                title={isSubmitting ? "处理中" : "重试"}
                                aria-label="重试"
                              >
                                <RotateCcw className="size-4" />
                              </button>
                            </div>
                          </div>
                        ) : (
                          <div className="flex min-h-[320px] flex-col items-center justify-center gap-3 bg-stone-50 px-6 py-8 text-center text-stone-500">
                            <div className="rounded-full bg-white p-3 shadow-sm">
                              <LoaderCircle className="size-5 animate-spin" />
                            </div>
                            <p className="text-sm font-medium text-stone-700">
                              {turnProcessing && processingStatus ? `${processingStatus.title}${waitingDots}` : "正在处理图片..."}
                            </p>
                            <p className="text-xs leading-6 text-stone-400">
                              {turnProcessing && processingStatus
                                ? `${processingStatus.detail} · 已等待 ${formatProcessingDuration(submitElapsedSeconds)}`
                                : "图片处理通常需要几十秒，请稍候"}
                            </p>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              ) : null}
            </div>
          </div>
        );
      })}
    </div>
  );
});

function PromptComposer({
  mode,
  imageCount,
  imageSize,
  imageQuality,
  upscaleScale,
  availableQuota,
  sourceImages,
  imagePrompt,
  hasGenerateReferences,
  isSubmitting,
  textareaRef,
  uploadInputRef,
  maskInputRef,
  onModeChange,
  onImageCountChange,
  onImageSizeChange,
  onImageQualityChange,
  onUpscaleScaleChange,
  onPromptChange,
  onPromptPaste,
  onRemoveSourceImage,
  onAppendFiles,
  onMobileCollapsedChange,
  onSubmit,
}: PromptComposerProps) {
  const hasComposerContent = imagePrompt.trim().length > 0 || sourceImages.length > 0;
  const previousHasComposerContentRef = useRef(hasComposerContent);
  const [isMobileComposerExpanded, setIsMobileComposerExpanded] = useState(hasComposerContent);
  const isMobileComposerCollapsed = !isMobileComposerExpanded;
  const showMobileExpandedSections = !isMobileComposerCollapsed;

  useEffect(() => {
    if (hasComposerContent && !previousHasComposerContentRef.current) {
      setIsMobileComposerExpanded(true);
    } else if (!hasComposerContent && previousHasComposerContentRef.current) {
      setIsMobileComposerExpanded(false);
    }
    previousHasComposerContentRef.current = hasComposerContent;
  }, [hasComposerContent]);

  useEffect(() => {
    onMobileCollapsedChange(isMobileComposerCollapsed);
  }, [isMobileComposerCollapsed, onMobileCollapsedChange]);

  return (
    <div
      className={cn(
        "fixed inset-x-0 bottom-0 z-30 px-3 backdrop-blur supports-[padding:max(0px)]:pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-4 lg:static lg:inset-auto lg:bottom-auto lg:z-20 lg:rounded-none lg:border-x-0 lg:border-b-0 lg:border-t lg:bg-white lg:px-5 lg:shadow-none",
        isMobileComposerCollapsed
          ? "border-transparent bg-white/96 shadow-none"
          : "rounded-[26px] border border-stone-200 bg-white/96 shadow-[0_18px_50px_-24px_rgba(15,23,42,0.35)]",
        isMobileComposerCollapsed ? "py-1.5 sm:py-2" : "py-2 sm:py-3",
        "lg:border-stone-200 lg:bg-white lg:py-4 lg:shadow-none",
      )}
    >
      <div className="mx-auto flex max-w-[1120px] flex-col gap-3">
        <div
          className={cn(
            "flex-col gap-2 xl:flex-row xl:items-center xl:justify-between",
            showMobileExpandedSections ? "flex" : "hidden lg:flex",
          )}
        >
          <div className="flex items-center gap-2">
            <div className="hide-scrollbar min-w-0 flex-1 -mx-1 overflow-x-auto px-1 xl:mx-0 xl:px-0">
              <div className="inline-flex min-w-max rounded-full bg-stone-100 p-1">
                {modeOptions.map((item) => (
                  <button
                    key={item.value}
                    type="button"
                    onClick={() => onModeChange(item.value)}
                    className={cn(
                      "rounded-full px-3 py-1.5 text-[13px] font-medium transition sm:px-4 sm:py-2 sm:text-sm",
                      mode === item.value
                        ? "bg-stone-950 text-white shadow-sm"
                        : "text-stone-600 hover:bg-stone-200 hover:text-stone-900",
                    )}
                  >
                    {item.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="hide-scrollbar -mx-1 flex items-center gap-1.5 overflow-x-auto px-1 pb-1 sm:mx-0 sm:flex-wrap sm:gap-2 sm:overflow-visible sm:px-0 sm:pb-0">
            {mode === "generate" && !hasGenerateReferences ? (
              <>
                <Select value={imageSize} onValueChange={onImageSizeChange}>
                  <SelectTrigger className="h-9 w-[128px] shrink-0 rounded-full border-stone-200 bg-white text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-10 sm:w-[150px] sm:text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {sizeOptions.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>

                <Select value={imageQuality} onValueChange={(value) => onImageQualityChange(value as ImageQuality)}>
                  <SelectTrigger className="h-9 w-[130px] shrink-0 rounded-full border-stone-200 bg-white text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-10 sm:w-[150px] sm:text-sm">
                    <SelectValue>{`质量 ${qualityOptions.find((item) => item.value === imageQuality)?.label || imageQuality}`}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {qualityOptions.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        <span title={item.description}>{item.label}</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>

                <div className="flex shrink-0 items-center gap-1 rounded-full border border-stone-200 bg-white px-2 py-0.5 sm:gap-1.5 sm:px-2.5 sm:py-1">
                  <span className="text-[13px] font-medium text-stone-700 sm:text-sm">张数</span>
                  <Input
                    type="number"
                    min="1"
                    max="4"
                    step="1"
                    value={imageCount}
                    onChange={(event) => onImageCountChange(event.target.value)}
                    className="h-7 w-[36px] border-0 bg-transparent px-0 text-center text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-8 sm:w-[42px] sm:text-sm"
                  />
                </div>
              </>
            ) : null}

            {mode === "upscale" ? (
              <Select value={upscaleScale} onValueChange={onUpscaleScaleChange}>
                <SelectTrigger className="h-9 w-[110px] shrink-0 rounded-full border-stone-200 bg-white text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-10 sm:w-[124px] sm:text-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {upscaleOptions.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}

            <span className="shrink-0 rounded-full bg-stone-100 px-2.5 py-1.5 text-[11px] font-medium text-stone-600 sm:px-3 sm:py-2 sm:text-xs">
              剩余额度 {availableQuota}
            </span>
          </div>
        </div>

        <div
          className="overflow-hidden rounded-[24px] border border-stone-200 bg-[#fafaf9] shadow-[inset_0_1px_0_rgba(255,255,255,0.9)] sm:rounded-[28px]"
          onClick={() => {
            setIsMobileComposerExpanded(true);
            textareaRef.current?.focus();
          }}
        >
          {sourceImages.length > 0 ? (
            <div
              className={cn(
                "hide-scrollbar gap-2 overflow-x-auto border-b border-stone-200 px-3 py-2 sm:gap-3 sm:px-4 sm:py-3",
                showMobileExpandedSections ? "flex" : "hidden lg:flex",
              )}
            >
              {sourceImages.map((item) => (
                <div key={item.id} className="w-[104px] shrink-0 overflow-hidden rounded-[16px] border border-stone-200 bg-white sm:w-[126px] sm:rounded-[18px]">
                  <div className="flex items-center justify-between border-b border-stone-100 px-3 py-2 text-[11px] font-medium text-stone-500">
                    <span>{item.role === "mask" ? "遮罩" : "源图"}</span>
                    <button
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        onRemoveSourceImage(item.id);
                      }}
                      className="rounded-md p-1 text-stone-400 transition hover:bg-stone-100 hover:text-rose-500"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                  <Zoom>
                    <Image src={buildSourceImageUrl(item)} alt={item.name} width={160} height={110} unoptimized className="block h-16 w-full cursor-zoom-in bg-stone-50 object-contain sm:h-20" />
                  </Zoom>
                </div>
              ))}
            </div>
          ) : null}

          <div className="relative px-3 pb-1.5 pt-2.5 sm:px-4 sm:pb-2 sm:pt-3">
            <Textarea
              ref={textareaRef}
              value={imagePrompt}
              onChange={(event) => onPromptChange(event.target.value)}
              placeholder={mode === "generate" ? "描述你想生成的画面，也可以先上传参考图" : mode === "edit" ? "描述你想如何修改当前图片" : "可选：描述你想增强的方向"}
              onPaste={onPromptPaste}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  if (!isSubmitting) {
                    void onSubmit();
                  }
                }
              }}
              className={cn(
                "resize-none border-0 bg-transparent !px-1 !pb-1 text-[14px] text-stone-900 shadow-none placeholder:text-stone-400 focus-visible:ring-0 sm:min-h-[92px] sm:max-h-[480px] sm:text-[15px] sm:leading-7",
                isMobileComposerCollapsed ? "min-h-[24px] max-h-[24px] overflow-hidden !pt-0.5 !pb-0.5 pr-9 leading-6" : "min-h-[72px] max-h-[180px] overflow-y-auto !pt-1 pr-10 leading-6",
              )}
            />
          </div>
          <div className={cn("px-3 pb-2 pt-1.5 sm:px-4 sm:pb-4 sm:pt-2", showMobileExpandedSections ? "block" : "hidden lg:block")}>
            <div className="flex items-end justify-between gap-3">
              <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 rounded-full border-stone-200 bg-white px-2 text-[11px] font-medium text-stone-700 shadow-none sm:h-8 sm:px-2.5 sm:text-xs"
                  onClick={(event) => {
                    event.stopPropagation();
                    uploadInputRef.current?.click();
                  }}
                >
                  <ImagePlus className="size-3.5" />
                  {mode === "generate" ? "上传参考图" : mode === "edit" ? "上传源图" : "上传图片"}
                </Button>

                {mode === "edit" ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-7 rounded-full border-stone-200 bg-white px-2 text-[11px] font-medium text-stone-700 shadow-none sm:h-8 sm:px-2.5 sm:text-xs"
                    onClick={(event) => {
                      event.stopPropagation();
                      maskInputRef.current?.click();
                    }}
                  >
                    <Upload className="size-3.5" />
                    遮罩
                  </Button>
                ) : null}
              </div>

              <button
                type="button"
                onClick={() => void onSubmit()}
                disabled={isSubmitting}
                className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-stone-950 text-white transition hover:bg-stone-800 disabled:cursor-not-allowed disabled:bg-stone-300 sm:size-9"
                aria-label="提交图片任务"
              >
                {isSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : <ArrowUp className="size-4" />}
              </button>
            </div>
          </div>

          <input
            ref={uploadInputRef}
            type="file"
            accept="image/*"
            multiple={mode !== "upscale"}
            className="hidden"
            onChange={(event) => {
              void onAppendFiles(event.target.files, "image");
              event.currentTarget.value = "";
            }}
          />
          <input
            ref={maskInputRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(event) => {
              void onAppendFiles(event.target.files, "mask");
              event.currentTarget.value = "";
            }}
          />
        </div>
      </div>
    </div>
  );
}

export default function ImagePage() {
  const mountedRef = useRef(true);
  const didLoadBootstrapRef = useRef(false);
  const resultsViewportRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const uploadInputRef = useRef<HTMLInputElement | null>(null);
  const maskInputRef = useRef<HTMLInputElement | null>(null);

  const [mode, setMode] = useState<ImageMode>("generate");
  const [imagePrompt, setImagePrompt] = useState("");
  const [imageCount, setImageCount] = useState("1");
  const [imageSize, setImageSize] = useState(sizeOptions[0].value);
  const [imageQuality, setImageQuality] = useState<ImageQuality>("high");
  const [upscaleScale, setUpscaleScale] = useState(upscaleOptions[0]);
  const [sourceImages, setSourceImages] = useState<StoredSourceImage[]>([]);
  const [conversations, setConversations] = useState<ImageConversation[]>([]);
  const [selectedConversationId, setSelectedConversationId] = useState<string | null>(null);
  const [isLoadingHistory, setIsLoadingHistory] = useState(true);
  const [availableAccounts, setAvailableAccounts] = useState<Account[]>([]);
  const [availableQuota, setAvailableQuota] = useState("加载中");
  const [publishedRecords, setPublishedRecords] = useState<Record<string, { work_id: string; published_at: string; key: string }>>({});
  const [publishingImageKey, setPublishingImageKey] = useState<string | null>(null);
  const [workspaceConfig, setWorkspaceConfig] = useState<WorkspaceConfig>({
    allowDisabledStudioAccounts: false,
  });
  const [historyCollapsed, setHistoryCollapsed] = useState(false);
  const [activeConversationIds, setActiveConversationIds] = useState<Set<string>>(() => new Set());
  const [activeRequest, setActiveRequest] = useState<ActiveRequestState | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitStartedAt, setSubmitStartedAt] = useState<number | null>(null);
  const [submitElapsedSeconds, setSubmitElapsedSeconds] = useState(0);
  const [isMobileComposerCollapsed, setIsMobileComposerCollapsed] = useState(true);

  const parsedCount = useMemo(() => Math.max(1, Math.min(4, Number(imageCount) || 1)), [imageCount]);
  const selectedConversation = useMemo(
    () => conversations.find((item) => item.id === selectedConversationId) ?? null,
    [conversations, selectedConversationId],
  );
  const selectedConversationTurns = useMemo(
    () => selectedConversation?.turns ?? [],
    [selectedConversation],
  );
  const publishedImageKeys = useMemo(
    () => new Set(Object.keys(publishedRecords)),
    [publishedRecords],
  );
  const imageSources = useMemo(
    () => sourceImages.filter((item) => item.role === "image"),
    [sourceImages],
  );
  const hasGenerateReferences = mode === "generate" && imageSources.length > 0;
  const processingStatus = useMemo(
    () => (activeRequest ? buildProcessingStatus(activeRequest.mode, submitElapsedSeconds, activeRequest.count) : null),
    [activeRequest, submitElapsedSeconds],
  );
  const waitingDots = useMemo(() => buildWaitingDots(submitElapsedSeconds), [submitElapsedSeconds]);

  const syncRuntimeTaskState = useCallback((preferredConversationId?: string | null) => {
    const tasks = listActiveImageTasks();
    const nextTask =
      tasks.find((task) => preferredConversationId && task.conversationId === preferredConversationId) ??
      tasks[0] ??
      null;

    setIsSubmitting(tasks.length > 0);
    setActiveConversationIds(new Set(tasks.map((task) => task.conversationId)));
    setActiveRequest(
      nextTask
        ? {
            conversationId: nextTask.conversationId,
            turnId: nextTask.turnId,
            mode: nextTask.mode,
            count: nextTask.count,
            variant: nextTask.variant,
          }
        : null,
    );
    setSubmitStartedAt(nextTask?.startedAt ?? null);
    if (!nextTask) {
      setSubmitElapsedSeconds(0);
    }
  }, []);

  const persistConversation = useCallback(
    async (conversation: ImageConversation) => {
      const normalizedConversation = normalizeConversation(conversation);
      await saveImageConversation(normalizedConversation);
      if (!mountedRef.current) {
        return normalizedConversation;
      }

      setConversations((current) =>
        sortConversations([normalizedConversation, ...current.filter((item) => item.id !== normalizedConversation.id)]),
      );
      setSelectedConversationId(normalizedConversation.id);
      syncRuntimeTaskState(normalizedConversation.id);
      return normalizedConversation;
    },
    [syncRuntimeTaskState],
  );

  const updateConversation = useCallback(
    async (
      conversationId: string,
      updater: (current: ImageConversation | null) => ImageConversation,
    ) => {
      const nextConversation = await updateImageConversation(conversationId, updater);
      if (!mountedRef.current) {
        return nextConversation;
      }

      setConversations((current) =>
        sortConversations([nextConversation, ...current.filter((item) => item.id !== conversationId)]),
      );
      return nextConversation;
    },
    [],
  );

  const resetComposer = useCallback((nextMode: ImageMode = "generate") => {
    setMode(nextMode);
    setImagePrompt("");
    setImageCount("1");
    setImageSize(sizeOptions[0].value);
    setImageQuality("high");
    setUpscaleScale(upscaleOptions[0]);
    setSourceImages([]);
  }, []);

  const syncQuotaAfterResult = useCallback(
    (images: StoredImage[]) => {
      const sourceAccountId =
        images.find((item) => item.status === "success" && item.source_account_id)?.source_account_id ??
        images.find((item) => item.source_account_id)?.source_account_id;
      if (!sourceAccountId) {
        return;
      }

      void (async () => {
        try {
          const quota = await fetchAccountQuota(sourceAccountId, { refresh: false });
          if (!mountedRef.current) {
            return;
          }

          setAvailableAccounts((current) => {
            const next = current.map((account) =>
              account.id === sourceAccountId ? applyQuotaResultToAccount(account, quota) : account,
            );
            setAvailableQuota(formatAvailableQuota(next, workspaceConfig.allowDisabledStudioAccounts));
            return next;
          });
        } catch {
          // Best-effort UI sync only.
        }
      })();
    },
    [workspaceConfig.allowDisabledStudioAccounts],
  );

  const buildDraftTurn = useCallback(
    ({
      turnId,
      nextMode,
      prompt,
      count,
      createdAt,
      nextSourceImages,
      nextSize,
      nextQuality,
      nextScale,
    }: {
      turnId: string;
      nextMode: ImageMode;
      prompt: string;
      count: number;
      createdAt: string;
      nextSourceImages: StoredSourceImage[];
      nextSize?: string;
      nextQuality?: ImageQuality;
      nextScale?: string;
    }): ImageConversationTurn => ({
      id: turnId,
      title: buildConversationTitle(nextMode, prompt, nextScale || ""),
      mode: nextMode,
      prompt,
      model: "gpt-image-2",
      count,
      size: nextSize,
      quality: nextQuality,
      scale: nextScale,
      sourceImages: nextSourceImages,
      images: createLoadingImages(count, turnId),
      createdAt,
      status: "generating",
    }),
    [],
  );

  const buildSourceFile = useCallback(async (source: StoredSourceImage, fallbackName: string) => {
    const sourceUrl = buildSourceImageUrl(source);
    if (!sourceUrl) {
      throw new Error("源图数据不可用");
    }
    return dataUrlToFile(sourceUrl, source.name || fallbackName);
  }, []);

  const runTurnRequest = useCallback(
    async (turn: ImageConversationTurn) => {
      const turnPrompt = turn.prompt.trim();
      const turnSourceImages = Array.isArray(turn.sourceImages) ? turn.sourceImages : [];
      const turnImageSources = turnSourceImages.filter((item) => item.role === "image");
      const turnMaskSource = turnSourceImages.find((item) => item.role === "mask") ?? null;

      if (turn.mode === "generate") {
        if (turnImageSources.length > 0) {
          const files = await Promise.all(
            turnImageSources.map((item, index) => buildSourceFile(item, `reference-${index + 1}.png`)),
          );
          const payload = await editImage({
            prompt: turnPrompt,
            images: files,
            model: turn.model,
          });
          return mergeResultImages(turn.id, payload.data || [], 1);
        }

        const payload = await generateImageWithOptions(turnPrompt, {
          model: turn.model,
          count: Math.max(1, turn.count || 1),
          size: turn.size,
          quality: turn.quality || "high",
        });
        return mergeResultImages(turn.id, payload.data || [], Math.max(1, turn.count || 1));
      }

      if (turn.mode === "edit") {
        if (turnImageSources.length === 0) {
          throw new Error("编辑模式至少需要一张源图");
        }

        const files = await Promise.all(
          turnImageSources.map((item, index) => buildSourceFile(item, `image-${index + 1}.png`)),
        );
        const mask = turnMaskSource ? await buildSourceFile(turnMaskSource, "mask.png") : null;
        const payload = await editImage({
          prompt: turnPrompt,
          images: files,
          mask,
          model: turn.model,
        });
        return mergeResultImages(turn.id, payload.data || [], 1);
      }

      if (turnImageSources.length === 0) {
        throw new Error("放大模式至少需要一张源图");
      }

      const imageFile = await buildSourceFile(turnImageSources[0], "upscale.png");
      const payload = await upscaleImage({
        image: imageFile,
        prompt: turnPrompt,
        scale: turn.scale || "2x",
        model: turn.model,
      });
      return mergeResultImages(turn.id, payload.data || [], 1);
    },
    [buildSourceFile],
  );

  const finalizeTurn = useCallback(
    async (conversationId: string, draftTurn: ImageConversationTurn, resultImages: StoredImage[]) => {
      const failedCount = countFailures(resultImages);
      await updateConversation(conversationId, (current) => ({
        ...(current ?? buildConversationBase(conversationId, draftTurn)),
        turns: (current?.turns ?? [draftTurn]).map((turn) =>
          turn.id === draftTurn.id
            ? {
                ...turn,
                images: resultImages,
                status: failedCount > 0 ? "error" : "success",
                error: failedCount > 0 ? `其中 ${failedCount} 张处理失败` : undefined,
              }
            : turn,
        ),
      }));
      return failedCount;
    },
    [updateConversation],
  );

  const markTurnFailed = useCallback(
    async (conversationId: string, draftTurn: ImageConversationTurn, message: string) => {
      await updateConversation(conversationId, (current) => ({
        ...(current ?? buildConversationBase(conversationId, draftTurn)),
        turns: (current?.turns ?? [draftTurn]).map((turn) =>
          turn.id === draftTurn.id
            ? {
                ...turn,
                status: "error",
                error: message,
                images: turn.images.map((image) => ({
                  ...image,
                  status: "error" as const,
                  error: message,
                })),
              }
            : turn,
        ),
      }));
    },
    [updateConversation],
  );

  const handleModeChange = useCallback((nextMode: ImageMode) => {
    setMode(nextMode);
    setSourceImages((current) => {
      const currentImages = current.filter((item) => item.role === "image");
      const currentMask = current.find((item) => item.role === "mask") ?? null;
      if (nextMode === "generate") {
        return currentImages;
      }
      if (nextMode === "edit") {
        return currentMask ? [...currentImages, currentMask] : currentImages;
      }
      return currentImages.length > 0 ? [currentImages[0]] : [];
    });
  }, []);

  const handleAppendFiles = useCallback(
    async (files: FileList | null, role: "image" | "mask") => {
      if (!files || files.length === 0) {
        return;
      }

      try {
        const nextItems = await Promise.all(
          Array.from(files).map(async (file) => ({
            id: makeId(),
            role,
            name: file.name,
            dataUrl: await fileToDataUrl(file),
          })),
        );

        if (!mountedRef.current) {
          return;
        }

        if (mode === "upscale" && role === "image" && nextItems.length > 1) {
          toast.warning("放大模式只会保留第一张图片");
        }

        setSourceImages((current) => {
          const currentImages = current.filter((item) => item.role === "image");
          const currentMask = current.find((item) => item.role === "mask") ?? null;

          if (role === "mask") {
            const latestMask = nextItems[nextItems.length - 1];
            return [...currentImages, latestMask];
          }

          const mergedImages =
            mode === "upscale" ? nextItems.slice(0, 1) : [...currentImages, ...nextItems];
          if (mode === "edit" && currentMask) {
            return [...mergedImages, currentMask];
          }
          return mergedImages;
        });
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "读取图片失败");
      }
    },
    [mode],
  );

  const handlePromptPaste = useCallback(
    async (event: ReactClipboardEvent<HTMLTextAreaElement>) => {
      const imageFiles = Array.from(event.clipboardData.items)
        .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
        .map((item) => item.getAsFile())
        .filter((file): file is File => Boolean(file));

      if (imageFiles.length === 0) {
        return;
      }

      event.preventDefault();
      const transfer = new DataTransfer();
      imageFiles.forEach((file) => transfer.items.add(file));
      await handleAppendFiles(transfer.files, "image");
    },
    [handleAppendFiles],
  );

  const handleRemoveSourceImage = useCallback((id: string) => {
    setSourceImages((current) => current.filter((item) => item.id !== id));
  }, []);

  const handleCreateDraft = useCallback(() => {
    setSelectedConversationId(null);
    resetComposer("generate");
    window.requestAnimationFrame(() => {
      textareaRef.current?.focus();
    });
  }, [resetComposer]);

  const handleApplyPromptExample = useCallback((example: (typeof inspirationExamples)[number]) => {
    setSelectedConversationId(null);
    setMode("generate");
    setImagePrompt(example.prompt);
    setImageCount("1");
    setSourceImages([]);
    window.requestAnimationFrame(() => {
      textareaRef.current?.focus();
    });
  }, []);

  const handleFocusConversation = useCallback((id: string) => {
    setSelectedConversationId(id);
  }, []);

  const handleDeleteConversation = useCallback(
    async (conversationId: string) => {
      await deleteImageConversation(conversationId);
      if (!mountedRef.current) {
        return;
      }

      setConversations((current) => current.filter((item) => item.id !== conversationId));
      if (selectedConversationId === conversationId) {
        const nextConversation = conversations.find((item) => item.id !== conversationId) ?? null;
        setSelectedConversationId(nextConversation?.id ?? null);
      }
    },
    [conversations, selectedConversationId],
  );

  const handleClearHistory = useCallback(async () => {
    await Promise.all(conversations.map((item) => deleteImageConversation(item.id)));
    if (!mountedRef.current) {
      return;
    }

    setConversations([]);
    setSelectedConversationId(null);
  }, [conversations]);

  const handleSeedFromResult = useCallback((conversationId: string, image: StoredImage, nextMode: ImageMode) => {
    const sourceUrl = buildImageDataUrl(image);
    if (!sourceUrl) {
      toast.error("当前结果图不可用");
      return;
    }

    setSelectedConversationId(conversationId);
    setMode(nextMode);
    setImagePrompt("");
    setImageCount("1");
    setSourceImages([
      {
        id: makeId(),
        role: "image",
        name: nextMode === "upscale" ? "upscale-source.png" : "edit-source.png",
        dataUrl: image.b64_json ? sourceUrl : undefined,
        url: image.url && !image.b64_json ? image.url : undefined,
      },
    ]);
    window.requestAnimationFrame(() => {
      textareaRef.current?.focus();
    });
  }, []);

  const handlePublishTurnImage = useCallback(
    async (conversationId: string, turn: ImageConversationTurn, image: StoredImage) => {
      const imageDataUrl = buildImageDataUrl(image);
      if (!imageDataUrl) {
        toast.error("当前结果图不可用");
        return;
      }

      const recordKey = buildGalleryPublishRecordKey(conversationId, turn.id, image.id);
      if (publishedRecords[recordKey]) {
        toast.message("这张图片已经发布过了");
        return;
      }

      setPublishingImageKey(recordKey);
      try {
        const prompt = buildGalleryWorkPrompt(turn);
        const payload = await publishPortalGalleryWork({
          title: buildGalleryWorkTitle(turn),
          prompt,
          image_data_url: imageDataUrl.startsWith("data:") ? imageDataUrl : undefined,
          image_url: imageDataUrl.startsWith("data:") ? undefined : imageDataUrl,
          model: turn.model,
          size: turn.size,
        });

        const record = {
          key: recordKey,
          work_id: payload.item.id,
          published_at: new Date().toISOString(),
        };
        await saveGalleryPublishRecord(record);
        if (!mountedRef.current) {
          return;
        }

        setPublishedRecords((current) => ({
          ...current,
          [recordKey]: record,
        }));
        toast.success("已发布到作品广场");
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "发布失败");
      } finally {
        if (mountedRef.current) {
          setPublishingImageKey(null);
        }
      }
    },
    [publishedRecords],
  );

  const handleRetryTurn = useCallback(
    async (conversationId: string, turn: ImageConversationTurn) => {
      if (isSubmitting) {
        toast.error("正在处理中，请稍后再试");
        return;
      }

      const prompt = turn.prompt.trim();
      const turnMode = turn.mode || "generate";
      const turnSourceImages = Array.isArray(turn.sourceImages) ? turn.sourceImages : [];
      const turnImageSources = turnSourceImages.filter((item) => item.role === "image");
      const turnScale = turnMode === "upscale" ? turn.scale || "2x" : undefined;
      const expectedCount = Math.max(1, turn.count || 1);

      if (turnMode === "generate" && !prompt) {
        toast.error("该记录缺少提示词，无法重试");
        return;
      }
      if (turnMode === "edit" && (!prompt || turnImageSources.length === 0)) {
        toast.error("该记录缺少编辑所需信息，无法重试");
        return;
      }
      if (turnMode === "upscale" && turnImageSources.length === 0) {
        toast.error("该记录缺少待放大图片，无法重试");
        return;
      }

      const now = new Date().toISOString();
      const draftTurn = buildDraftTurn({
        turnId: turn.id,
        nextMode: turnMode,
        prompt,
        count: expectedCount,
        createdAt: now,
        nextSourceImages: turnSourceImages,
        nextSize: turn.size,
        nextQuality: turnMode === "generate" && turnImageSources.length === 0 ? turn.quality || "high" : undefined,
        nextScale: turnScale,
      });

      const startedAt = Date.now();
      setSelectedConversationId(conversationId);
      setIsSubmitting(true);
      setActiveRequest({
        conversationId,
        turnId: turn.id,
        mode: turnMode,
        count: expectedCount,
        variant: "standard",
      });
      setSubmitElapsedSeconds(0);
      setSubmitStartedAt(startedAt);
      startImageTask({
        conversationId,
        turnId: turn.id,
        mode: turnMode,
        count: expectedCount,
        variant: "standard",
        startedAt,
      });

      try {
        await updateConversation(conversationId, (current) => {
          const currentTurns = current?.turns ?? [];
          const nextTurns = currentTurns.some((item) => item.id === turn.id)
            ? currentTurns.map((item) => (item.id === turn.id ? draftTurn : item))
            : [...currentTurns, draftTurn];
          return {
            ...(current ?? buildConversationBase(conversationId, draftTurn)),
            turns: nextTurns,
          };
        });

        const resultImages = await runTurnRequest(draftTurn);
        syncQuotaAfterResult(resultImages);
        const failedCount = await finalizeTurn(conversationId, draftTurn, resultImages);

        if (failedCount > 0) {
          toast.error(`已返回结果，但有 ${failedCount} 张处理失败`);
        } else {
          toast.success(
            turnMode === "generate" ? "图片已生成" : turnMode === "edit" ? "图片已编辑" : "图片已放大",
          );
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : "图片任务失败";
        await markTurnFailed(conversationId, draftTurn, message);
        toast.error(message);
      } finally {
        finishImageTask(conversationId, turn.id);
        setIsSubmitting(false);
        setActiveRequest(null);
        setSubmitStartedAt(null);
        setSubmitElapsedSeconds(0);
      }
    },
    [buildDraftTurn, finalizeTurn, isSubmitting, markTurnFailed, runTurnRequest, syncQuotaAfterResult, updateConversation],
  );

  const handleSubmit = useCallback(async () => {
    const prompt = imagePrompt.trim();
    if (!prompt && mode !== "upscale") {
      toast.error("请输入提示词");
      return;
    }
    if (mode === "edit" && imageSources.length === 0) {
      toast.error("编辑模式至少需要一张源图");
      return;
    }
    if (mode === "upscale" && imageSources.length === 0) {
      toast.error("放大模式至少需要一张源图");
      return;
    }

    const conversationId = selectedConversationId ?? makeId();
    const turnId = makeId();
    const expectedCount = mode === "generate" && imageSources.length === 0 ? parsedCount : 1;
    const now = new Date().toISOString();
    const draftTurn = buildDraftTurn({
      turnId,
      nextMode: mode,
      prompt,
      count: expectedCount,
      createdAt: now,
      nextSourceImages: sourceImages,
      nextSize: mode === "generate" ? imageSize : undefined,
      nextQuality: mode === "generate" && imageSources.length === 0 ? imageQuality : undefined,
      nextScale: mode === "upscale" ? upscaleScale : undefined,
    });

    const startedAt = Date.now();
    setSelectedConversationId(conversationId);
    setIsSubmitting(true);
    setActiveRequest({
      conversationId,
      turnId,
      mode,
      count: expectedCount,
      variant: "standard",
    });
    setSubmitElapsedSeconds(0);
    setSubmitStartedAt(startedAt);
    setImagePrompt("");
    setSourceImages([]);
    startImageTask({
      conversationId,
      turnId,
      mode,
      count: expectedCount,
      variant: "standard",
      startedAt,
    });

    try {
      if (selectedConversationId) {
        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns: [...(current?.turns ?? []), draftTurn],
        }));
      } else {
        await persistConversation(buildConversationBase(conversationId, draftTurn));
      }

      const resultImages = await runTurnRequest(draftTurn);
      syncQuotaAfterResult(resultImages);
      const failedCount = await finalizeTurn(conversationId, draftTurn, resultImages);

      resetComposer(mode);
      if (failedCount > 0) {
        toast.error(`已返回结果，但有 ${failedCount} 张处理失败`);
      } else {
        toast.success(
          mode === "generate"
            ? hasGenerateReferences
              ? "参考图生成已完成"
              : "图片已生成"
            : mode === "edit"
              ? "图片已编辑"
              : "图片已放大",
        );
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "图片任务失败";
      await markTurnFailed(conversationId, draftTurn, message);
      toast.error(message);
    } finally {
      finishImageTask(conversationId, turnId);
      setIsSubmitting(false);
      setActiveRequest(null);
      setSubmitStartedAt(null);
      setSubmitElapsedSeconds(0);
    }
  }, [
    buildDraftTurn,
    finalizeTurn,
    hasGenerateReferences,
    imagePrompt,
    imageQuality,
    imageSize,
    imageSources.length,
    markTurnFailed,
    mode,
    parsedCount,
    persistConversation,
    resetComposer,
    runTurnRequest,
    selectedConversationId,
    sourceImages,
    syncQuotaAfterResult,
    upscaleScale,
    updateConversation,
  ]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (didLoadBootstrapRef.current) {
      return;
    }
    didLoadBootstrapRef.current = true;

    const loadWorkspace = async () => {
      try {
        const [historyItems, bootstrap, publishRecords] = await Promise.all([
          listImageConversations(),
          fetchPortalWorkspaceBootstrap(),
          listGalleryPublishRecords(),
        ]);
        const normalizedHistory = await normalizeConversationHistory(historyItems);
        if (!mountedRef.current) {
          return;
        }

        const allowDisabledStudioAccounts = bootstrap.workspace.allow_disabled_studio_accounts;
        setConversations(normalizedHistory);
        setSelectedConversationId(normalizedHistory[0]?.id ?? null);
        setAvailableAccounts(bootstrap.accounts);
        setWorkspaceConfig({ allowDisabledStudioAccounts });
        setAvailableQuota(formatAvailableQuota(bootstrap.accounts, allowDisabledStudioAccounts));
        setPublishedRecords(publishRecords);
        syncRuntimeTaskState(normalizedHistory[0]?.id ?? null);
      } catch (error) {
        setAvailableQuota("—");
        toast.error(error instanceof Error ? error.message : "初始化图片工作台失败");
      } finally {
        if (mountedRef.current) {
          setIsLoadingHistory(false);
        }
      }
    };

    void loadWorkspace();
  }, [syncRuntimeTaskState]);

  useEffect(() => {
    syncRuntimeTaskState(selectedConversationId);
    const unsubscribe = () => {
      syncRuntimeTaskState(selectedConversationId);
    };
    const stop = subscribeImageTasks(unsubscribe);
    return () => {
      stop();
    };
  }, [selectedConversationId, syncRuntimeTaskState]);

  useEffect(() => {
    if (!isSubmitting || submitStartedAt === null) {
      return;
    }

    const updateElapsed = () => {
      setSubmitElapsedSeconds(Math.max(0, Math.floor((Date.now() - submitStartedAt) / 1000)));
    };

    updateElapsed();
    const timer = window.setInterval(updateElapsed, 1000);
    return () => {
      window.clearInterval(timer);
    };
  }, [isSubmitting, submitStartedAt]);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) {
      return;
    }

    textarea.style.height = "auto";
    const maxHeight = Math.min(480, Math.max(180, Math.floor(window.innerHeight * 0.42)));
    textarea.style.height = `${Math.min(textarea.scrollHeight, maxHeight)}px`;
  }, [imagePrompt, mode]);

  useEffect(() => {
    const viewport = resultsViewportRef.current;
    if (!viewport) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      viewport.scrollTo({
        top: viewport.scrollHeight,
        behavior: "smooth",
      });
    });

    return () => {
      window.cancelAnimationFrame(frame);
    };
  }, [activeRequest?.turnId, selectedConversationId, selectedConversationTurns.length]);

  return (
    <section
      className={cn(
        "grid grid-cols-1 gap-3 lg:h-full lg:min-h-0",
        historyCollapsed ? "lg:grid-cols-[minmax(0,1fr)]" : "lg:grid-cols-[320px_minmax(0,1fr)]",
      )}
    >
      {!historyCollapsed ? (
        <HistorySidebar
          conversations={conversations}
          selectedConversationId={selectedConversationId}
          isLoadingHistory={isLoadingHistory}
          hasActiveTasks={isSubmitting}
          activeConversationIds={activeConversationIds}
          onCreateDraft={handleCreateDraft}
          onClearHistory={handleClearHistory}
          onFocusConversation={handleFocusConversation}
          onDeleteConversation={handleDeleteConversation}
        />
      ) : null}

      <div className="order-1 flex flex-col overflow-visible rounded-[30px] border border-stone-200 bg-white shadow-[0_14px_40px_rgba(15,23,42,0.05)] lg:order-none lg:min-h-0 lg:overflow-hidden">
        <WorkspaceHeader
          historyCollapsed={historyCollapsed}
          selectedConversationTitle={selectedConversation?.title ?? null}
          onToggleHistory={() => setHistoryCollapsed((current) => !current)}
        />

        <div className="relative min-h-[240px] bg-[#fcfcfb] lg:min-h-0 lg:flex-1">
          <div
            ref={resultsViewportRef}
            className={cn(
              "hide-scrollbar min-h-[240px] overflow-visible lg:h-full lg:min-h-0 lg:overflow-y-auto lg:pb-0",
              isMobileComposerCollapsed ? "pb-[88px] sm:pb-[96px]" : "pb-[320px] sm:pb-[340px]",
            )}
          >
            {!selectedConversation ? (
              <EmptyState onApplyPromptExample={handleApplyPromptExample} />
            ) : (
              <ConversationTurns
                conversationId={selectedConversation.id}
                turns={selectedConversationTurns}
                activeRequest={activeRequest}
                isSubmitting={isSubmitting}
                processingStatus={processingStatus}
                waitingDots={waitingDots}
                submitElapsedSeconds={submitElapsedSeconds}
                publishedImageKeys={publishedImageKeys}
                publishingImageKey={publishingImageKey}
                onSeedFromResult={handleSeedFromResult}
                onPublishImage={handlePublishTurnImage}
                onRetryTurn={handleRetryTurn}
              />
            )}
          </div>
        </div>

        <PromptComposer
          mode={mode}
          imageCount={imageCount}
          imageSize={imageSize}
          imageQuality={imageQuality}
          upscaleScale={upscaleScale}
          availableQuota={availableQuota}
          sourceImages={sourceImages}
          imagePrompt={imagePrompt}
          hasGenerateReferences={hasGenerateReferences}
          isSubmitting={isSubmitting}
          textareaRef={textareaRef}
          uploadInputRef={uploadInputRef}
          maskInputRef={maskInputRef}
          onModeChange={handleModeChange}
          onImageCountChange={setImageCount}
          onImageSizeChange={setImageSize}
          onImageQualityChange={setImageQuality}
          onUpscaleScaleChange={setUpscaleScale}
          onPromptChange={setImagePrompt}
          onPromptPaste={handlePromptPaste}
          onRemoveSourceImage={handleRemoveSourceImage}
          onAppendFiles={handleAppendFiles}
          onMobileCollapsedChange={setIsMobileComposerCollapsed}
          onSubmit={handleSubmit}
        />
      </div>
    </section>
  );
}
