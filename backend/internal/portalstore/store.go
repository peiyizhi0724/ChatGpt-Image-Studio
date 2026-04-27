package portalstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgpt2api/internal/config"
	"chatgpt2api/internal/users"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("gallery work not found")

const defaultAssetMIME = "image/png"

type Store struct {
	db       *sql.DB
	imageDir string
}

type UserUsage struct {
	ImageRequests   int `json:"image_requests"`
	GeneratedImages int `json:"generated_images"`
	PublishedWorks  int `json:"published_works"`
}

type PublishInput struct {
	UserID          string
	UserEmail       string
	UserDisplayName string
	UserAvatarURL   string
	Title           string
	Prompt          string
	ImageDataURL    string
	ImageURL        string
	Model           string
	Size            string
}

type GalleryWork struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	UserEmail       string `json:"user_email"`
	UserDisplayName string `json:"user_display_name"`
	UserAvatarURL   string `json:"user_avatar_url"`
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	ImageURL        string `json:"image_url"`
	Model           string `json:"model"`
	Size            string `json:"size"`
	LikeCount       int    `json:"like_count"`
	CommentCount    int    `json:"comment_count"`
	CreatedAt       string `json:"created_at"`
	LikedByViewer   bool   `json:"liked_by_viewer"`
}

type GalleryComment struct {
	ID              string `json:"id"`
	WorkID          string `json:"work_id"`
	UserID          string `json:"user_id"`
	UserEmail       string `json:"user_email"`
	UserDisplayName string `json:"user_display_name"`
	UserAvatarURL   string `json:"user_avatar_url"`
	Content         string `json:"content"`
	CreatedAt       string `json:"created_at"`
}

type GalleryListOptions struct {
	ViewerUserID string
	Search       string
	Sort         string
	Limit        int
}

type LikeToggleResult struct {
	Liked     bool `json:"liked"`
	LikeCount int  `json:"like_count"`
}

type execContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewStore(cfg *config.Config) (*Store, error) {
	path := cfg.ResolvePath(cfg.Storage.SQLitePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:       db,
		imageDir: cfg.ResolvePath(cfg.Storage.ImageDir),
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS portal_user_usage (
			user_id TEXT PRIMARY KEY,
			image_requests INTEGER NOT NULL DEFAULT 0,
			generated_images INTEGER NOT NULL DEFAULT 0,
			published_works INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS portal_gallery_works (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_email TEXT NOT NULL,
			title TEXT NOT NULL,
			prompt TEXT NOT NULL,
			image_filename TEXT NOT NULL,
			image_url TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			size TEXT NOT NULL DEFAULT '',
			like_count INTEGER NOT NULL DEFAULT 0,
			comment_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portal_gallery_works_created_at ON portal_gallery_works(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_portal_gallery_works_user_id ON portal_gallery_works(user_id);`,
		`CREATE TABLE IF NOT EXISTS portal_gallery_likes (
			work_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (work_id, user_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portal_gallery_likes_work_id ON portal_gallery_likes(work_id);`,
		`CREATE TABLE IF NOT EXISTS portal_gallery_comments (
			id TEXT PRIMARY KEY,
			work_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			user_email TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portal_gallery_comments_work_id ON portal_gallery_comments(work_id, created_at ASC);`,
	}

	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListUsage(ctx context.Context) (map[string]UserUsage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, image_requests, generated_images, published_works FROM portal_user_usage`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]UserUsage{}
	for rows.Next() {
		var userID string
		var usage UserUsage
		if err := rows.Scan(&userID, &usage.ImageRequests, &usage.GeneratedImages, &usage.PublishedWorks); err != nil {
			return nil, err
		}
		result[strings.TrimSpace(userID)] = usage
	}
	return result, rows.Err()
}

func (s *Store) IncrementUsage(ctx context.Context, userID string, imageRequests, generatedImages, publishedWorks int) error {
	return incrementUsage(ctx, s.db, userID, imageRequests, generatedImages, publishedWorks)
}

func incrementUsage(ctx context.Context, executor execContext, userID string, imageRequests, generatedImages, publishedWorks int) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	if imageRequests == 0 && generatedImages == 0 && publishedWorks == 0 {
		return nil
	}
	_, err := executor.ExecContext(ctx, `
		INSERT INTO portal_user_usage (user_id, image_requests, generated_images, published_works, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			image_requests = portal_user_usage.image_requests + excluded.image_requests,
			generated_images = portal_user_usage.generated_images + excluded.generated_images,
			published_works = portal_user_usage.published_works + excluded.published_works,
			updated_at = excluded.updated_at
	`, userID, imageRequests, generatedImages, publishedWorks, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) PublishWork(ctx context.Context, input PublishInput) (GalleryWork, error) {
	userID := strings.TrimSpace(input.UserID)
	userEmail := strings.TrimSpace(input.UserEmail)
	prompt := strings.TrimSpace(input.Prompt)
	if userID == "" || userEmail == "" {
		return GalleryWork{}, fmt.Errorf("user is required")
	}
	if prompt == "" {
		return GalleryWork{}, fmt.Errorf("prompt is required")
	}

	imageURL, imageFilename, err := s.resolvePublishedImage(input)
	if err != nil {
		return GalleryWork{}, err
	}

	work := GalleryWork{
		ID:              uuid.NewString(),
		UserID:          userID,
		UserEmail:       userEmail,
		UserDisplayName: firstNonEmpty(strings.TrimSpace(input.UserDisplayName), userEmail),
		UserAvatarURL:   strings.TrimSpace(input.UserAvatarURL),
		Title:           firstNonEmpty(strings.TrimSpace(input.Title), buildWorkTitle(prompt)),
		Prompt:          prompt,
		ImageURL:        imageURL,
		Model:           strings.TrimSpace(input.Model),
		Size:            strings.TrimSpace(input.Size),
		LikeCount:       0,
		CommentCount:    0,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		LikedByViewer:   false,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GalleryWork{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portal_gallery_works (
			id, user_id, user_email, title, prompt, image_filename, image_url, model, size, like_count, comment_count, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)
	`, work.ID, work.UserID, work.UserEmail, work.Title, work.Prompt, imageFilename, work.ImageURL, work.Model, work.Size, work.CreatedAt, work.CreatedAt); err != nil {
		return GalleryWork{}, err
	}

	if err := incrementUsage(ctx, tx, userID, 0, 0, 1); err != nil {
		return GalleryWork{}, err
	}

	if err := tx.Commit(); err != nil {
		return GalleryWork{}, err
	}
	return work, nil
}

func (s *Store) ListWorks(ctx context.Context, options GalleryListOptions) ([]GalleryWork, error) {
	limit := options.Limit
	if limit <= 0 || limit > 120 {
		limit = 60
	}
	search := strings.TrimSpace(options.Search)
	searchLike := likePattern(search)
	orderBy := "w.created_at DESC"
	switch strings.ToLower(strings.TrimSpace(options.Sort)) {
	case "likes":
		orderBy = "w.like_count DESC, w.created_at DESC"
	case "comments":
		orderBy = "w.comment_count DESC, w.created_at DESC"
	}

	query := fmt.Sprintf(`
		SELECT
			w.id,
			w.user_id,
			w.user_email,
			COALESCE(NULLIF(TRIM(u.display_name), ''), w.user_email) AS user_display_name,
			COALESCE(NULLIF(TRIM(u.avatar_url), ''), '') AS user_avatar_url,
			w.title,
			w.prompt,
			w.image_url,
			w.model,
			w.size,
			w.like_count,
			w.comment_count,
			w.created_at,
			CASE
				WHEN ? <> '' AND EXISTS (
					SELECT 1 FROM portal_gallery_likes l WHERE l.work_id = w.id AND l.user_id = ?
				) THEN 1
				ELSE 0
			END AS liked_by_viewer
		FROM portal_gallery_works w
		LEFT JOIN portal_users u ON u.id = w.user_id
		WHERE (
			? = ''
			OR w.title LIKE ? ESCAPE '\'
			OR w.prompt LIKE ? ESCAPE '\'
			OR w.user_email LIKE ? ESCAPE '\'
			OR COALESCE(NULLIF(TRIM(u.display_name), ''), w.user_email) LIKE ? ESCAPE '\'
		)
		ORDER BY %s
		LIMIT ?
	`, orderBy)

	rows, err := s.db.QueryContext(
		ctx,
		query,
		strings.TrimSpace(options.ViewerUserID),
		strings.TrimSpace(options.ViewerUserID),
		search,
		searchLike,
		searchLike,
		searchLike,
		searchLike,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]GalleryWork, 0)
	for rows.Next() {
		var item GalleryWork
		var liked int
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.UserEmail,
			&item.UserDisplayName,
			&item.UserAvatarURL,
			&item.Title,
			&item.Prompt,
			&item.ImageURL,
			&item.Model,
			&item.Size,
			&item.LikeCount,
			&item.CommentCount,
			&item.CreatedAt,
			&liked,
		); err != nil {
			return nil, err
		}
		item.LikedByViewer = liked != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetWork(ctx context.Context, workID, viewerUserID string) (GalleryWork, []GalleryComment, error) {
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return GalleryWork{}, nil, ErrNotFound
	}

	var item GalleryWork
	var liked int
	err := s.db.QueryRowContext(ctx, `
		SELECT
			w.id,
			w.user_id,
			w.user_email,
			COALESCE(NULLIF(TRIM(u.display_name), ''), w.user_email) AS user_display_name,
			COALESCE(NULLIF(TRIM(u.avatar_url), ''), '') AS user_avatar_url,
			w.title,
			w.prompt,
			w.image_url,
			w.model,
			w.size,
			w.like_count,
			w.comment_count,
			w.created_at,
			CASE
				WHEN ? <> '' AND EXISTS (
					SELECT 1 FROM portal_gallery_likes l WHERE l.work_id = w.id AND l.user_id = ?
				) THEN 1
				ELSE 0
			END AS liked_by_viewer
		FROM portal_gallery_works w
		LEFT JOIN portal_users u ON u.id = w.user_id
		WHERE w.id = ?
	`, strings.TrimSpace(viewerUserID), strings.TrimSpace(viewerUserID), workID).Scan(
		&item.ID,
		&item.UserID,
		&item.UserEmail,
		&item.UserDisplayName,
		&item.UserAvatarURL,
		&item.Title,
		&item.Prompt,
		&item.ImageURL,
		&item.Model,
		&item.Size,
		&item.LikeCount,
		&item.CommentCount,
		&item.CreatedAt,
		&liked,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GalleryWork{}, nil, ErrNotFound
	}
	if err != nil {
		return GalleryWork{}, nil, err
	}
	item.LikedByViewer = liked != 0

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			c.id,
			c.work_id,
			c.user_id,
			c.user_email,
			COALESCE(NULLIF(TRIM(u.display_name), ''), c.user_email) AS user_display_name,
			COALESCE(NULLIF(TRIM(u.avatar_url), ''), '') AS user_avatar_url,
			c.content,
			c.created_at
		FROM portal_gallery_comments c
		LEFT JOIN portal_users u ON u.id = c.user_id
		WHERE work_id = ?
		ORDER BY created_at ASC
	`, workID)
	if err != nil {
		return GalleryWork{}, nil, err
	}
	defer rows.Close()

	comments := make([]GalleryComment, 0)
	for rows.Next() {
		var comment GalleryComment
		if err := rows.Scan(
			&comment.ID,
			&comment.WorkID,
			&comment.UserID,
			&comment.UserEmail,
			&comment.UserDisplayName,
			&comment.UserAvatarURL,
			&comment.Content,
			&comment.CreatedAt,
		); err != nil {
			return GalleryWork{}, nil, err
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return GalleryWork{}, nil, err
	}

	return item, comments, nil
}

func (s *Store) ToggleLike(ctx context.Context, workID, userID string) (LikeToggleResult, error) {
	workID = strings.TrimSpace(workID)
	userID = strings.TrimSpace(userID)
	if workID == "" || userID == "" {
		return LikeToggleResult{}, fmt.Errorf("work and user are required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LikeToggleResult{}, err
	}
	defer tx.Rollback()

	exists, err := workExistsTx(ctx, tx, workID)
	if err != nil {
		return LikeToggleResult{}, err
	}
	if !exists {
		return LikeToggleResult{}, ErrNotFound
	}

	var alreadyLiked int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM portal_gallery_likes WHERE work_id = ? AND user_id = ?`, workID, userID).Scan(&alreadyLiked); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return LikeToggleResult{}, err
	}

	liked := false
	delta := 1
	if alreadyLiked == 1 {
		delta = -1
		if _, err := tx.ExecContext(ctx, `DELETE FROM portal_gallery_likes WHERE work_id = ? AND user_id = ?`, workID, userID); err != nil {
			return LikeToggleResult{}, err
		}
	} else {
		liked = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO portal_gallery_likes (work_id, user_id, created_at) VALUES (?, ?, ?)`, workID, userID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return LikeToggleResult{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE portal_gallery_works
		SET like_count = CASE WHEN like_count + ? < 0 THEN 0 ELSE like_count + ? END, updated_at = ?
		WHERE id = ?
	`, delta, delta, time.Now().UTC().Format(time.RFC3339Nano), workID); err != nil {
		return LikeToggleResult{}, err
	}

	var likeCount int
	if err := tx.QueryRowContext(ctx, `SELECT like_count FROM portal_gallery_works WHERE id = ?`, workID).Scan(&likeCount); err != nil {
		return LikeToggleResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return LikeToggleResult{}, err
	}
	return LikeToggleResult{Liked: liked, LikeCount: likeCount}, nil
}

func (s *Store) AddComment(ctx context.Context, workID string, user users.PublicUser, content string) (GalleryComment, int, error) {
	workID = strings.TrimSpace(workID)
	content = strings.TrimSpace(content)
	if workID == "" {
		return GalleryComment{}, 0, fmt.Errorf("work is required")
	}
	if content == "" {
		return GalleryComment{}, 0, fmt.Errorf("comment is required")
	}
	if len([]rune(content)) > 500 {
		return GalleryComment{}, 0, fmt.Errorf("comment is too long")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GalleryComment{}, 0, err
	}
	defer tx.Rollback()

	exists, err := workExistsTx(ctx, tx, workID)
	if err != nil {
		return GalleryComment{}, 0, err
	}
	if !exists {
		return GalleryComment{}, 0, ErrNotFound
	}

	comment := GalleryComment{
		ID:              uuid.NewString(),
		WorkID:          workID,
		UserID:          strings.TrimSpace(user.ID),
		UserEmail:       strings.TrimSpace(user.Email),
		UserDisplayName: firstNonEmpty(strings.TrimSpace(user.DisplayName), strings.TrimSpace(user.Email)),
		UserAvatarURL:   strings.TrimSpace(user.AvatarURL),
		Content:         content,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portal_gallery_comments (id, work_id, user_id, user_email, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, comment.ID, comment.WorkID, comment.UserID, comment.UserEmail, comment.Content, comment.CreatedAt); err != nil {
		return GalleryComment{}, 0, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE portal_gallery_works
		SET comment_count = comment_count + 1, updated_at = ?
		WHERE id = ?
	`, time.Now().UTC().Format(time.RFC3339Nano), workID); err != nil {
		return GalleryComment{}, 0, err
	}

	var commentCount int
	if err := tx.QueryRowContext(ctx, `SELECT comment_count FROM portal_gallery_works WHERE id = ?`, workID).Scan(&commentCount); err != nil {
		return GalleryComment{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return GalleryComment{}, 0, err
	}
	return comment, commentCount, nil
}

func workExistsTx(ctx context.Context, tx *sql.Tx, workID string) (bool, error) {
	var existing string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM portal_gallery_works WHERE id = ?`, workID).Scan(&existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) resolvePublishedImage(input PublishInput) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input.ImageDataURL)), "data:") {
		return s.saveDataURLAsset(input.ImageDataURL, "gallery", input.Title)
	}

	filename := filenameFromImageURL(input.ImageURL)
	if filename == "" {
		return "", "", fmt.Errorf("image data is required")
	}

	path := filepath.Join(s.imageDir, filename)
	if _, err := os.Stat(path); err != nil {
		return "", "", fmt.Errorf("source image is unavailable")
	}
	return "/v1/files/image/" + filename, filename, nil
}

func (s *Store) saveDataURLAsset(raw, kind, name string) (string, string, error) {
	payload, mimeType, err := decodeDataURL(raw)
	if err != nil {
		return "", "", err
	}
	return s.saveAsset(payload, kind, firstNonEmpty(mimeType, mime.TypeByExtension(filepath.Ext(name)), defaultAssetMIME))
}

func (s *Store) saveAsset(payload []byte, kind, mimeType string) (string, string, error) {
	if len(payload) == 0 {
		return "", "", fmt.Errorf("image is empty")
	}
	if err := os.MkdirAll(s.imageDir, 0o755); err != nil {
		return "", "", err
	}

	sum := sha256.Sum256(payload)
	ext := extensionForMIME(mimeType)
	filename := fmt.Sprintf("%s-%x%s", sanitizeKind(kind), sum[:16], ext)
	path := filepath.Join(s.imageDir, filename)
	if _, err := os.Stat(path); err == nil {
		return "/v1/files/image/" + filename, filename, nil
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return "", "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	return "/v1/files/image/" + filename, filename, nil
}

func decodeDataURL(raw string) ([]byte, string, error) {
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return nil, "", fmt.Errorf("invalid data url")
	}
	meta := strings.ToLower(strings.TrimSpace(raw[:comma]))
	if !strings.Contains(meta, ";base64") {
		return nil, "", fmt.Errorf("only base64 data urls are supported")
	}
	mimeType := strings.TrimPrefix(strings.Split(meta, ";")[0], "data:")
	payload, err := base64.StdEncoding.DecodeString(raw[comma+1:])
	if err != nil {
		return nil, "", fmt.Errorf("decode data url: %w", err)
	}
	if len(payload) == 0 {
		return nil, "", fmt.Errorf("image is empty")
	}
	return payload, mimeType, nil
}

func extensionForMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func sanitizeKind(value string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if cleaned == "" {
		return "image"
	}
	var builder strings.Builder
	for _, r := range cleaned {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "image"
	}
	return builder.String()
}

func buildWorkTitle(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "未命名作品"
	}
	runes := []rune(prompt)
	if len(runes) <= 24 {
		return prompt
	}
	return strings.TrimSpace(string(runes[:24])) + "..."
}

func filenameFromImageURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	index := strings.LastIndex(trimmed, "/v1/files/image/")
	if index >= 0 {
		return filepath.Base(trimmed[index+len("/v1/files/image/"):])
	}
	return ""
}

func likePattern(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(strings.TrimSpace(value)) + "%"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
