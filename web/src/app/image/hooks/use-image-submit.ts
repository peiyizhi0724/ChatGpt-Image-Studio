"use client";

import { useCallback, useRef } from "react";
import { toast } from "sonner";

import {
  createImageTask,
  type ImageModel,
  type ImageQuality,
  type ImageResolutionAccess,
} from "@/lib/api";
import type {
  ImageConversation,
  ImageConversationTurn,
  ImageMode,
  StoredSourceImage,
} from "@/store/image-conversations";

import type { EditorTarget } from "./use-image-source-inputs";
import {
  buildConversationTitle,
  buildInpaintSourceReference,
  createConversationTurn,
  createLoadingImages,
} from "../submit-utils";
import { buildSourceImageUrl } from "../view-utils";

type UseImageSubmitOptions = {
  mode: ImageMode;
  imagePrompt: string;
  imageModel: ImageModel;
  imageSources: StoredSourceImage[];
  maskSource: StoredSourceImage | null;
  sourceImages: StoredSourceImage[];
  parsedCount: number;
  imageSize: string;
  imageResolutionAccess: ImageResolutionAccess;
  imageQuality: ImageQuality;
  selectedConversationId: string | null;
  editorTarget: EditorTarget | null;
  makeId: () => string;
  focusConversation: (conversationId: string) => void;
  closeSelectionEditor: () => void;
  setImagePrompt: (value: string) => void;
  setSourceImages: (value: StoredSourceImage[]) => void;
  setSubmitElapsedSeconds: (value: number) => void;
  persistConversation: (conversation: ImageConversation) => Promise<void>;
  updateConversation: (
    conversationId: string,
    updater: (current: ImageConversation | null) => ImageConversation,
  ) => Promise<void>;
  resetComposer: (nextMode?: ImageMode) => void;
};

