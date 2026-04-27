"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import {
  clearImageConversations,
  deleteImageConversation,
  getCachedImageConversationsSnapshot,
  listImageConversations,
  type ImageConversation,
} from "@/store/image-conversations";

type UseImageHistoryOptions = {
  normalizeHistory: (
    items: ImageConversation[],
  ) => Promise<ImageConversation[]>;
  mountedRef: React.RefObject<boolean>;
  draftSelectionRef: React.RefObject<boolean>;
  activeConversationIds: Set<string>;
  preferredActiveConversationId: string | null;
};

function mergeActiveTaskConversations(
  currentItems: ImageConversation[],
  refreshedItems: ImageConversation[],
  activeConversationIds: Set<string>,
) {
  const nextById = new Map(refreshedItems.map((item) => [item.id, item]));

  if (activeConversationIds.size === 0) {
    return refreshedItems;
  }

  const missingActiveItems = currentItems.filter(
    (item) => activeConversationIds.has(item.id) && !nextById.has(item.id),
  );
  if (missingActiveItems.length === 0) {
    return refreshedItems;
  }

  return [...missingActiveItems, ...refreshedItems].sort((a, b) =>
    b.createdAt.localeCompare(a.createdAt),
  );
}

export function useImageHistory({
  normalizeHistory,
  mountedRef,
  draftSelectionRef,
  activeConversationIds,
  preferredActiveConversationId,
}: UseImageHistoryOptions) {
  const cachedConversations = getCachedImageConversationsSnapshot();
  const conversationsRef = useRef<ImageConversation[]>(
    cachedConversations ?? [],
  );
  const [conversations, setConversations] = useState<ImageConversation[]>(
    cachedConversations ?? [],
  );
  const [selectedConversationId, setSelectedConversationId] = useState<
    string | null
  >(cachedConversations?.[0]?.id ?? null);
  const [isLoadingHistory, setIsLoadingHistory] =
    useState(!cachedConversations);

  useEffect(() => {
    conversationsRef.current = conversations;
  }, [conversations]);

  const focusConversation = useCallback(
    (conversationId: string) => {
      draftSelectionRef.current = false;
      setSelectedConversationId(conversationId);
    },
    [draftSelectionRef],
  );

  const openDraftConversation = useCallback(() => {
    draftSelectionRef.current = true;
    setSelectedConversationId(null);
  }, [draftSelectionRef]);

  const hasActiveTask = useCallback((conversationId?: string) => {
    if (!conversationId) {
      return activeConversationIds.size > 0;
    }
    return activeConversationIds.has(conversationId);
  }, [activeConversationIds]);

  const refreshHistory = useCallback(
    async (
      options: {
        normalize?: boolean;
        silent?: boolean;
        withLoading?: boolean;
      } = {},
    ) => {
      const {
        normalize = false,
        silent = false,
        withLoading = false,
      } = options;

      try {
        if (
          withLoading &&
          mountedRef.current &&
          !getCachedImageConversationsSnapshot()
        ) {
          setIsLoadingHistory(true);
        }
        const items = await listImageConversations();
        const nextItems = normalize ? await normalizeHistory(items) : items;
        if (!mountedRef.current) {
          return;
        }
        const currentItems =
          getCachedImageConversationsSnapshot() ?? conversationsRef.current;
        const mergedItems = mergeActiveTaskConversations(
          currentItems,
          nextItems,
          activeConversationIds,
        );
        conversationsRef.current = mergedItems;
        setConversations(mergedItems);
        setSelectedConversationId((current) => {
          if (current && mergedItems.some((item) => item.id === current)) {
            return current;
          }
          if (draftSelectionRef.current) {
            return null;
          }
          if (
            preferredActiveConversationId &&
            mergedItems.some((item) => item.id === preferredActiveConversationId)
          ) {
            return preferredActiveConversationId;
          }
          return mergedItems[0]?.id ?? null;
        });
      } catch (error) {
        if (!silent && mountedRef.current) {
          const message =
            error instanceof Error ? error.message : "加载会话失败";
          toast.error(message);
        }
      } finally {
        if (withLoading && mountedRef.current) {
          setIsLoadingHistory(false);
        }
      }
    },
    [activeConversationIds, draftSelectionRef, mountedRef, normalizeHistory, preferredActiveConversationId],
  );

  const handleCreateDraft = useCallback(
    (
      resetComposer: (nextMode?: "generate" | "edit") => void,
      textareaRef: React.RefObject<HTMLTextAreaElement | null>,
    ) => {
      openDraftConversation();
      resetComposer("generate");
      textareaRef.current?.focus();
    },
    [openDraftConversation],
  );

  const handleDeleteConversation = useCallback(
    async (id: string) => {
      if (hasActiveTask(id)) {
        toast.error("当前会话仍在处理中，请等待任务完成后再删除");
        return;
      }

      const previousSelectedId = selectedConversationId;
      const previousDraftSelection = draftSelectionRef.current;

      setConversations((current) => {
        const nextConversations = current.filter((item) => item.id !== id);
        conversationsRef.current = nextConversations;
        setSelectedConversationId((prev) => {
          if (prev !== id) {
            return prev;
          }
          draftSelectionRef.current = false;
          return nextConversations[0]?.id ?? null;
        });
        return nextConversations;
      });

      try {
        await deleteImageConversation(id);
      } catch (error) {
        const message = error instanceof Error ? error.message : "删除会话失败";
        toast.error(message);
        const items = await listImageConversations();
        if (!mountedRef.current) {
          return;
        }
        draftSelectionRef.current = previousDraftSelection;
        conversationsRef.current = items;
        setConversations(items);
        setSelectedConversationId(() => {
          if (
            previousSelectedId &&
            items.some((item) => item.id === previousSelectedId)
          ) {
            return previousSelectedId;
          }
          if (previousDraftSelection) {
            return null;
          }
          return items[0]?.id ?? null;
        });
      }
    },
    [draftSelectionRef, hasActiveTask, mountedRef, selectedConversationId],
  );

  const handleClearHistory = useCallback(async () => {
    if (hasActiveTask()) {
      toast.error("仍有图片任务在处理中，暂时不能清空历史记录");
      return;
    }

    try {
      await clearImageConversations();
      draftSelectionRef.current = true;
      conversationsRef.current = [];
      setConversations([]);
      setSelectedConversationId(null);
      toast.success("已清空历史记录");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "清空历史记录失败";
      toast.error(message);
    }
  }, [draftSelectionRef, hasActiveTask]);

  return {
    conversations,
    selectedConversationId,
    isLoadingHistory,
    setConversations,
    setSelectedConversationId,
    setIsLoadingHistory,
    focusConversation,
    openDraftConversation,
    refreshHistory,
    handleCreateDraft,
    handleDeleteConversation,
    handleClearHistory,
  };
}
