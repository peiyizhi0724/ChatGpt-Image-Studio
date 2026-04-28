package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"chatgpt2api/handler"
)

func TestCreateImageTaskRunsToSuccess(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	req := httptest.NewRequest(http.MethodPost, "/api/image/tasks", strings.NewReader(`{
		"conversationId":"conv-task-1",
		"turnId":"turn-task-1",
		"mode":"generate",
		"prompt":"draw a cat",
		"model":"gpt-image-2",
		"count":1,
		"size":"1248x1248",
		"quality":"high"
	}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.AuthKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Task struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Task.ID != "turn-task-1" {
		t.Fatalf("task id = %q, want turn-task-1", payload.Task.ID)
	}

	waitForTaskStatus(t, server, payload.Task.ID, imageTaskStatusSucceeded)

	task, _, err := server.imageTasks.getTask(payload.Task.ID)
	if err != nil {
		t.Fatalf("getTask() returned error: %v", err)
	}
	if len(task.Images) != 1 {
		t.Fatalf("task images = %d, want 1", len(task.Images))
	}
	if task.Images[0].URL == "" {
		t.Fatalf("task image url = empty, want cached file url")
	}
	if !strings.HasPrefix(task.Images[0].URL, "/v1/files/image/") {
		t.Fatalf("task image url = %q, want relative cached file url", task.Images[0].URL)
	}
	if recorder.officialCalls != 1 {
		t.Fatalf("officialCalls = %d, want 1", recorder.officialCalls)
	}
}

func TestCreateImageTaskSelectionEditBypassesPolicySnapshot(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Plus",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	accountID := compatPrimaryAccountID(t, server)
	req := httptest.NewRequest(http.MethodPost, "/api/image/tasks", strings.NewReader(`{
		"conversationId":"conv-selection-1",
		"turnId":"turn-selection-1",
		"mode":"edit",
		"prompt":"selection edit",
		"model":"gpt-image-2",
		"count":1,
		"sourceImages":[
			{
				"id":"mask-1",
				"role":"mask",
				"name":"mask.png",
				"dataUrl":"data:image/png;base64,aW1hZ2U="
			}
		],
		"sourceReference":{
			"original_file_id":"file-1",
			"original_gen_id":"gen-1",
			"conversation_id":"conv-1",
			"parent_message_id":"msg-1",
			"source_account_id":"`+accountID+`"
		},
		"policy":{
			"enabled":true,
			"sortMode":"imported_at",
			"groupSize":10,
			"enabledGroupIndexes":[999],
			"reserveMode":"daily_first_seen_percent",
			"reservePercent":20
		}
	}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.AuthKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	waitForTaskStatus(t, server, payload.Task.ID, imageTaskStatusSucceeded)
	if recorder.officialCalls != 1 {
		t.Fatalf("officialCalls = %d, want 1 source-bound official call", recorder.officialCalls)
	}
	if len(recorder.callSequence) != 1 {
		t.Fatalf("callSequence = %#v, want 1 entry", recorder.callSequence)
	}
	if !strings.Contains(recorder.callSequence[0], ":selection-edit") {
		t.Fatalf("callSequence[0] = %q, want selection-edit operation", recorder.callSequence[0])
	}
}

func TestCreateImageEditTaskHighResolutionUsesPaidAccount(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "free-priority.json",
				accessToken: "token-free-priority",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "paid-priority.json",
				accessToken: "token-paid-priority",
				accountType: "Plus",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
	})

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-edit-paid-1",
		TurnID:         "turn-edit-paid-1",
		Mode:           "edit",
		Prompt:         "high resolution edit",
		Model:          "gpt-image-2",
		Count:          1,
		Size:           "3840x2160",
		Quality:        "high",
		SourceImages: []imageTaskSourceImagePayload{
			{
				ID:      "source-1",
				Role:    "image",
				Name:    "source.png",
				DataURL: "data:image/png;base64,aW1hZ2U=",
			},
		},
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-edit-paid-1", imageTaskStatusSucceeded)
	if len(recorder.callSequence) == 0 {
		t.Fatal("callSequence = empty, want paid edit execution")
	}
	lastCall := recorder.callSequence[len(recorder.callSequence)-1]
	if !strings.Contains(lastCall, "token-paid-priority") {
		t.Fatalf("callSequence = %#v, want paid account selected for high-resolution edit", recorder.callSequence)
	}
}

