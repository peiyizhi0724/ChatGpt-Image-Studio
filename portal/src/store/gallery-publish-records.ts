"use client";

import localforage from "localforage";

export type GalleryPublishRecord = {
  key: string;
  work_id: string;
  published_at: string;
};

const galleryPublishStorage = localforage.createInstance({
  name: "chatgpt2api-studio-portal",
  storeName: "gallery_publish_records",
});

const GALLERY_PUBLISH_RECORDS_KEY = "items";

let cachedRecords: Record<string, GalleryPublishRecord> | null = null;
let loadPromise: Promise<Record<string, GalleryPublishRecord>> | null = null;
let writeQueue: Promise<void> = Promise.resolve();

export function buildGalleryPublishRecordKey(conversationId: string, turnId: string, imageId: string) {
  return [conversationId, turnId, imageId].map((item) => String(item || "").trim()).join(":");
}

async function loadRecordCache(): Promise<Record<string, GalleryPublishRecord>> {
  if (cachedRecords) {
    return cachedRecords;
  }

  if (!loadPromise) {
    loadPromise = galleryPublishStorage
      .getItem<Record<string, GalleryPublishRecord>>(GALLERY_PUBLISH_RECORDS_KEY)
      .then((items) => {
        cachedRecords = { ...(items || {}) };
        return cachedRecords;
      })
      .finally(() => {
        loadPromise = null;
      });
  }

  return loadPromise;
}

async function persistRecordCache() {
  const snapshot = { ...(cachedRecords || {}) };
  cachedRecords = snapshot;
  writeQueue = writeQueue.then(async () => {
    await galleryPublishStorage.setItem(GALLERY_PUBLISH_RECORDS_KEY, snapshot);
  });
  await writeQueue;
}

export async function listGalleryPublishRecords() {
  const items = await loadRecordCache();
  return { ...items };
}

export async function getGalleryPublishRecord(key: string) {
  const items = await loadRecordCache();
  return items[String(key || "").trim()] ?? null;
}

export async function saveGalleryPublishRecord(record: GalleryPublishRecord) {
  const items = await loadRecordCache();
  cachedRecords = {
    ...items,
    [record.key]: {
      ...record,
      key: String(record.key || "").trim(),
      work_id: String(record.work_id || "").trim(),
      published_at: String(record.published_at || "").trim(),
    },
  };
  await persistRecordCache();
}
