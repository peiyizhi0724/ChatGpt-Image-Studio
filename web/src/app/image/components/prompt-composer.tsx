"use client";

import { useEffect, useRef, useState, type ClipboardEvent as ReactClipboardEvent, type ReactNode, type RefObject } from "react";
import Zoom from "react-medium-image-zoom";
import { ArrowUp, Brush, ChevronDown, CircleHelp, ImagePlus, Trash2 } from "lucide-react";

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
import type { ImageQuality } from "@/lib/api";
import type { ImageMode, StoredSourceImage } from "@/store/image-conversations";
import { cn } from "@/lib/utils";
import { buildSourceImageUrl } from "../view-utils";

type PromptComposerProps = {
  mode: ImageMode;
  modeOptions: Array<{ label: string; value: ImageMode; description: string }>;
  imageCount: string;
  imageAspectRatio: string;
  imageAspectRatioOptions: Array<{ label: string; value: string }>;
  imageResolutionTier: string;
  imageResolutionTierLabel: string;
  imageResolutionTierOptions: Array<{ label: string; value: string; disabled?: boolean }>;
  imageSizeHint: ReactNode;
  imageQuality: ImageQuality;
  imageQualityOptions: Array<{ label: string; value: ImageQuality; description: string }>;
  imageQualityDisabled: boolean;
  imageQualityDisabledReason: string;
  availableQuota: string;
  sourceImages: StoredSourceImage[];
  imagePrompt: string;
  textareaRef: RefObject<HTMLTextAreaElement | null>;
  uploadInputRef: RefObject<HTMLInputElement | null>;
  maskInputRef: RefObject<HTMLInputElement | null>;
  onModeChange: (mode: ImageMode) => void;
  onImageCountChange: (value: string) => void;
  onImageAspectRatioChange: (value: string) => void;
  onImageResolutionTierChange: (value: string) => void;
  onImageQualityChange: (value: string) => void;
  onPromptChange: (value: string) => void;
  onPromptPaste: (event: ReactClipboardEvent<HTMLTextAreaElement>) => void;
  onRemoveSourceImage: (id: string) => void;
  onOpenSourceSelectionEditor: (sourceImageId: string) => void;
  onAppendFiles: (files: FileList | null, role: "image" | "mask") => Promise<void>;
  onMobileCollapsedChange?: (collapsed: boolean) => void;
  onSubmit: () => Promise<void>;
};