func TestCreateImageGenerateTaskAutoFreeUsesFreeAccount(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "free-auto.json",
				accessToken: "token-free-auto",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "paid-auto.json",
				accessToken: "token-paid-auto",
				accountType: "Plus",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
	})

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID:   "conv-generate-auto-free-1",
		TurnID:           "turn-generate-auto-free-1",
		Mode:             "generate",
		Prompt:           "auto free generate",
		Model:            "gpt-image-2",
		Count:            1,
		Size:             "",
		ResolutionAccess: "free",
		Quality:          "high",
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-generate-auto-free-1", imageTaskStatusSucceeded)
	if len(recorder.callSequence) == 0 {
		t.Fatal("callSequence = empty, want free auto execution")
	}
	lastCall := recorder.callSequence[len(recorder.callSequence)-1]
	if !strings.Contains(lastCall, "token-free-auto") {
		t.Fatalf("callSequence = %#v, want free account selected for auto-free generate", recorder.callSequence)
	}
}

func TestCreateImageGenerateTaskAutoPaidUsesPaidAccount(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "free-priority-auto-paid.json",
				accessToken: "token-free-priority-auto-paid",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "paid-priority-auto-paid.json",
				accessToken: "token-paid-priority-auto-paid",
				accountType: "Plus",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
	})

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID:   "conv-generate-auto-paid-1",
		TurnID:           "turn-generate-auto-paid-1",
		Mode:             "generate",
		Prompt:           "auto paid generate",
		Model:            "gpt-image-2",
		Count:            1,
		Size:             "",
		ResolutionAccess: "paid",
		Quality:          "high",
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-generate-auto-paid-1", imageTaskStatusSucceeded)
	if len(recorder.callSequence) == 0 {
		t.Fatal("callSequence = empty, want paid auto execution")
	}
	lastCall := recorder.callSequence[len(recorder.callSequence)-1]
	if !strings.Contains(lastCall, "token-paid-priority-auto-paid") {
		t.Fatalf("callSequence = %#v, want paid account selected for auto-paid generate", recorder.callSequence)
	}
}

func TestCreateImageGenerateTaskAutoPaidRequiresPaidAccount(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "free-only-auto-paid.json",
				accessToken: "token-free-only-auto-paid",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
		},
	})

	_, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID:   "conv-generate-auto-paid-no-paid",
		TurnID:           "turn-generate-auto-paid-no-paid",
		Mode:             "generate",
		Prompt:           "auto paid without paid account",
		Model:            "gpt-image-2",
		Count:            1,
		Size:             "",
		ResolutionAccess: "paid",
		Quality:          "high",
	})
	if err == nil {
		t.Fatal("createTask() returned nil error, want paid account validation failure")
	}
	var reqErr *requestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("createTask() error = %T, want *requestError", err)
	}
	if reqErr.code != "paid_resolution_requires_paid_account" {
		t.Fatalf("request error code = %q, want paid_resolution_requires_paid_account", reqErr.code)
	}
}

func waitForTaskStatus(t *testing.T, server *Server, taskID string, want imageTaskStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, _, err := server.imageTasks.getTask(taskID)
		if err == nil && task != nil && imageTaskStatus(task.Status) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	task, _, err := server.imageTasks.getTask(taskID)
	if err != nil {
		t.Fatalf("getTask(%s) returned error: %v", taskID, err)
	}
	t.Fatalf("task %s status = %q, want %q", taskID, task.Status, want)
}

