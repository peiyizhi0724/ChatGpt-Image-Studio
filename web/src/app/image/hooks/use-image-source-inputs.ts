"use client";

import { useCallback, useState, type ClipboardEvent as ReactClipboardEvent, type RefObject } from "react";
import { toast } from "sonner";

import type { ImageMode } from "@/store/image-conversations";
import type { StoredImage, StoredSourceImage } from "@/store/image-conversations";

import { buildImageDataUrl } from "../view-utils";

export type EditorTarget = {
  conversationId: string;
  turnId: string;
  image: StoredImage;
  imageName: string;
  sourceDataUrl: string;
};

type UseImageSourceInputsOptions = {
  mode: ImageMode;
  setMode: (mode: ImageMode) => void;
  setImagePrompt: (value: string) => void;
  focusConversation: (conversationId: string) => void;
  textareaRef: RefObject<HTMLTextAreaElement | null>;
  makeId: () => string;
};

async function fileToDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(new Error(`读取 ${file.name} 失败`));
    reader.readAsDataURL(file);
  });
}

export function useImageSourceInputs({
  mode,
  setMode,
  setImagePrompt,
  focusConversation,
  textareaRef,
  makeId,
}: UseImageSourceInputsOptions) {
  const [sourceImages, setSourceImages] = useState<StoredSourceImage[]>([]);
  const [editorTarget, setEditorTarget] = useState<EditorTarget | null>(null);

  const appendFiles = useCallback(async (files: File[] | FileList | null, role: "image" | "mask") => {
    const normalizedFiles = files ? Array.from(files) : [];
    if (normalizedFiles.length === 0) {
      return;
    }
    const nextItems = await Promise.all(
      normalizedFiles.map(async (file) => ({
        id: makeId(),
        role,
        name: file.name,
        dataUrl: await fileToDataUrl(file),
      })),
    );
    setSourceImages((prev) => {
      if (role === "mask") {
        return [...prev.filter((item) => item.role !== "mask"), nextItems[0]];
      }
      if (mode === "upscale") {
        return [
          ...prev.filter((item) => item.role === "mask"),
          {
            ...nextItems[0],
            name: nextItems[0]?.name || "upscale.png",
          },
        ];
      }
      return [...prev.filter((item) => item.role !== "mask"), ...prev.filter((item) => item.role === "mask"), ...nextItems];
    });
  }, [makeId, mode]);

  const handlePromptPaste = useCallback((event: ReactClipboardEvent<HTMLTextAreaElement>) => {
    const clipboardImages = Array.from(event.clipboardData.items)
      .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
      .map((item) => item.getAsFile())
      .filter((file): file is File => Boolean(file));

    if (clipboardImages.length === 0) {
      return;
    }

    event.preventDefault();
    void appendFiles(clipboardImages, "image");
    toast.success(
      mode === "generate"
        ? "已从剪贴板添加参考图"
        : mode === "edit"
          ? "已从剪贴板添加源图"
          : "已从剪贴板添加放大源图",
    );
  }, [appendFiles, mode]);

  const removeSourceImage = useCallback((id: string) => {
    setSourceImages((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const seedFromResult = useCallback((conversationId: string, image: StoredImage, nextMode: ImageMode) => {
    const dataUrl = buildImageDataUrl(image);
    if (!dataUrl) {
      toast.error("当前图片没有可复用的数据");
      return;
    }
    focusConversation(conversationId);
    setMode(nextMode);
    setSourceImages([
      {
        id: makeId(),
        role: "image",
        name: "source.png",
        dataUrl,
      },
    ]);
    if (nextMode === "upscale") {
      setImagePrompt("");
    }
    textareaRef.current?.focus();
  }, [focusConversation, makeId, setImagePrompt, setMode, textareaRef]);

  const openSelectionEditor = useCallback((conversationId: string, turnId: string, image: StoredImage, imageName: string) => {
    const dataUrl = buildImageDataUrl(image);
    if (!dataUrl) {
      toast.error("当前图片没有可复用的数据");
      return;
    }
    setEditorTarget({
      conversationId,
      turnId,
      image,
      imageName,
      sourceDataUrl: dataUrl,
    });
  }, []);

  const closeSelectionEditor = useCallback(() => {
    setEditorTarget(null);
  }, []);

  return {
    sourceImages,
    setSourceImages,
    editorTarget,
    setEditorTarget,
    appendFiles,
    handlePromptPaste,
    removeSourceImage,
    seedFromResult,
    openSelectionEditor,
    closeSelectionEditor,
  };
}
