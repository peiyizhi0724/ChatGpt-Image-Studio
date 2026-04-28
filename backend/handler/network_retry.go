package handler

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultNetworkRetryAttempts = 2
	defaultNetworkRetryBackoff  = 1200 * time.Millisecond
)

type NetworkRetryInfo struct {
	Operation  string
	Attempt    int
	MaxRetries int
	Err        error
}

type NetworkRetryHook func(context.Context, NetworkRetryInfo) error

func shouldRetryNetworkRequest(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	for _, marker := range []string{
		"i/o timeout",
		"timeout awaiting headers",
		"tls handshake timeout",
		"proxyconnect tcp",
		"proxy connect tcp",
		"connection reset by peer",
		"connection refused",
		"connect timeout",
		"unexpected eof",
		" eof",
		"eof",
		"no such host",
		"server misbehaving",
		"use of closed network connection",
		"ssl_error_syscall",
		"dial tcp",
		"lookup ",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (c *ChatGPTClient) doRequestWithRetry(
	ctx context.Context,
	client *http.Client,
	operation string,
	build func() (*http.Request, error),
) (*http.Response, error) {
	maxRetries := c.networkRetryAttempts
	if maxRetries < 0 {
		maxRetries = 0
	}

	for attempt := 0; ; attempt++ {
		req, err := build()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}
		if !shouldRetryNetworkRequest(err) || attempt >= maxRetries {
			return nil, err
		}

		log.Printf("[proxy-retry] %s failed on attempt %d/%d: %v", operation, attempt+1, maxRetries+1, err)
		if c.beforeNetworkRetry != nil {
			hookErr := c.beforeNetworkRetry(ctx, NetworkRetryInfo{
				Operation:  operation,
				Attempt:    attempt + 1,
				MaxRetries: maxRetries,
				Err:        err,
			})
			if hookErr != nil {
				log.Printf("[proxy-retry] %s switch hook failed: %v", operation, hookErr)
			}
		}

		backoff := c.networkRetryBackoff
		if backoff <= 0 {
			backoff = defaultNetworkRetryBackoff
		}
		timer := time.NewTimer(time.Duration(attempt+1) * backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