func TestImageTaskStreamWritesInitPayload(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-stream-1",
		TurnID:         "turn-stream-1",
		Mode:           "generate",
		Prompt:         "stream init",
		Model:          "gpt-image-2",
		Count:          1,
		Size:           "1248x1248",
		Quality:        "high",
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/image/tasks/stream", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.AuthKey)
	rec := httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Handler().ServeHTTP(rec, req)
	}()

	time.Sleep(80 * time.Millisecond)
	cancel()
	wg.Wait()

	body := rec.Body.String()
	if !strings.Contains(body, "event: init") {
		t.Fatalf("stream body = %q, want init event", body)
	}
	if !strings.Contains(body, `"turnId":"turn-stream-1"`) {
		t.Fatalf("stream body = %q, want queued task in init payload", body)
	}
	if !strings.Contains(body, `"snapshot"`) {
		t.Fatalf("stream body = %q, want snapshot payload", body)
	}
}

func TestSchedulePublishesQueuedBlockerUpdatesWhenConcurrencyIsFull(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	task, err := server.imageTasks.newTask(createImageTaskRequest{
		ConversationID: "conv-blocker-1",
		TurnID:         "turn-blocker-1",
		Mode:           "generate",
		Prompt:         "blocked by concurrency",
		Model:          "gpt-image-2",
		Count:          1,
	})
	if err != nil {
		t.Fatalf("newTask() returned error: %v", err)
	}

	server.imageTasks.mu.Lock()
	server.imageTasks.tasks[task.ID] = task
	server.imageTasks.order = append(server.imageTasks.order, task.ID)
	server.imageTasks.runningUnits = server.imageTasks.maxRunningLocked()
	server.imageTasks.mu.Unlock()

	subID, ch := server.imageTasks.subscribe()
	defer server.imageTasks.unsubscribe(subID)

	if server.imageTasks.tryScheduleOne() {
		t.Fatal("tryScheduleOne() = true, want no work scheduled when concurrency is full")
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for task.upsert blocker update")
		case event := <-ch:
			if event.Type != "task.upsert" || event.Task == nil {
				continue
			}
			if event.Task.ID != task.ID {
				continue
			}
			if event.Task.WaitingReason != imageTaskWaitingReasonGlobalConcurrency {
				t.Fatalf("WaitingReason = %q, want %q", event.Task.WaitingReason, imageTaskWaitingReasonGlobalConcurrency)
			}
			if len(event.Task.Blockers) == 0 || event.Task.Blockers[0].Code != string(imageTaskWaitingReasonGlobalConcurrency) {
				t.Fatalf("Blockers = %#v, want global concurrency blocker", event.Task.Blockers)
			}
			return
		}
	}
}

func TestCreateImageTaskRetriesRateLimitedAccount(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "limited.json",
				accessToken: "token-limited",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "healthy.json",
				accessToken: "token-healthy",
				accountType: "Free",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
		behavior: compatClientBehavior{
			officialGenerateErrors: map[string]error{
				"token-limited": errors.New("backend-api failed: HTTP 429 too many requests"),
			},
		},
	})

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-retry-1",
		TurnID:         "turn-retry-1",
		Mode:           "generate",
		Prompt:         "retry please",
		Model:          "gpt-image-2",
		Count:          1,
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-retry-1", imageTaskStatusSucceeded)

	task, _, err := server.imageTasks.getTask("turn-retry-1")
	if err != nil {
		t.Fatalf("getTask() returned error: %v", err)
	}
	if len(task.Images) != 1 || task.Images[0].URL == "" {
		t.Fatalf("task images = %#v, want successful cached image", task.Images)
	}
	if len(recorder.callSequence) < 2 {
		t.Fatalf("callSequence = %#v, want limited then fallback attempt", recorder.callSequence)
	}
	if !strings.Contains(recorder.callSequence[0], "token-limited") {
		t.Fatalf("callSequence[0] = %q, want first limited account", recorder.callSequence[0])
	}
	if !strings.Contains(recorder.callSequence[len(recorder.callSequence)-1], "token-healthy") {
		t.Fatalf("callSequence = %#v, want fallback healthy account", recorder.callSequence)
	}
}

