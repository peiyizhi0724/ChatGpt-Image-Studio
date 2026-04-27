"use client";

import localforage from "localforage";

import type { ImageModel, ImageQuality } from "@/lib/api";

export type ImageMode = "generate" | "edit" | "upscale";

export type StoredSourceImage = {
  id: string;
  role: "image" | "mask";
  name: string;
  dataUrl?: string;
  url?: string;
};

export type StoredImage = {
  id: string;
  status?: "loading" | "success" | "error";
  b64_json?: string;
  url?: string;
  revised_prompt?: string;
  file_id?: string;
  gen_id?: string;
  conversation_id?: string;
  parent_message_id?: string;
  source_account_id?: string;
  error?: string;
};

export type ImageConversationStatus = "generating" | "success" | "error";

export type ImageConversationTurn = {
  id: string;
  title: string;
  mode: ImageMode;
  prompt: string;
  model: ImageModel;
  count: number;
  size?: string;
  quality?: ImageQuality;
  scale?: string;
  sourceImages?: StoredSourceImage[];
  images: StoredImage[];
  createdAt: string;
  status: ImageConversationStatus;
  error?: string;
};

export type ImageConversation = {
  id: string;
  title: string;
  mode: ImageMode;
  prompt: string;
  model: ImageModel;
  count: number;
  size?: string;
  quality?: ImageQuality;
  scale?: string;
  sourceImages?: StoredSourceImage[];
  images: StoredImage[];
  createdAt: string;
  status: ImageConversationStatus;
  error?: string;
  turns?: ImageConversationTurn[];
};

const imageConversationStorage = localforage.createInstance({
  name: "chatgpt2api-studio-portal",
  storeName: "image_conversations",
});

const IMAGE_CONVERSATIONS_KEY = "items";
let cachedConversations: ImageConversation[] | null = null;
let loadPromise: Promise<ImageConversation[]> | null = null;
let writeQueue: Promise<void> = Promise.resolve();

function sortConversations(items: ImageConversation[]) {
  return [...items].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

async function loadConversationCache(): Promise<ImageConversation[]> {
  if (cachedConversations) {
    return cachedConversations;
  }

  if (!loadPromise) {
    loadPromise = imageConversationStorage
      .getItem<ImageConversation[]>(IMAGE_CONVERSATIONS_KEY)
      .then((items) => {
        cachedConversations = sortConversations((items || []).map(normalizeConversation));
        return cachedConversations;
      })
      .finally(() => {
        loadPromise = null;
      });
  }

  return loadPromise;
}

async function persistConversationCache() {
  const snapshot = sortConversations((cachedConversations || []).map(normalizeConversation));
  cachedConversations = snapshot;
  writeQueue = writeQueue.then(async () => {
    await imageConversationStorage.setItem(IMAGE_CONVERSATIONS_KEY, snapshot);
  });
  await writeQueue;
}

function normalizeStoredImage(image: StoredImage): StoredImage {
  if (image.status === "loading" || image.status === "error" || image.status === "success") {
    return image;
  }
  return {
    ...image,
    status: image.b64_json || image.url ? "success" : "loading",
  };
}

function normalizeTurn(turn: ImageConversationTurn): ImageConversationTurn {
  return {
    ...turn,
    mode: turn.mode || "generate",
    sourceImages: Array.isArray(turn.sourceImages) ? turn.sourceImages : [],
    images: (turn.images || []).map(normalizeStoredImage),
  };
}

export function normalizeConversation(conversation: ImageConversation): ImageConversation {
  const turns =
    Array.isArray(conversation.turns) && conversation.turns.length > 0
      ? conversation.turns.map(normalizeTurn)
      : [
          normalizeTurn({
            id: `${conversation.id}-legacy`,
            title: conversation.title,
            mode: conversation.mode || "generate",
            prompt: conversation.prompt,
            model: conversation.model,
            count: conversation.count,
            size: conversation.size,
            quality: conversation.quality,
            scale: conversation.scale,
            sourceImages: conversation.sourceImages,
            images: conversation.images || [],
            createdAt: conversation.createdAt,
            status: conversation.status,
            error: conversation.error,
          }),
        ];

  const latestTurn = turns[turns.length - 1];
  return {
    ...conversation,
    title: latestTurn.title,
    mode: latestTurn.mode,
    prompt: latestTurn.prompt,
    model: latestTurn.model,
    count: latestTurn.count,
    size: latestTurn.size,
    quality: latestTurn.quality,
    scale: latestTurn.scale,
    sourceImages: latestTurn.sourceImages,
    images: latestTurn.images,
    createdAt: latestTurn.createdAt,
    status: latestTurn.status,
    error: latestTurn.error,
    turns,
  };
}

export async function listImageConversations(): Promise<ImageConversation[]> {
  const items = await loadConversationCache();
  return sortConversations(items.map(normalizeConversation));
}

export async function getImageConversation(id: string): Promise<ImageConversation | null> {
  const items = await loadConversationCache();
  return items.find((item) => item.id === id) ?? null;
}

export async function saveImageConversation(conversation: ImageConversation): Promise<void> {
  const items = await loadConversationCache();
  cachedConversations = sortConversations([
    normalizeConversation(conversation),
    ...items.filter((item) => item.id !== conversation.id),
  ]);
  await persistConversationCache();
}

export async function updateImageConversation(
  id: string,
  updater: (current: ImageConversation | null) => ImageConversation,
): Promise<ImageConversation> {
  const items = await loadConversationCache();
  const current = items.find((item) => item.id === id) ?? null;
  const nextConversation = normalizeConversation(updater(current));
  cachedConversations = sortConversations([nextConversation, ...items.filter((item) => item.id !== id)]);
  await persistConversationCache();
  return nextConversation;
}

export async function deleteImageConversation(id: string): Promise<void> {
  const items = await loadConversationCache();
  cachedConversations = items.filter((item) => item.id !== id);
  await persistConversationCache();
}

export async function clearImageConversations(): Promise<void> {
  cachedConversations = [];
  loadPromise = null;
  writeQueue = writeQueue.then(async () => {
    await imageConversationStorage.removeItem(IMAGE_CONVERSATIONS_KEY);
  });
  await writeQueue;
}