function buildConversationBase(
  conversationId: string,
  draftTurn: ImageConversationTurn,
): ImageConversation {
  return {
    id: conversationId,
    title: draftTurn.title,
    mode: draftTurn.mode,
    prompt: draftTurn.prompt,
    model: draftTurn.model,
    count: draftTurn.count,
    size: draftTurn.size,
    resolutionAccess: draftTurn.resolutionAccess,
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

function buildSourceReference(payload: {
  id: string;
  role: "image" | "mask";
  name: string;
  url: string;
}): StoredSourceImage {
  if (payload.url.startsWith("data:")) {
    return {
      id: payload.id,
      role: payload.role,
      name: payload.name,
      dataUrl: payload.url,
    };
  }
  return {
    id: payload.id,
    role: payload.role,
    name: payload.name,
    url: payload.url,
  };
}

function normalizeImageQuality(value: string | undefined, fallback: ImageQuality) {
  const trimmed = String(value || "").trim();
  if (trimmed === "low" || trimmed === "medium" || trimmed === "high") {
    return trimmed;
  }
  return fallback;
}

export function useImageSubmit({
  mode,
  imagePrompt,
  imageModel,
  imageSources,
  maskSource,
  sourceImages,
  parsedCount,
  imageSize,
  imageResolutionAccess,
  imageQuality,
  selectedConversationId,
  editorTarget,
  makeId,
  focusConversation,
  closeSelectionEditor,
  setImagePrompt,
  setSourceImages,
  setSubmitElapsedSeconds,
  persistConversation,
  updateConversation,
  resetComposer,
}: UseImageSubmitOptions) {
  const isSelectionEditDispatchingRef = useRef(false);
  const isSubmitDispatchingRef = useRef(false);
  const retryingTurnIdsRef = useRef<Set<string>>(new Set());

  const handleSelectionEditSubmit = useCallback(
    async ({
      prompt,
      mask,
      aspectRatio: _aspectRatio,
      resolutionTier: _resolutionTier,
      quality: overrideQuality,
    }: {
      prompt: string;
      mask: {
        file: File;
        previewDataUrl: string;
      };
      aspectRatio?: string;
      resolutionTier?: string;
      quality?: string;
    }) => {
      if (isSelectionEditDispatchingRef.current || !editorTarget) {
        return;
      }
      isSelectionEditDispatchingRef.current = true;

      const sourceReference = editorTarget.image
        ? buildInpaintSourceReference(editorTarget.image)
        : undefined;
      const targetConversationId =
        editorTarget.conversationId ?? selectedConversationId;
      const conversationId = targetConversationId ?? makeId();
      const supportsEditableOutputOptions = true;
      const nextQuality = supportsEditableOutputOptions
        ? normalizeImageQuality(overrideQuality, imageQuality)
        : imageQuality;
      const turnId = makeId();
      const now = new Date().toISOString();
      const draftTurn = createConversationTurn({
        turnId,
        title: buildConversationTitle("edit", prompt),
        mode: "edit",
        prompt,
        model: imageModel,
        count: 1,
        size: supportsEditableOutputOptions ? imageSize : undefined,
        resolutionAccess: supportsEditableOutputOptions
          ? imageResolutionAccess
          : undefined,
        quality: supportsEditableOutputOptions ? nextQuality : undefined,
        sourceImages: [
          buildSourceReference({
            id: makeId(),
            role: "image",
            name: editorTarget.imageName,
            url: editorTarget.sourceDataUrl,
          }),
          {
            id: makeId(),
            role: "mask",
            name: "mask.png",
            dataUrl: mask.previewDataUrl,
          },
        ],
        sourceReference,
        images: createLoadingImages(1, turnId),
        createdAt: now,
        status: "queued",
      });

      setSubmitElapsedSeconds(0);
      focusConversation(conversationId);
      setImagePrompt("");
      setSourceImages([]);
      closeSelectionEditor();

      try {
        if (targetConversationId) {
          await updateConversation(conversationId, (current) => {
            if (!current) {
              return buildConversationBase(conversationId, draftTurn);
            }
            return {
              ...current,
              turns: [...(current.turns ?? []), draftTurn],
            };
          });
        } else {
          await persistConversation(
            buildConversationBase(conversationId, draftTurn),
          );
        }

        const result = await createImageTask({
          conversationId,
          turnId,
          mode: "edit",
          prompt,
          model: imageModel,
          count: 1,
          size: supportsEditableOutputOptions ? imageSize : undefined,
          resolutionAccess: supportsEditableOutputOptions
            ? imageResolutionAccess
            : undefined,
          quality: supportsEditableOutputOptions ? nextQuality : undefined,
          sourceImages: draftTurn.sourceImages,
          sourceReference,
        });

        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns: (current?.turns ?? [draftTurn]).map((turn) =>
            turn.id === turnId
              ? {
                  ...turn,
                  taskId: result.task.id,
                  queuePosition: result.task.queuePosition,
                  waitingReason: result.task.waitingReason,
                  waitingDetail: result.task.blockers?.[0]?.detail,
                }
              : turn,
          ),
        }));
        toast.success("图片编辑任务已加入队列");
      } catch (error) {
        const message =
          error instanceof Error ? error.message : "提交编辑失败";
        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns: (current?.turns ?? [draftTurn]).map((turn) =>
            turn.id === turnId
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
        toast.error(message);
      } finally {
        isSelectionEditDispatchingRef.current = false;
      }
    },
    [
      closeSelectionEditor,
      editorTarget,
      focusConversation,
      imageModel,
      imageQuality,
      imageResolutionAccess,
      imageSize,
      makeId,
      persistConversation,
      selectedConversationId,
      setImagePrompt,
      setSourceImages,
      setSubmitElapsedSeconds,
      updateConversation,
    ],
  );

  const handleRetryTurn = useCallback(
    async (
      conversationId: string,
      turn: ImageConversationTurn,
      imageIndex?: number,
    ) => {
      if (retryingTurnIdsRef.current.has(turn.id)) {
        return;
      }

      const prompt = turn.prompt?.trim() ?? "";
      const turnMode = turn.mode || "generate";
      const turnSourceImages = Array.isArray(turn.sourceImages)
        ? turn.sourceImages
        : [];
      const turnImageSources = turnSourceImages.filter(
        (item) => item.role === "image" && buildSourceImageUrl(item),
      );
      const turnQuality = turn.quality || "high";
      const isSingleImageRetry =
        turnMode === "generate" &&
        typeof imageIndex === "number" &&
        imageIndex >= 0 &&
        (turn.count || 1) > 1;
      const displayCount = Math.max(1, turn.count || 1);
      const requestCount = isSingleImageRetry ? 1 : displayCount;
      const retryTaskId = makeId();

      if (turnMode === "generate" && !prompt) {
        toast.error("该记录缺少提示词，无法重试");
        return;
      }
      if (turnMode === "edit" && turnImageSources.length === 0) {
        toast.error("该记录缺少源图，无法重试");
        return;
      }

      retryingTurnIdsRef.current.add(turn.id);
      const nextImages = isSingleImageRetry
        ? turn.images.map((image, index) =>
            index === imageIndex
              ? {
                  ...image,
                  status: "loading" as const,
                  error: undefined,
                }
              : image,
          )
        : createLoadingImages(requestCount, turn.id);
      const draftTurn = createConversationTurn({
        turnId: turn.id,
        title: buildConversationTitle(turnMode, prompt),
        mode: turnMode,
        prompt,
        model: turn.model,
        count: displayCount,
        size: turn.size,
        resolutionAccess: turn.resolutionAccess,
        quality: turnQuality,
        sourceImages: turnSourceImages,
        sourceReference: turn.sourceReference,
        images: nextImages,
        createdAt: new Date().toISOString(),
        status: isSingleImageRetry ? "running" : "queued",
      });

      setSubmitElapsedSeconds(0);
      focusConversation(conversationId);

      try {
        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns:
            current?.turns?.map((item) =>
              item.id === turn.id ? draftTurn : item,
            ) ?? [draftTurn],
        }));

        const result = await createImageTask({
          taskId: retryTaskId,
          conversationId,
          turnId: turn.id,
          mode: turnMode,
          prompt,
          model: turn.model,
          count: requestCount,
          retryImageIndex: isSingleImageRetry ? imageIndex : undefined,
          size: turn.size,
          resolutionAccess: turn.resolutionAccess,
          quality: turnQuality,
          sourceImages: turnSourceImages,
          sourceReference: turn.sourceReference,
        });

        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns: (current?.turns ?? [draftTurn]).map((item) =>
            item.id === turn.id
              ? {
                  ...item,
                  taskId: result.task.id,
                  queuePosition: result.task.queuePosition,
                  waitingReason: result.task.waitingReason,
                  waitingDetail: result.task.blockers?.[0]?.detail,
                  status: isSingleImageRetry ? "running" : item.status,
                }
              : item,
          ),
        }));
        toast.success(
          isSingleImageRetry ? "失败图片已重新加入队列" : "任务已重新加入队列",
        );
      } catch (error) {
        const message =
          error instanceof Error ? error.message : "提交任务失败";
        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns: (current?.turns ?? [draftTurn]).map((item) =>
            item.id === turn.id
              ? {
                  ...item,
                  status:
                    isSingleImageRetry
                      ? item.images.some(
                          (image, index) =>
                            index !== imageIndex && image.status === "loading",
                        )
                        ? "running"
                        : "error"
                      : "error",
                  error: message,
                  images: isSingleImageRetry
                    ? item.images.map((image, index) =>
                        index === imageIndex
                          ? {
                              ...image,
                              status: "error" as const,
                              error: message,
                            }
                          : image,
                      )
                    : item.images.map((image) => ({
                        ...image,
                        status: "error" as const,
                        error: message,
                      })),
                }
              : item,
          ),
        }));
        toast.error(message);
      } finally {
        retryingTurnIdsRef.current.delete(turn.id);
      }
    },
    [focusConversation, makeId, setSubmitElapsedSeconds, updateConversation],
  );

  const handleSubmit = useCallback(async () => {
    if (isSubmitDispatchingRef.current) {
      return;
    }
    const prompt = imagePrompt.trim();
    if (mode === "generate" && !prompt) {
      toast.error("请输入提示词");
      return;
    }
    if (mode === "edit" && imageSources.length === 0) {
      toast.error("编辑模式至少需要一张源图");
      return;
    }
    if (mode === "edit" && !prompt) {
      toast.error("编辑模式需要提示词");
      return;
    }
    isSubmitDispatchingRef.current = true;

    const conversationId = selectedConversationId ?? makeId();
    const turnId = makeId();
      const expectedCount =
      mode === "generate" ? parsedCount : 1;
    const draftTurn = createConversationTurn({
      turnId,
      title: buildConversationTitle(mode, prompt),
      mode,
      prompt,
      model: imageModel,
      count: expectedCount,
      size: imageSize,
      resolutionAccess: imageResolutionAccess,
      quality: imageQuality,
      sourceImages,
      images: createLoadingImages(expectedCount, turnId),
      createdAt: new Date().toISOString(),
      status: "queued",
    });

    setSubmitElapsedSeconds(0);
    focusConversation(conversationId);
    setImagePrompt("");
    setSourceImages([]);

    try {
      if (selectedConversationId) {
        await updateConversation(conversationId, (current) => ({
          ...(current ?? buildConversationBase(conversationId, draftTurn)),
          turns: [...(current?.turns ?? []), draftTurn],
        }));
      } else {
        await persistConversation(
          buildConversationBase(conversationId, draftTurn),
        );
      }

      const result = await createImageTask({
        conversationId,
        turnId,
        mode,
        prompt,
        model: imageModel,
        count: expectedCount,
        size: imageSize,
        resolutionAccess: imageResolutionAccess,
        quality: imageQuality,
        sourceImages,
      });

      await updateConversation(conversationId, (current) => ({
        ...(current ?? buildConversationBase(conversationId, draftTurn)),
        turns: (current?.turns ?? [draftTurn]).map((turn) =>
          turn.id === turnId
            ? {
                ...turn,
                taskId: result.task.id,
                queuePosition: result.task.queuePosition,
                waitingReason: result.task.waitingReason,
                waitingDetail: result.task.blockers?.[0]?.detail,
              }
            : turn,
        ),
      }));
      resetComposer(mode === "generate" ? "generate" : "edit");
      toast.success("图片任务已加入队列");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "提交任务失败";
      await updateConversation(conversationId, (current) => ({
        ...(current ?? buildConversationBase(conversationId, draftTurn)),
        turns: (current?.turns ?? [draftTurn]).map((turn) =>
          turn.id === turnId
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
      toast.error(message);
    } finally {
      isSubmitDispatchingRef.current = false;
    }
  }, [
    focusConversation,
    imageModel,
    imagePrompt,
    imageSources,
    makeId,
    mode,
    imageSize,
    imageResolutionAccess,
    imageQuality,
    parsedCount,
    persistConversation,
    resetComposer,
    selectedConversationId,
    setImagePrompt,
    setSourceImages,
    setSubmitElapsedSeconds,
    sourceImages,
    updateConversation,
  ]);

  return {
    handleSelectionEditSubmit,
    handleRetryTurn,
    handleSubmit,
  };
}