func TestCreateImageTaskRetriesTransientResponsesSSEError(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Plus",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "transient-paid.json",
				accessToken: "token-transient-paid",
				accountType: "Plus",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
		},
	})

	var transientCalls int32
	server.responsesClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = proxyURL
		_ = authData
		_ = requestConfig
		recorder.responsesCalls++
		errOnce := error(nil)
		if accessToken == "token-transient-paid" && atomic.AddInt32(&transientCalls, 1) == 1 {
			errOnce = errors.New("responses SSE read error: stream error: stream ID 1; INTERNAL_ERROR; received from peer")
		}
		return &compatStubWorkflowClient{
			factory:     "responses",
			token:       accessToken,
			recorder:    recorder,
			generateErr: errOnce,
		}
	}

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-transient-1",
		TurnID:         "turn-transient-1",
		Mode:           "generate",
		Prompt:         "retry transient stream",
		Model:          "gpt-image-2",
		Count:          1,
		Size:           "2048x2048",
		Quality:        "high",
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskPredicate(t, server, "turn-transient-1", func(task *imageTaskView) bool {
		return task.Status == imageTaskStatusSucceeded
	})

	task, _, err := server.imageTasks.getTask("turn-transient-1")
	if err != nil {
		t.Fatalf("getTask() returned error: %v", err)
	}
	if len(task.Images) != 1 || task.Images[0].URL == "" {
		t.Fatalf("task images = %#v, want successful cached image", task.Images)
	}
	if got := atomic.LoadInt32(&transientCalls); got < 2 {
		t.Fatalf("transientCalls = %d, want retry after first SSE failure", got)
	}
}

func TestCancelRunningImageTaskCancelsQueuedUnits(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	var active int32
	var maxActive int32
	server.officialClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = proxyURL
		_ = authData
		_ = requestConfig
		return &parallelGenerateWorkflowClient{
			token:     accessToken,
			active:    &active,
			maxActive: &maxActive,
			delay:     180 * time.Millisecond,
		}
	}

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-cancel-1",
		TurnID:         "turn-cancel-1",
		Mode:           "generate",
		Prompt:         "cancel me",
		Model:          "gpt-image-2",
		Count:          3,
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskPredicate(t, server, "turn-cancel-1", func(task *imageTaskView) bool {
		return task.Status == imageTaskStatusRunning
	})

	if _, err := server.imageTasks.cancelTask("turn-cancel-1"); err != nil {
		t.Fatalf("cancelTask() returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-cancel-1", imageTaskStatusCancelled)

	task, _, err := server.imageTasks.getTask("turn-cancel-1")
	if err != nil {
		t.Fatalf("getTask() returned error: %v", err)
	}
	cancelledUnits := 0
	for _, image := range task.Images {
		if image.Error == "任务已取消" {
			cancelledUnits++
		}
	}
	if cancelledUnits < 1 {
		t.Fatalf("task images = %#v, want at least one queued unit cancelled", task.Images)
	}
}

func TestCancelRunningImageTaskInterruptsUpstreamRequest(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	server.officialClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = proxyURL
		_ = authData
		_ = requestConfig
		return &parallelGenerateWorkflowClient{
			token:     accessToken,
			active:    new(int32),
			maxActive: new(int32),
			delay:     5 * time.Second,
		}
	}

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-cancel-fast-1",
		TurnID:         "turn-cancel-fast-1",
		Mode:           "generate",
		Prompt:         "cancel fast",
		Model:          "gpt-image-2",
		Count:          1,
	}); err != nil {
		t.Fatalf("createTask() returned error: %v", err)
	}

	waitForTaskPredicate(t, server, "turn-cancel-fast-1", func(task *imageTaskView) bool {
		return task.Status == imageTaskStatusRunning
	})

	startedAt := time.Now()
	if _, err := server.imageTasks.cancelTask("turn-cancel-fast-1"); err != nil {
		t.Fatalf("cancelTask() returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-cancel-fast-1", imageTaskStatusCancelled)

	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("cancel finished in %s, want running request to stop promptly", elapsed)
	}
}

