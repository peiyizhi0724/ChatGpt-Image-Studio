"use client";

import type {
  InpaintSourceReference,
  ImageModel,
  ImageQuality,
  ImageResolutionAccess,
} from "@/lib/api";
import type {
  ImageConversationTurn,
  ImageMode,
  StoredImage,
  StoredSourceImage,
} from "@/store/image-conversations";

export function buildConversationTitle(mode: ImageMode, prompt: string, scale = "") {
  const trimmed = prompt.trim();
  const prefix = mode === "generate" ? "生成" : "编辑";
  if (!trimmed) {
    return scale ? `${prefix} · ${scale}` : prefix;
  }
  if (trimmed.length <= 8) {
    return `${prefix} · ${trimmed}`;
  }
  return `${prefix} · ${trimmed.slice(0, 8)}...`;
}

export function createLoadingImages(count: number, conversationId: string) {
  return Array.from({ length: count }, (_, index) => ({
    id: `${conversationId}-${index}`,
    status: "loading" as const,
  }));
}

export function createConversationTurn(payload: {
  turnId: string;
  title: string;
  mode: ImageMode;
  prompt: string;
  model: ImageModel;
  count: number;
  size?: string;
  resolutionAccess?: ImageResolutionAccess;
  quality?: ImageQuality;
  scale?: string;
  sourceImages?: StoredSourceImage[];
  sourceReference?: InpaintSourceReference;
  images: StoredImage[];
  createdAt: string;
  status: "queued" | "running" | "generating" | "success" | "error" | "cancelled";
  error?: string;
}): ImageConversationTurn {
  return {
    id: payload.turnId,
    title: payload.title,
    mode: payload.mode,
    prompt: payload.prompt,
    model: payload.model,
    count: payload.count,
    size: payload.size,
    resolutionAccess: payload.resolutionAccess,
    quality: payload.quality,
    scale: payload.scale,
    sourceImages: payload.sourceImages ?? [],
    sourceReference: payload.sourceReference,
    images: payload.images,
    createdAt: payload.createdAt,
    status: payload.status,
    error: payload.error,
  };
}

export async function dataUrlToFile(dataUrl: string, fileName: string) {
  const response = await fetch(dataUrl);
  const blob = await response.blob();
  return new File([blob], fileName, { type: blob.type || "image/png" });
}

export function mergeResultImages(
  conversationId: string,
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
          id: `${conversationId}-${index}`,
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
          id: `${conversationId}-${index}`,
          status: "error",
          error: "接口没有返回图片数据",
        },
  );

  while (results.length < expected) {
    results.push({
      id: `${conversationId}-${results.length}`,
      status: "error",
      error: "接口返回的图片数量不足",
    });
  }
  return results;
}

export function countFailures(images: StoredImage[]) {
  return images.filter((image) => image.status === "error").length;
}

export function formatImageErrorMessage(message: string) {
  const trimmed = String(message || "").trim();
  if (!trimmed) {
    return "处理图片失败";
  }

  const normalized = trimmed.toLowerCase();
  if (normalized.includes("an error occurred while processing your request")) {
    const requestId = trimmed.match(/request id\s+([a-z0-9-]+)/i)?.[1];
    return [
      "提示词内容过多，或当前分辨率/质量组合过高。",
      "建议减少提示词内容，或降低分辨率、质量后重试。",
      requestId ? `请求 ID：${requestId}` : "",
    ].filter(Boolean).join("\n");
  }

  if (normalized.includes("no images generated") && normalized.includes("model may have refused")) {
    return "没有生成图片，模型可能检测到敏感内容，拒绝了这次请求，建议重试或调整提示词。";
  }

  if (normalized.includes("timed out waiting for async image generation")) {
    return "图片生成等待超时，建议稍后重试或增加超时时间，或降低分辨率/质量。";
  }

  return trimmed;
}

export function formatImageError(error: unknown) {
  return formatImageErrorMessage(error instanceof Error ? error.message : String(error || "处理图片失败"));
}

export function buildInpaintSourceReference(image: StoredImage): InpaintSourceReference | undefined {
  if (!image.file_id || !image.gen_id || !image.source_account_id) {
    return undefined;
  }
  return {
    original_file_id: image.file_id,
    original_gen_id: image.gen_id,
    conversation_id: image.conversation_id,
    parent_message_id: image.parent_message_id,
    source_account_id: image.source_account_id,
  };
}

function extractErrorCode(error: unknown) {
  if (!error || typeof error !== "object" || !("code" in error)) {
    return "";
  }
  const code = (error as { code?: unknown }).code;
  return typeof code === "string" ? code : "";
}

export function shouldFallbackSelectionEdit(error: unknown) {
  const code = extractErrorCode(error);
  if (["source_account_not_found", "source_account_unavailable", "source_context_missing"].includes(code)) {
    return true;
  }

  const normalized = (error instanceof Error ? error.message : String(error || "")).toLowerCase();
  return (
    normalized.includes("conversation not found") ||
    normalized.includes("source account") ||
    normalized.includes("image account is unavailable") ||
    normalized.includes("原始图片") ||
    normalized.includes("所属账号")
  );
}
