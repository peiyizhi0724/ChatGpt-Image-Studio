import { describe, expect, it } from "vitest";

import type { ImageTaskStreamEvent, ImageTaskView } from "@/lib/api";

import {
  buildActiveRequestState,
  buildEmptyTaskSnapshot,
  deriveTaskSnapshotFromItems,
  reduceTaskItems,
  selectConversationActiveTask,
} from "./task-runtime";

function makeTask(overrides: Partial<ImageTaskView>): ImageTaskView {
  return {
    id: "task-1",
    conversationId: "conv-1",
    turnId: "turn-1",
    mode: "generate",
    status: "queued",
    createdAt: "2026-04-27T10:00:00Z",
    count: 1,
    images: [],
    ...overrides,
  };
}

describe("task-runtime", () => {
  it("selectConversationActiveTask only tracks the selected conversation", () => {
    const activeTasks = [
      makeTask({
        id: "task-other",
        conversationId: "conv-other",
        turnId: "turn-other",
        createdAt: "2026-04-27T10:00:00Z",
      }),
      makeTask({
        id: "task-current-old",
        conversationId: "conv-current",
        turnId: "turn-current-old",
        createdAt: "2026-04-27T10:05:00Z",
      }),
      makeTask({
        id: "task-current-new",
        conversationId: "conv-current",
        turnId: "turn-current-new",
        createdAt: "2026-04-27T10:10:00Z",
        mode: "edit",
        count: 2,
      }),
    ];

    const task = selectConversationActiveTask(activeTasks, "conv-current");
    expect(task?.id).toBe("task-current-new");

    const activeRequest = buildActiveRequestState(task);
    expect(activeRequest).toEqual({
      conversationId: "conv-current",
      turnId: "turn-current-new",
      mode: "edit",
      count: 2,
      variant: "standard",
    });

    expect(selectConversationActiveTask(activeTasks, "conv-missing")).toBeNull();
    expect(selectConversationActiveTask(activeTasks, null)).toBeNull();
  });

  it("reduceTaskItems removes cleared tasks and keeps upserts ordered", () => {
    const first = makeTask({
      id: "task-a",
      conversationId: "conv-a",
      createdAt: "2026-04-27T10:00:00Z",
    });
    const second = makeTask({
      id: "task-b",
      conversationId: "conv-b",
      createdAt: "2026-04-27T10:10:00Z",
    });

    const upsertEvent: ImageTaskStreamEvent = {
      type: "task.upsert",
      task: makeTask({
        id: "task-c",
        conversationId: "conv-c",
        createdAt: "2026-04-27T10:05:00Z",
      }),
    };

    expect(reduceTaskItems([second, first], upsertEvent).map((task) => task.id)).toEqual([
      "task-a",
      "task-c",
      "task-b",
    ]);

    const removeEvent: ImageTaskStreamEvent = {
      type: "task.remove",
      taskId: "task-b",
    };

    expect(reduceTaskItems([first, second], removeEvent).map((task) => task.id)).toEqual([
      "task-a",
    ]);
  });

  it("buildEmptyTaskSnapshot returns the zeroed queue metrics shape", () => {
    expect(buildEmptyTaskSnapshot()).toEqual({
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
    });
  });

  it("deriveTaskSnapshotFromItems keeps queued and final counters in sync with visible tasks", () => {
    const base = {
      ...buildEmptyTaskSnapshot(),
      maxRunning: 8,
      retentionSeconds: 1800,
    };
    const snapshot = deriveTaskSnapshotFromItems(
      [
        makeTask({
          id: "task-running",
          conversationId: "conv-running",
          status: "running",
        }),
        makeTask({
          id: "task-queued",
          conversationId: "conv-queued",
          status: "queued",
        }),
        makeTask({
          id: "compat-task-done",
          conversationId: "",
          status: "succeeded",
        }),
      ],
      base,
    );

    expect(snapshot.maxRunning).toBe(8);
    expect(snapshot.retentionSeconds).toBe(1800);
    expect(snapshot.total).toBe(3);
    expect(snapshot.queued).toBe(1);
    expect(snapshot.activeSources.workspace).toBe(2);
    expect(snapshot.activeSources.compat).toBe(0);
    expect(snapshot.finalStatuses.succeeded).toBe(1);
  });
});