export function PromptComposer({
  mode,
  modeOptions,
  imageCount,
  imageAspectRatio,
  imageAspectRatioOptions,
  imageResolutionTier,
  imageResolutionTierLabel,
  imageResolutionTierOptions,
  imageSizeHint,
  imageQuality,
  imageQualityOptions,
  imageQualityDisabled,
  imageQualityDisabledReason,
  availableQuota,
  sourceImages,
  imagePrompt,
  textareaRef,
  uploadInputRef,
  maskInputRef,
  onModeChange,
  onImageCountChange,
  onImageAspectRatioChange,
  onImageResolutionTierChange,
  onImageQualityChange,
  onPromptChange,
  onPromptPaste,
  onRemoveSourceImage,
  onOpenSourceSelectionEditor,
  onAppendFiles,
  onMobileCollapsedChange,
  onSubmit,
}: PromptComposerProps) {
  const imageQualityLabel = imageQualityOptions.find((item) => item.value === imageQuality)?.label ?? imageQuality;
  const showImageOutputControls = mode === "edit" || mode === "generate";
  const sizeHintAriaLabel = mode === "edit" ? "查看编辑输出说明" : "查看分辨率说明";
  const imageQualityPrefix = mode === "edit" ? "输出质量" : "质量";
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
    onMobileCollapsedChange?.(isMobileComposerCollapsed);
  }, [isMobileComposerCollapsed, onMobileCollapsedChange]);

  const sizeHintTooltip =
    showImageOutputControls ? (
      <span className="group relative hidden shrink-0 items-center align-middle sm:inline-flex">
        <span
          tabIndex={0}
          className="inline-flex size-9 cursor-help items-center justify-center rounded-full border border-stone-200 bg-white text-stone-400 transition-colors hover:text-stone-700 focus-visible:text-stone-700 focus-visible:outline-none"
          aria-label={sizeHintAriaLabel}
        >
          <CircleHelp className="size-4" />
        </span>
        <span className="pointer-events-none absolute right-0 bottom-full z-30 mb-2 w-80 max-w-[calc(100vw-2rem)] rounded-2xl border border-stone-200 bg-white px-4 py-3 text-xs font-normal leading-6 text-stone-600 opacity-0 shadow-[0_18px_50px_-24px_rgba(15,23,42,0.35)] transition-all duration-200 group-hover:pointer-events-auto group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:translate-y-0 group-focus-within:opacity-100">
          {imageSizeHint}
        </span>
      </span>
    ) : null;

  return (
    <div
        className={cn(
        "fixed inset-x-0 bottom-0 z-30 px-3 backdrop-blur supports-[padding:max(0px)]:pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-4 lg:static lg:inset-auto lg:bottom-auto lg:z-20 lg:rounded-none lg:border-x-0 lg:border-b-0 lg:border-t lg:bg-white lg:px-5 lg:shadow-none dark:lg:border-[var(--studio-border)] dark:lg:bg-[var(--studio-panel-soft)]",
        isMobileComposerCollapsed
          ? "border-transparent bg-white/96 shadow-none dark:bg-[color:var(--studio-bg)]"
          : "rounded-[26px] border border-stone-200 bg-white/96 shadow-[0_18px_50px_-24px_rgba(15,23,42,0.35)] dark:border-[var(--studio-border)] dark:bg-[color:var(--studio-bg)] dark:shadow-[0_24px_70px_-30px_rgba(0,0,0,0.82)]",
        isMobileComposerCollapsed ? "py-1 sm:py-1.5" : "py-1 sm:py-1.5",
        "lg:border-stone-200 lg:bg-white lg:py-2 lg:shadow-none",
      )}
    >
      <div className="mx-auto flex w-full max-w-[1120px] flex-col gap-2.5 px-4 sm:px-6">
        <div
          className={cn(
            "flex-col gap-2.5 xl:flex-row xl:items-center xl:justify-between",
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
                        ? "bg-stone-950 text-white shadow-sm dark:bg-[var(--studio-accent-strong)] dark:text-[var(--studio-accent-foreground)]"
                        : "text-stone-600 hover:bg-stone-200 hover:text-stone-900 dark:text-[var(--studio-text)] dark:hover:bg-[var(--studio-panel-muted)] dark:hover:text-[var(--studio-text-strong)]",
                    )}
                  >
                    {item.label}
                  </button>
                ))}
              </div>
            </div>
            {isMobileComposerExpanded ? (
              <button
                type="button"
                className="inline-flex size-8 shrink-0 items-center justify-center rounded-full text-stone-400 transition hover:bg-stone-100 hover:text-stone-700 sm:hidden"
                onClick={(event) => {
                  event.stopPropagation();
                  setIsMobileComposerExpanded(false);
                  textareaRef.current?.blur();
                }}
                aria-label="收起输入框"
                title="收起输入框"
              >
                <ChevronDown className="size-4" />
              </button>
            ) : null}
          </div>

          <div className="hide-scrollbar -mx-1 flex items-center gap-1.5 overflow-x-auto px-1 pb-1 sm:mx-0 sm:flex-wrap sm:gap-2 sm:overflow-visible sm:px-0 sm:pb-0">
            {showImageOutputControls ? (
              <Select value={imageAspectRatio} onValueChange={onImageAspectRatioChange}>
                <SelectTrigger className="h-9 w-[84px] shrink-0 rounded-full border-stone-200 bg-white text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-10 sm:w-[108px] sm:text-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {imageAspectRatioOptions.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}

            {showImageOutputControls ? (
              <Select value={imageResolutionTier} onValueChange={onImageResolutionTierChange}>
                <SelectTrigger
                  className="h-9 w-[168px] shrink-0 rounded-full border-stone-200 bg-white text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-10 sm:w-[238px] sm:text-sm"
                  title={imageResolutionTierLabel}
                >
                  <SelectValue>{imageResolutionTierLabel}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {imageResolutionTierOptions.map((item) => (
                    <SelectItem key={item.value} value={item.value} disabled={item.disabled}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}

            {sizeHintTooltip}

            {showImageOutputControls ? (
              <Select value={imageQuality} onValueChange={onImageQualityChange} disabled={imageQualityDisabled}>
                <SelectTrigger
                  className={cn(
                    "h-10 w-[136px] shrink-0 rounded-full border-stone-200 bg-white text-sm font-medium text-stone-700 shadow-none focus-visible:ring-0",
                    "h-9 w-[108px] text-[13px] sm:h-10 sm:w-[136px] sm:text-sm",
                    imageQualityDisabled && "cursor-not-allowed bg-stone-50 text-stone-400 opacity-80",
                  )}
                  title={
                    imageQualityDisabled
                      ? imageQualityDisabledReason
                      : imageQualityOptions.find((item) => item.value === imageQuality)?.description
                  }
                >
                  <SelectValue>{`${imageQualityPrefix} ${imageQualityLabel}`}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {imageQualityOptions.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      <span title={item.description}>{imageQualityPrefix} {item.label}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}

            {mode === "generate" ? (
              <div className="flex shrink-0 items-center gap-1 rounded-full border border-stone-200 bg-white px-2 py-0.5 sm:gap-1.5 sm:px-2.5 sm:py-1">
                <span className="text-[13px] font-medium text-stone-700 sm:text-sm">张数</span>
                <Input
                  type="number"
                  min="1"
                  max="8"
                  step="1"
                  value={imageCount}
                  onChange={(event) => onImageCountChange(event.target.value)}
                  className="h-7 w-[36px] border-0 bg-transparent px-0 text-center text-[13px] font-medium text-stone-700 shadow-none focus-visible:ring-0 sm:h-8 sm:w-[42px] sm:text-sm"
                />
              </div>
            ) : null}

            <span className="shrink-0 rounded-full bg-stone-100 px-2.5 py-1.5 text-[11px] font-medium text-stone-600 sm:px-3 sm:py-2 sm:text-xs">
              剩余额度 {availableQuota}
            </span>
          </div>
        </div>

          <div
          className="overflow-hidden rounded-[24px] border border-stone-200 bg-[#fafaf9] shadow-[inset_0_1px_0_rgba(255,255,255,0.9)] transition-colors duration-200 dark:border-[var(--studio-border)] dark:bg-[var(--studio-panel)] dark:shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] sm:rounded-[28px]"
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
                <div
                  key={item.id}
                  className="w-[104px] shrink-0 overflow-hidden rounded-[16px] border border-stone-200 bg-white sm:w-[126px] sm:rounded-[18px]"
                >
                  <div className="flex items-center justify-between border-b border-stone-100 px-3 py-2 text-[11px] font-medium text-stone-500">
                    <span>{item.role === "mask" ? "遮罩" : "源图"}</span>
                    <div className="flex items-center gap-1">
                      {mode === "edit" && item.role === "image" ? (
                        <button
                          type="button"
                          onClick={(event) => {
                            event.stopPropagation();
                            onOpenSourceSelectionEditor(item.id);
                          }}
                          className="rounded-md p-1 text-stone-400 transition hover:bg-stone-100 hover:text-stone-700"
                          title="选区编辑"
                          aria-label="选区编辑"
                        >
                          <Brush className="size-3.5" />
                        </button>
                      ) : null}
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
                  </div>
                  <Zoom>
                    <Image
                      src={buildSourceImageUrl(item)}
                      alt={item.name}
                      width={160}
                      height={110}
                      unoptimized
                      className="block h-16 w-full cursor-zoom-in bg-stone-50 object-contain sm:h-20"
                    />
                  </Zoom>
                </div>
              ))}
            </div>
          ) : null}

          <div className="relative px-3 pb-1.5 pt-2 sm:px-4 sm:pb-2 sm:pt-2.5">
            {isMobileComposerCollapsed ? (
              <>
                <button
                  type="button"
                  className="flex min-h-[22px] w-full items-center px-1 py-0 text-left text-[14px] leading-5 text-stone-400 sm:hidden"
                  onClick={() => {
                    setIsMobileComposerExpanded(true);
                    textareaRef.current?.focus();
                  }}
                >
                  <span className="block w-full truncate">
                    {imagePrompt.trim() ||
                      (mode === "generate"
                        ? "描述你想生成的画面，也可以先上传参考图"
                        : "描述你想如何修改当前图片")}
                  </span>
                </button>
                <Textarea
                  ref={textareaRef}
                  value={imagePrompt}
                  onChange={(event) => onPromptChange(event.target.value)}
                  placeholder={
                    mode === "generate"
                      ? "描述你想生成的画面，也可以先上传参考图"
                      : mode === "edit"
                        ? "描述你想如何修改当前图片"
                        : "可选：描述你想增强的方向"
                  }
                  onPaste={onPromptPaste}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey) {
                      event.preventDefault();
                      void onSubmit();
                    }
                  }}
                  className="hidden resize-none border-0 bg-transparent !px-1 !pb-1 text-[14px] text-stone-900 shadow-none placeholder:text-stone-400 focus-visible:ring-0 sm:block sm:min-h-[38px] sm:max-h-[70px] sm:overflow-y-auto sm:!pt-1 sm:pr-10 sm:text-[15px] sm:leading-7"
                  onFocus={() => setIsMobileComposerExpanded(true)}
                />
              </>
            ) : (
              <Textarea
                ref={textareaRef}
                value={imagePrompt}
                onChange={(event) => onPromptChange(event.target.value)}
                placeholder={
                  mode === "generate"
                    ? "描述你想生成的画面，也可以先上传参考图"
                    : mode === "edit"
                      ? "描述你想如何修改当前图片"
                      : "可选：描述你想增强的方向"
                }
                onPaste={onPromptPaste}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    void onSubmit();
                  }
                }}
                className="resize-none border-0 bg-transparent !px-1 !pb-1 text-[14px] text-stone-900 shadow-none placeholder:text-stone-400 focus-visible:ring-0 min-h-[30px] max-h-[70px] overflow-y-auto !pt-1 pr-10 leading-6 sm:min-h-[38px] sm:text-[15px] sm:leading-7"
                onFocus={() => setIsMobileComposerExpanded(true)}
              />
            )}
          </div>
          <div className={cn("px-3 pb-1.5 pt-2.5 sm:px-4 sm:pb-2.5 sm:pt-2.5", showMobileExpandedSections ? "block" : "hidden lg:block")}>
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
                  {mode === "generate" ? "上传参考图" : "上传源图"}
                </Button>

              </div>

              <button
                type="button"
                onClick={() => void onSubmit()}
                className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-stone-950 text-white transition hover:bg-stone-800 disabled:cursor-not-allowed disabled:bg-stone-300 dark:bg-[var(--studio-accent-strong)] dark:text-[var(--studio-accent-foreground)] dark:hover:bg-[var(--studio-accent)] dark:disabled:bg-[var(--studio-panel-muted)] dark:disabled:text-[var(--studio-text-muted)] sm:size-9"
                aria-label="提交图片任务"
              >
                <ArrowUp className="size-4" />
              </button>
            </div>
          </div>

          <input
            ref={uploadInputRef}
            type="file"
            accept="image/*"
            multiple
            className="hidden"
            onChange={(event) => {
              void onAppendFiles(event.target.files, "image");
              event.currentTarget.value = "";
            }}
          />
        </div>
      </div>
    </div>
  );
}
