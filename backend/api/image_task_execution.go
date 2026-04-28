package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"

	"chatgpt2api/handler"
	"chatgpt2api/internal/imagehistory"
)

const imageTaskFakeHost = "workspace.local"

type imageTaskDeferredError struct {
	cause error
}

func (e *imageTaskDeferredError) Error() string {
	if e == nil || e.cause == nil {
		return "image task deferred"
	}
	return e.cause.Error()
}

func (e *imageTaskDeferredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (s *Server) executeImageTaskUnit(ctx context.Context, taskID string, unitIndex int, lease *imageTaskLease) ([]imagehistory.Image, error) {
	if lease == nil || lease.auth == nil {
		return nil, fmt.Errorf("task lease is required")
	}
	task := s.imageTasks.copyTask(taskID)
	if task == nil {
		return nil, fmt.Errorf("task not found")
	}

	fakeReq := httptest.NewRequest("POST", "http://"+imageTaskFakeHost+"/api/image/tasks", nil).WithContext(ctx)
	metadata := newImageRequestMetadata(task.Prompt, task.Size, task.Quality)
	requestedModel := normalizeRequestedImageModel(task.Model, s.cfg.ChatGPT.Model)

	var (
		items     []map[string]any
		retryable bool
		err       error
	)

	switch {
	case task.SourceReference != nil:
		mask, imageFiles, err := s.resolveTaskEditInputs(task)
		if err != nil {
			return nil, err
		}
		if len(mask) == 0 {
			return nil, fmt.Errorf("selection edit mask is required")
		}
		items, retryable, err = s.runImageRequestWithAdmission(
			ctx,
			lease.auth,
			lease.account,
			lease.release,
			lease.decision,
			"selection-edit",
			task.ResponseFormat,
			true,
			requestedModel,
			false,
			metadata,
			func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
				return client.InpaintImageByMask(
					ctx,
					task.Prompt,
					upstreamModel,
					task.SourceReference.OriginalFileID,
					task.SourceReference.OriginalGenID,
					task.SourceReference.ConversationID,
					task.SourceReference.ParentMessageID,
					mask,
					task.Size,
					task.Quality,
				)
			},
			fakeReq,
			false,
		)
		_ = imageFiles
	case task.Mode == "edit" || len(task.SourceImages) > 0:
		mask, imageFiles, err := s.resolveTaskEditInputs(task)
		if err != nil {
			return nil, err
		}
		responsesEligible := handler.SupportsResponsesInlineEdit(imageFiles, mask)
		items, retryable, err = s.runImageRequestWithAdmission(
			ctx,
			lease.auth,
			lease.account,
			lease.release,
			lease.decision,
			"edit",
			task.ResponseFormat,
			false,
			requestedModel,
			responsesEligible,
			metadata,
			func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
				return client.EditImageByUpload(ctx, task.Prompt, upstreamModel, imageFiles, mask, task.Size, task.Quality)
			},
			fakeReq,
			false,
		)
	default:
		items, retryable, err = s.runImageRequestWithAdmission(
			ctx,
			lease.auth,
			lease.account,
			lease.release,
			lease.decision,
			"generate",
			task.ResponseFormat,
			false,
			requestedModel,
			true,
			metadata,
			func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
				return client.GenerateImage(ctx, task.Prompt, upstreamModel, 1, task.Size, task.Quality, task.Background)
			},
			fakeReq,
			false,
		)
	}

	lease.release = nil
	if err != nil {
		if retryable {
			return nil, &imageTaskDeferredError{cause: err}
		}
		return nil, err
	}
	return historyImagesFromResponseItems(items), nil
}

func historyImagesFromResponseItems(items []map[string]any) []imagehistory.Image {
	images := make([]imagehistory.Image, 0, len(items))
	for index, item := range items {
		url := strings.TrimSpace(stringValue(item["url"]))
		if strings.Contains(url, imageTaskFakeHost) {
			if parts := strings.SplitN(url, imageTaskFakeHost, 2); len(parts) == 2 {
				url = parts[1]
				if strings.HasPrefix(url, ":") {
					if slash := strings.Index(url, "/"); slash >= 0 {
						url = url[slash:]
					}
				}
			}
		}
		if !strings.HasPrefix(url, "/") && strings.Contains(url, "/v1/files/image/") {
			if slash := strings.Index(url, "/v1/files/image/"); slash >= 0 {
				url = url[slash:]
			}
		}
		images = append(images, imagehistory.Image{
			ID:              firstNonEmpty(stringValue(item["id"]), fmt.Sprintf("image-%d", index)),
			Status:          firstNonEmpty(stringValue(item["status"]), "success"),
			B64JSON:         stringValue(item["b64_json"]),
			URL:             url,
			RevisedPrompt:   stringValue(item["revised_prompt"]),
			FileID:          stringValue(item["file_id"]),
			GenID:           stringValue(item["gen_id"]),
			ConversationID:  stringValue(item["conversation_id"]),
			ParentMessageID: stringValue(item["parent_message_id"]),
			SourceAccountID: stringValue(item["source_account_id"]),
			Error:           stringValue(item["error"]),
		})
	}
	return images
}

func (s *Server) resolveTaskEditInputs(task *imageTask) ([]byte, [][]byte, error) {
	imageFiles := make([][]byte, 0)
	var mask []byte
	for _, source := range task.SourceImages {
		data, err := s.resolveTaskSourceImageBytes(source)
		if err != nil {
			return nil, nil, err
		}
		if source.Role == "mask" {
			mask = data
			continue
		}
		imageFiles = append(imageFiles, data)
	}
	return mask, imageFiles, nil
}

func (s *Server) resolveTaskSourceImageBytes(source imageTaskSourceImage) ([]byte, error) {
	if strings.TrimSpace(source.DataURL) != "" {
		payload, err := decodeTaskDataURL(strings.TrimSpace(source.DataURL))
		if err != nil {
			return nil, err
		}
		return payload, nil
	}
	rawURL := strings.TrimSpace(source.URL)
	if rawURL == "" {
		return nil, fmt.Errorf("source image is empty")
	}
	if index := strings.Index(rawURL, "/v1/files/image/"); index >= 0 {
		name := rawURL[index+len("/v1/files/image/"):]
		name = strings.ReplaceAll(name, "/", "-")
		path := s.resolveImageFilePath(name)
		if path == "" {
			return nil, fmt.Errorf("image not found")
		}
		return os.ReadFile(path)
	}
	resp, err := compatImageFetchClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch image returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func decodeTaskDataURL(raw string) ([]byte, error) {
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return nil, fmt.Errorf("invalid data url")
	}
	meta := raw[:comma]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return nil, fmt.Errorf("only base64 data urls are supported")
	}
	payload, err := base64.StdEncoding.DecodeString(raw[comma+1:])
	if err != nil {
		return nil, fmt.Errorf("decode data url: %w", err)
	}
	return payload, nil
}
