"use client";

import type { ClipboardEvent as ReactClipboardEvent, ReactNode, RefObject } from "react";
import Zoom from "react-medium-image-zoom";
import { ArrowUp, CircleHelp, ImagePlus, Trash2, Upload } from "lucide-react";

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
  upscaleScale: string;
  upscaleOptions: string[];
  hasGenerateReferences: boolean;
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
  onUpscaleScaleChange: (value: string) => void;
  onPromptChange: (value: string) => void;
  onPromptPaste: (event: ReactClipboardEvent<HTMLTextAreaElement>) => void;
  onRemoveSourceImage: (id: string) => void;
  onAppendFiles: (files: FileList | null, role: "image" | "mask") => Promise<void>;
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
  upscaleScale,
  upscaleOptions,
  hasGenerateReferences,
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
  onUpscaleScaleChange,
  onPromptChange,
  onPromptPaste,
  onRemoveSourceImage,
  onAppendFiles,
  onSubmit,
}: PromptComposerProps) {
  const imageQualityLabel = imageQualityOptions.find((item) => item.value === imageQuality)?.label ?? imageQuality;
  const trimmedPrompt = imagePrompt.trim();
  const sourceImageCount = sourceImages.filter((item) => item.role === "image").length;
  const canSubmit =
    mode === "generate"
      ? trimmedPrompt.length > 0
      : mode === "edit"
        ? trimmedPrompt.length > 0 && sourceImageCount > 0
        : sourceImageCount > 0;
  const sizeHintTooltip =
    mode === "generate" && !hasGenerateReferences ? (
      <span className="group relative inline-flex items-center align-middle">
        <span
          tabIndex={0}
          className="inline-flex size-9 cursor-help items-center justify-center rounded-full border border-stone-200 bg-white text-stone-400 transition-colors hover:text-stone-700 focus-visible:text-stone-700 focus-visible:outline-none"
          aria-label="查看分辨率说明"
        >
          <CircleHelp className="size-4" />
        </span>
        <span className="pointer-events-none absolute right-0 bottom-full z-30 mb-2 w-80 max-w-[calc(100vw-2rem)] rounded-2xl border border-stone-200 bg-white px-4 py-3 text-xs font-normal leading-6 text-stone-600 opacity-0 shadow-[0_18px_50px_-24px_rgba(15,23,42,0.35)] transition-all duration-200 group-hover:pointer-events-auto group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:translate-y-0 group-focus-within:opacity-100">
          {imageSizeHint}
        </span>
      </span>
    ) : null;

  return (
    <div className="shrink-0 border-t border-stone-200 bg-white px-3 py-3 sm:px-5 sm:py-4">
      <div className="mx-auto flex max-w-[1120px] flex-col gap-3">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div className="inline-flex rounded-full bg-stone-100 p-1">
            {modeOptions.map((item) => (
              <button
                key={item.value}
                type="button"
                onClick={() => onModeChange(item.value)}
                className={cn(
                  "rounded-full px-4 py-2 text-sm font-medium transition",
                  mode === item.value
                    ? "bg-stone-950 text-white shadow-sm"
                    : "text-stone-600 hover:bg-stone-200 hover:text-stone-900",
                )}
              >
                {item.label}
              </button>
            ))}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {mode === "generate" && !hasGenerateReferences ? (
              <Select value={imageAspectRatio} onValueChange={onImageAspectRatioChange}>
                <SelectTrigger className="h-10 w-[108px] rounded-full border-stone-200 bg-white text-sm font-medium text-stone-700 shadow-none focus-visible:ring-0">
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

            {mode === "generate" && !hasGenerateReferences ? (
              <Select value={imageResolutionTier} onValueChange={onImageResolutionTierChange}>
                <SelectTrigger
                  className="h-10 w-[238px] rounded-full border-stone-200 bg-white text-sm font-medium text-stone-700 shadow-none focus-visible:ring-0"
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

            {mode === "generate" && !hasGenerateReferences ? (
              <Select value={imageQuality} onValueChange={onImageQualityChange}>
                <SelectTrigger
                  className="h-10 w-[136px] rounded-full border-stone-200 bg-white text-sm font-medium text-stone-700 shadow-none focus-visible:ring-0"
                  title={imageQualityOptions.find((item) => item.value === imageQuality)?.description}
                >
                  <SelectValue>{`质量 ${imageQualityLabel}`}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {imageQualityOptions.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      <span title={item.description}>质量 {item.label}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}

            {mode === "generate" && !hasGenerateReferences ? (
              <div className="flex items-center gap-1.5 rounded-full border border-stone-200 bg-white px-2.5 py-1">
                <span className="text-sm font-medium text-stone-700">张数</span>
                <Input
                  type="number"
                  min="1"
                  max="8"
                  step="1"
                  value={imageCount}
                  onChange={(event) => onImageCountChange(event.target.value)}
                  className="h-8 w-[42px] border-0 bg-transparent px-0 text-center text-sm font-medium text-stone-700 shadow-none focus-visible:ring-0"
                />
              </div>
            ) : null}

            {mode === "upscale" ? (
              <Select value={upscaleScale} onValueChange={onUpscaleScaleChange}>
                <SelectTrigger className="h-10 w-[132px] rounded-full border-stone-200 bg-white text-sm font-medium text-stone-700 shadow-none focus-visible:ring-0">
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

            <span className="rounded-full bg-stone-100 px-3 py-2 text-xs font-medium text-stone-600">
              剩余额度 {availableQuota}
            </span>
          </div>
        </div>

        <div
          className="overflow-hidden rounded-[28px] border border-stone-200 bg-[#fafaf9] shadow-[inset_0_1px_0_rgba(255,255,255,0.9)]"
          onClick={() => {
            textareaRef.current?.focus();
          }}
        >
          {sourceImages.length > 0 ? (
            <div className="hide-scrollbar flex gap-3 overflow-x-auto border-b border-stone-200 px-4 py-3">
              {sourceImages.map((item) => (
                <div
                  key={item.id}
                  className="w-[126px] shrink-0 overflow-hidden rounded-[18px] border border-stone-200 bg-white"
                >
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
                    <Image
                      src={item.dataUrl}
                      alt={item.name}
                      width={160}
                      height={110}
                      unoptimized
                      className="block h-20 w-full cursor-zoom-in bg-stone-50 object-contain"
                    />
                  </Zoom>
                </div>
              ))}
            </div>
          ) : null}

          <div className="px-4 pb-2 pt-3">
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
                  if (canSubmit) {
                    void onSubmit();
                  }
                }
              }}
              className="min-h-[92px] max-h-[480px] resize-none border-0 bg-transparent !px-1 !pt-1 !pb-1 text-[15px] leading-7 text-stone-900 shadow-none placeholder:text-stone-400 focus-visible:ring-0 overflow-y-auto"
            />
          </div>
          <div className="px-4 pb-4 pt-2">
            <div className="flex items-end justify-between gap-3">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8 rounded-full border-stone-200 bg-white px-2.5 text-xs font-medium text-stone-700 shadow-none"
                  onClick={(event) => {
                    event.stopPropagation();
                    uploadInputRef.current?.click();
                  }}
                >
                  <ImagePlus className="size-3.5" />
                  {mode === "generate" ? "上传参考图" : "上传源图"}
                </Button>

                {mode === "edit" ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-8 rounded-full border-stone-200 bg-white px-2.5 text-xs font-medium text-stone-700 shadow-none"
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
                disabled={!canSubmit}
                className="inline-flex size-9 shrink-0 items-center justify-center rounded-full bg-stone-950 text-white transition hover:bg-stone-800 disabled:cursor-not-allowed disabled:bg-stone-300"
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