func TestDeferredQueuedUnitSchedulesRetryWakeup(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	task, err := server.imageTasks.newTask(createImageTaskRequest{
		ConversationID: "conv-backoff-1",
		TurnID:         "turn-backoff-1",
		Mode:           "generate",
		Prompt:         "backoff",
		Model:          "gpt-image-2",
		Count:          1,
	})
	if err != nil {
		t.Fatalf("newTask() returned error: %v", err)
	}

	server.imageTasks.mu.Lock()
	task.Units[0].NextAttemptAt = time.Now().Add(3 * time.Second)
	server.imageTasks.tasks[task.ID] = task
	server.imageTasks.order = append(server.imageTasks.order, task.ID)
	server.imageTasks.mu.Unlock()

	if server.imageTasks.tryScheduleOne() {
		t.Fatal("tryScheduleOne() = true, want deferred task to stay queued")
	}

	server.imageTasks.mu.Lock()
	defer server.imageTasks.mu.Unlock()
	if server.imageTasks.scheduleAt.IsZero() {
		t.Fatal("scheduleAt = zero, want retry wakeup to be scheduled")
	}
	if !server.imageTasks.scheduleAt.After(time.Now()) {
		t.Fatalf("scheduleAt = %s, want future retry wakeup", server.imageTasks.scheduleAt)
	}
}

func TestQueuedImageTaskExpiresBeforeFirstRun(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	server.cfg.Server.MaxImageConcurrency = 1
	server.cfg.Server.ImageTaskQueueTTLSeconds = 1
	server.officialClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = proxyURL
		_ = authData
		_ = requestConfig
		return &parallelGenerateWorkflowClient{
			token:     accessToken,
			active:    new(int32),
			maxActive: new(int32),
			delay:     1500 * time.Millisecond,
		}
	}

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-expire-runner",
		TurnID:         "turn-expire-runner",
		Mode:           "generate",
		Prompt:         "occupy slot",
		Model:          "gpt-image-2",
		Count:          1,
	}); err != nil {
		t.Fatalf("createTask(runner) returned error: %v", err)
	}
	waitForTaskPredicate(t, server, "turn-expire-runner", func(task *imageTaskView) bool {
		return task.Status == imageTaskStatusRunning
	})

	if _, err := server.imageTasks.createTask(createImageTaskRequest{
		ConversationID: "conv-expire-queued",
		TurnID:         "turn-expire-queued",
		Mode:           "generate",
		Prompt:         "should expire",
		Model:          "gpt-image-2",
		Count:          1,
	}); err != nil {
		t.Fatalf("createTask(queued) returned error: %v", err)
	}

	waitForTaskStatus(t, server, "turn-expire-queued", imageTaskStatusExpired)

	task, _, err := server.imageTasks.getTask("turn-expire-queued")
	if err != nil {
		t.Fatalf("getTask() returned error: %v", err)
	}
	if task.Error == "" {
		t.Fatal("expired task error = empty, want timeout message")
	}
	if len(task.Images) != 1 || task.Images[0].Error == "" {
		t.Fatalf("task images = %#v, want queued image marked as error", task.Images)
	}
}

func TestCompletedImageTaskIsPrunedAfterRetention(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	task, err := server.imageTasks.newTask(createImageTaskRequest{
		ConversationID: "conv-prune-1",
		TurnID:         "turn-prune-1",
		Mode:           "generate",
		Prompt:         "prune me",
		Model:          "gpt-image-2",
		Count:          1,
	})
	if err != nil {
		t.Fatalf("newTask() returned error: %v", err)
	}

	server.imageTasks.mu.Lock()
	task.Status = imageTaskStatusSucceeded
	task.FinishedAt = time.Now().Add(-imageTaskRetentionAfterFinish - time.Minute)
	server.imageTasks.tasks[task.ID] = task
	server.imageTasks.order = append(server.imageTasks.order, task.ID)
	server.imageTasks.mu.Unlock()

	if !server.imageTasks.tryScheduleOne() {
		t.Fatal("tryScheduleOne() = false, want prune cycle to run")
	}

	if _, _, err := server.imageTasks.getTask(task.ID); err == nil {
		t.Fatalf("getTask(%s) error = nil, want pruned task to be removed", task.ID)
	}
	items, snapshot := server.imageTasks.listTasks()
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 after pruning", len(items))
	}
	if snapshot.Total != 0 {
		t.Fatalf("snapshot.Total = %d, want 0 after pruning", snapshot.Total)
	}
}

