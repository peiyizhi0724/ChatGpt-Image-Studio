import type {
  ImageTaskSnapshot,
  ImageTaskStreamEvent,
  ImageTaskView,
} from "@/lib/api";
import type { ImageMode } from "@/store/image-conversations";

export type ActiveRequestState = {
  conversationId: string;
  turnId: string;
  mode: ImageMode;
  count: number;
  variant: "standard" | "selection-edit";
};

export function buildEmptyTaskSnapshot(): ImageTaskSnapshot {
  return {
    running: 0,
    maxRunning: 0,
    queued: 0,
    total: 0,
    activeSources: { workspace: 0, compat: 0 },
    finalStatuses: {
      succeeded: 0,
      failed: 0,
      cancelled: 0,
      expired: 0,
    },
    retentionSeconds: 0,
  };
}

export function deriveTaskSnapshotFromItems(
  items: ImageTaskView[],
  base: ImageTaskSnapshot,
): ImageTaskSnapshot {
  const next = buildEmptyTaskSnapshot();
  next.maxRunning = base.maxRunning;
  next.retentionSeconds = base.retentionSeconds;

  for (const item of items) {
    next.total += 1;
    switch (item.status) {
      case "queued":
        next.queued += 1;
        addActiveSource(next, item);
        break;
      case "running":
      case "cancel_requested":
        addActiveSource(next, item);
        break;
      case "succeeded":
        next.finalStatuses.succeeded += 1;
        break;
      case "failed":
        next.finalStatuses.failed += 1;
        break;
      case "cancelled":
        next.finalStatuses.cancelled += 1;
        break;
      case "expired":
        next.finalStatuses.expired += 1;
        break;
      default:
        break;
    }
  }

  next.running = next.activeSources.workspace + next.activeSources.compat - next.queued;
  if (next.running < 0) {
    next.running = 0;
  }

  return {
    ...base,
    queued: Math.max(base.queued, next.queued),
    total: Math.max(base.total, next.total),
    activeSources: {
      workspace: Math.max(
        base.activeSources.workspace,
        next.activeSources.workspace,
      ),
      compat: Math.max(base.activeSources.compat, next.activeSources.compat),
    },
    finalStatuses: next.finalStatuses,
    running: Math.max(base.running, next.running),
  };
}

export function reduceTaskItems(
  prev: ImageTaskView[],
  event: ImageTaskStreamEvent,
): ImageTaskView[] {
  if (event.type === "task.upsert" && event.task) {
    const next = [
      event.task,
      ...prev.filter((item) => item.id !== event.task?.id),
    ];
    return next.sort((left, right) =>
      left.createdAt.localeCompare(right.createdAt),
    );
  }
  if (event.type === "task.remove" && event.taskId) {
    return prev.filter((item) => item.id !== event.taskId);
  }
  return prev;
}

export function selectConversationActiveTask(
  activeTasks: ImageTaskView[],
  selectedConversationId: string | null,
): ImageTaskView | null {
  if (!selectedConversationId) {
    return null;
  }
  const matchingTasks = activeTasks.filter(
    (task) => task.conversationId === selectedConversationId,
  );
  if (matchingTasks.length === 0) {
    return null;
  }
  return [...matchingTasks].sort((left, right) =>
    right.createdAt.localeCompare(left.createdAt),
  )[0];
}

export function buildActiveRequestState(
  task: ImageTaskView | null,
): ActiveRequestState | null {
  if (!task) {
    return null;
  }
  return {
    conversationId: task.conversationId,
    turnId: task.turnId,
    mode: task.mode === "edit" ? "edit" : "generate",
    count: task.count,
    variant: "standard",
  };
}

function addActiveSource(snapshot: ImageTaskSnapshot, task: ImageTaskView) {
  if (task.conversationId) {
    if (task.id.startsWith("compat-") || task.conversationId === "") {
      snapshot.activeSources.compat += 1;
      return;
    }
  }
  if (task.id.startsWith("compat-")) {
    snapshot.activeSources.compat += 1;
    return;
  }
  snapshot.activeSources.workspace += 1;
}
