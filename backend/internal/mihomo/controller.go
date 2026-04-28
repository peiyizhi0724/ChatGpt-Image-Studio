package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Controller struct {
	baseURL  string
	secret   string
	group    string
	probeURL string
	client   *http.Client
}

type RetestResult struct {
	Group   string
	Before  string
	After   string
	Changed bool
}

func New(baseURL, secret, group, probeURL string, timeout time.Duration) *Controller {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	group = strings.TrimSpace(group)
	if group == "" {
		group = "Proxy"
	}
	probeURL = strings.TrimSpace(probeURL)
	if probeURL == "" {
		probeURL = "https://chatgpt.com/cdn-cgi/trace"
	}
	return &Controller{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		secret:   strings.TrimSpace(secret),
		group:    group,
		probeURL: probeURL,
		client:   &http.Client{Timeout: timeout},
	}
}

func (c *Controller) TriggerGroupRetest(ctx context.Context) (*RetestResult, error) {
	if c == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("mihomo controller is not configured")
	}

	before, _ := c.currentSelection(ctx)
	if err := c.delayGroup(ctx); err != nil {
		return nil, err
	}
	after, _ := c.currentSelection(ctx)

	return &RetestResult{
		Group:   c.group,
		Before:  before,
		After:   after,
		Changed: before != "" && after != "" && before != after,
	}, nil
}

func (c *Controller) currentSelection(ctx context.Context) (string, error) {
	endpoint := c.baseURL + "/proxies/" + url.PathEscape(c.group)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	c.applyHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("fetch proxy group returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Now), nil
}

func (c *Controller) delayGroup(ctx context.Context) error {
	query := url.Values{}
	query.Set("url", c.probeURL)
	query.Set("timeout", "20000")

	endpoint := c.baseURL + "/proxies/" + url.PathEscape(c.group) + "/delay?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	c.applyHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("proxy delay returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return nil
}

func (c *Controller) applyHeaders(req *http.Request) {
	if c == nil || req == nil {
		return
	}
	if strings.TrimSpace(c.secret) != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
}