func TestTaskSnapshotCountsQueuedUnitsWithinRunningParentTask(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	task, err := server.imageTasks.newTask(createImageTaskRequest{
		ConversationID: "conv-parent-1",
		TurnID:         "turn-parent-1",
		Mode:           "generate",
		Prompt:         "multi image",
		Model:          "gpt-image-2",
		Count:          6,
	})
	if err != nil {
		t.Fatalf("newTask() returned error: %v", err)
	}

	server.imageTasks.mu.Lock()
	task.Status = imageTaskStatusRunning
	task.ActiveUnits = 2
	task.Units[0].Status = imageTaskStatusRunning
	task.Units[1].Status = imageTaskStatusRunning
	server.imageTasks.tasks[task.ID] = task
	server.imageTasks.order = append(server.imageTasks.order, task.ID)
	server.imageTasks.runningUnits = 2
	snapshot := server.imageTasks.snapshotLocked()
	server.imageTasks.mu.Unlock()

	if snapshot.Running != 2 {
		t.Fatalf("snapshot.Running = %d, want 2", snapshot.Running)
	}
	if snapshot.Queued != 4 {
		t.Fatalf("snapshot.Queued = %d, want 4 queued units", snapshot.Queued)
	}
	if snapshot.ActiveSources.Workspace != 6 {
		t.Fatalf("snapshot.ActiveSources.Workspace = %d, want 6 active units", snapshot.ActiveSources.Workspace)
	}
}

func TestTaskQueuePositionCountsQueuedUnits(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	firstTask, err := server.imageTasks.newTask(createImageTaskRequest{
		ConversationID: "conv-queue-a",
		TurnID:         "turn-queue-a",
		Mode:           "generate",
		Prompt:         "first queued parent",
		Model:          "gpt-image-2",
		Count:          3,
	})
	if err != nil {
		t.Fatalf("newTask(first) returned error: %v", err)
	}
	secondTask, err := server.imageTasks.newTask(createImageTaskRequest{
		ConversationID: "conv-queue-b",
		TurnID:         "turn-queue-b",
		Mode:           "generate",
		Prompt:         "second queued parent",
		Model:          "gpt-image-2",
		Count:          2,
	})
	if err != nil {
		t.Fatalf("newTask(second) returned error: %v", err)
	}

	server.imageTasks.mu.Lock()
	server.imageTasks.tasks[firstTask.ID] = firstTask
	server.imageTasks.tasks[secondTask.ID] = secondTask
	server.imageTasks.order = append(server.imageTasks.order, firstTask.ID, secondTask.ID)
	view := server.imageTasks.buildTaskViewLocked(secondTask)
	server.imageTasks.mu.Unlock()

	if view.QueuePosition != 4 {
		t.Fatalf("QueuePosition = %d, want 4 because 3 queued units are ahead", view.QueuePosition)
	}
}

func waitForTaskPredicate(t *testing.T, server *Server, taskID string, predicate func(*imageTaskView) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, _, err := server.imageTasks.getTask(taskID)
		if err == nil && task != nil && predicate(task) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	task, _, err := server.imageTasks.getTask(taskID)
	if err != nil {
		t.Fatalf("getTask(%s) returned error: %v", taskID, err)
	}
	t.Fatalf("task %s did not satisfy predicate, current status = %q", taskID, task.Status)
}
