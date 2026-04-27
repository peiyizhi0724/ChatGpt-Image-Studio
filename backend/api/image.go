package api

import (
	"context"
	"net/http"

	"chatgpt2api/handler"
	"chatgpt2api/internal/imagehistory"
)

// buildImageResponse converts ImageResults to the OpenAI-compatible response format.
// Only includes url/b64_json and revised_prompt — no internal ChatGPT fields.
func buildImageResponse(r *http.Request, client imageDownloader, results []handler.ImageResult, responseFormat string, sourceAccountID string, cacheDir string) []map[string]any {
	return buildImageResponseItems(
		r.Context(),
		client,
		results,
		responseFormat,
		sourceAccountID,
		cacheDir,
		func(filename string) string {
			return gatewayImageURL(r, filename)
		},
	)
}

func buildImageHistoryImages(ctx context.Context, client imageDownloader, results []handler.ImageResult, responseFormat string, sourceAccountID string, cacheDir string) []imagehistory.Image {
	items := buildImageResponseItems(
		ctx,
		client,
		results,
		responseFormat,
		sourceAccountID,
		cacheDir,
		func(filename string) string {
			return "/v1/files/image/" + filename
		},
	)
	historyImages := make([]imagehistory.Image, 0, len(items))
	for index, item := range items {
		historyImages = append(historyImages, imagehistory.Image{
			ID:              firstNonEmpty(stringValue(item["id"]), stringValue(item["file_id"]), stringValue(item["gen_id"]), "image-"+stringValue(index)),
			Status:          "success",
			B64JSON:         stringValue(item["b64_json"]),
			URL:             stringValue(item["url"]),
			RevisedPrompt:   stringValue(item["revised_prompt"]),
			FileID:          stringValue(item["file_id"]),
			GenID:           stringValue(item["gen_id"]),
			ConversationID:  stringValue(item["conversation_id"]),
			ParentMessageID: stringValue(item["parent_message_id"]),
			SourceAccountID: stringValue(item["source_account_id"]),
			Error:           stringValue(item["error"]),
		})
	}
	return historyImages
}

func buildImageResponseItems(
	ctx context.Context,
	client imageDownloader,
	results []handler.ImageResult,
	responseFormat string,
	sourceAccountID string,
	cacheDir string,
	urlBuilder func(string) string,
) []map[string]any {
	data := make([]map[string]any, 0, len(results))
	for index, img := range results {
		item := map[string]any{
			"id":                firstNonEmpty(img.FileID, img.GenID, img.URL, "image"),
			"revised_prompt":    img.RevisedPrompt,
			"file_id":           img.FileID,
			"gen_id":            img.GenID,
			"conversation_id":   img.ConversationID,
			"parent_message_id": img.ParentMsgID,
		}
		if sourceAccountID != "" {
			item["source_account_id"] = sourceAccountID
		}
		if responseFormat == "b64_json" {
			b64, err := client.DownloadAsBase64(ctx, img.URL)
			if err != nil {
				item["url"] = img.URL
			} else {
				item["b64_json"] = b64
			}
		} else {
			filename, err := downloadAndCache(client, img.URL, cacheDir)
			if err != nil {
				item["url"] = img.URL
			} else {
				item["url"] = urlBuilder(filename)
			}
		}
		if item["id"] == "" {
			item["id"] = "image-" + stringValue(index)
		}
		data = append(data, item)
	}
	return data
}
