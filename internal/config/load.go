package config

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadMsg loads a system/role message.
//
// Supported inputs:
//   - raw strings
//   - http(s) URLs
//   - file:// paths
//
// For markdown files loaded via file://, YAML frontmatter is stripped.
func LoadMsg(msg string, httpProxy string) (string, error) {
	if strings.HasPrefix(msg, "https://") || strings.HasPrefix(msg, "http://") {
		return fetchRemoteMsg(msg, httpProxy)
	}
	if after, ok := strings.CutPrefix(msg, "file://"); ok {
		return loadFileMsg(after)
	}
	return msg, nil
}

const maxRemoteMsgBytes = 2 * 1024 * 1024

func fetchRemoteMsg(rawURL string, httpProxy string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("fetch role message: %w", err)
	}

	base, _ := http.DefaultTransport.(*http.Transport)
	tr := base.Clone()
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.IdleConnTimeout = 90 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second
	if httpProxy != "" {
		proxyURL, pErr := url.Parse(httpProxy)
		if pErr != nil {
			return "", fmt.Errorf("fetch role message: parse proxy: %w", pErr)
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	httpClient := &http.Client{Transport: tr}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch role message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bts, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return "", fmt.Errorf("fetch role message: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bts)))
	}
	bts, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteMsgBytes))
	if err != nil {
		return "", fmt.Errorf("read role message: %w", err)
	}
	if len(bts) >= maxRemoteMsgBytes {
		return "", fmt.Errorf("read role message: response too large (>%d bytes)", maxRemoteMsgBytes)
	}
	return string(bts), nil
}

func loadFileMsg(path string) (string, error) {
	bts, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read role file: %w", err)
	}
	content := string(bts)
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return content, nil
	}
	body, err := StripYAMLFrontmatter(content)
	if err != nil {
		return "", err
	}
	return body, nil
}

// StripYAMLFrontmatter removes YAML frontmatter from markdown content.
func StripYAMLFrontmatter(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content, nil
	}
	if strings.TrimSpace(lines[0]) != "---" {
		return content, nil
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", fmt.Errorf("invalid markdown frontmatter: missing closing delimiter")
	}

	frontmatter := strings.Join(lines[1:end], "\n")
	// Permissive unmarshal: frontmatter keys are discarded, only structure is validated.
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &parsed); err != nil {
		return "", fmt.Errorf("invalid markdown frontmatter: %w", err)
	}

	body := strings.Join(lines[end+1:], "\n")
	body = strings.TrimLeft(body, "\r\n")
	return body, nil
}
