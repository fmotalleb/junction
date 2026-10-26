package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	maxConfigBytes    = 4 << 20
	maxConfigIncludes = 128
	maxIncludeDepth   = 16
	httpScheme        = "http"
	httpsScheme       = "https"
)

var errConfigIncludeCycle = errors.New("configuration include cycle")

type configLoader struct {
	ctx     context.Context
	visited map[string]struct{}
	count   int
}

func loadConfig(ctx context.Context, path string) (map[string]any, error) {
	l := &configLoader{ctx: ctx, visited: make(map[string]struct{})}
	return l.read(path, "", 0)
}

func (l *configLoader) read(path, parent string, depth int) (map[string]any, error) {
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	if HardenedMode() && depth > maxIncludeDepth {
		return nil, fmt.Errorf("configuration includes exceed maximum depth %d", maxIncludeDepth)
	}
	if path == "" && parent == "" {
		var data []byte
		var err error
		if HardenedMode() {
			data, err = io.ReadAll(io.LimitReader(os.Stdin, maxConfigBytes+1))
		} else {
			data, err = io.ReadAll(os.Stdin)
		}
		if err != nil {
			return nil, fmt.Errorf("read configuration from stdin: %w", err)
		}
		return l.parse(data, ".toml", "")
	}

	key, err := canonicalConfigPath(path, parent)
	if err != nil {
		return nil, err
	}
	if _, seen := l.visited[key]; seen {
		return nil, fmt.Errorf("%w at %q", errConfigIncludeCycle, safeConfigPath(key))
	}
	if HardenedMode() {
		l.count++
		if l.count > maxConfigIncludes {
			return nil, fmt.Errorf("configuration includes exceed maximum count %d", maxConfigIncludes)
		}
	}
	l.visited[key] = struct{}{}
	defer delete(l.visited, key)

	data, ext, err := l.readBytes(key)
	if err != nil {
		return nil, err
	}
	current, err := l.parse(data, ext, key)
	if err != nil {
		return nil, err
	}
	includes, err := includePatterns(current["include"])
	if err != nil {
		return nil, fmt.Errorf("configuration %q: %w", safeConfigPath(key), err)
	}
	delete(current, "include")
	return l.readIncluded(current, key, depth, includes)
}

func (l *configLoader) readIncluded(current map[string]any, key string, depth int, includes []string) (map[string]any, error) {
	merged := make(map[string]any)
	for _, pattern := range includes {
		paths, err := l.expand(pattern, key)
		if err != nil {
			if HardenedMode() {
				return nil, fmt.Errorf("resolve include %q from %q: %w", RedactURL(pattern), safeConfigPath(key), err)
			}
			continue
		}
		if len(paths) == 0 {
			if HardenedMode() {
				return nil, fmt.Errorf("include %q from %q matched no files", RedactURL(pattern), safeConfigPath(key))
			}
			continue
		}
		for _, include := range paths {
			child, err := l.read(include, key, depth+1)
			if err != nil {
				if l.ctx.Err() != nil || HardenedMode() || errors.Is(err, errConfigIncludeCycle) {
					return nil, err
				}
				continue
			}
			mergeConfig(merged, child)
		}
	}
	mergeConfig(merged, current)
	return merged, nil
}

func (l *configLoader) parse(data []byte, ext, path string) (map[string]any, error) {
	if HardenedMode() && len(data) > maxConfigBytes {
		return nil, fmt.Errorf("configuration %q exceeds %d bytes", safeConfigPath(path), maxConfigBytes)
	}
	v := viper.New()
	v.SetConfigType(strings.TrimPrefix(ext, "."))
	if err := v.ReadConfig(strings.NewReader(string(data))); err != nil {
		return nil, fmt.Errorf("parse configuration %q: %w", safeConfigPath(path), err)
	}
	return v.AllSettings(), nil
}

func (l *configLoader) readBytes(path string) ([]byte, string, error) {
	u, err := url.Parse(path)
	if err == nil && isHTTPConfigURL(u) {
		if HardenedMode() && u.Scheme == httpScheme && os.Getenv("JUNCTION_ALLOW_INSECURE_CONFIG_HTTP") != "1" {
			return nil, "", errors.New("remote configuration requires HTTPS (set JUNCTION_ALLOW_INSECURE_CONFIG_HTTP=1 to allow HTTP temporarily)")
		}
		return l.readRemoteBytes(path, u)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- local config paths are canonicalized and intentionally user-selected.
	if err != nil {
		return nil, "", fmt.Errorf("read configuration %q: %w", safeConfigPath(path), err)
	}
	return data, filepath.Ext(path), nil
}

func isHTTPConfigURL(u *url.URL) bool {
	return u.Scheme == httpScheme || u.Scheme == httpsScheme
}

func (l *configLoader) readRemoteBytes(path string, u *url.URL) ([]byte, string, error) {
	requestCtx := l.ctx
	cancel := func() {}
	if HardenedMode() {
		requestCtx, cancel = context.WithTimeout(l.ctx, 15*time.Second)
	}
	defer cancel()
	req, requestErr := http.NewRequestWithContext(requestCtx, http.MethodGet, path, nil)
	if requestErr != nil {
		return nil, "", fmt.Errorf("create remote configuration request: %w", requestErr)
	}
	if u.User != nil {
		password, _ := u.User.Password()
		req.SetBasicAuth(u.User.Username(), password)
	}
	client := http.DefaultClient
	if HardenedMode() {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch remote configuration %q: request failed", RedactURL(path))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch remote configuration %q: HTTP %s", RedactURL(path), resp.Status)
	}
	var data []byte
	if HardenedMode() {
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxConfigBytes+1))
	} else {
		data, err = io.ReadAll(resp.Body)
	}
	if err != nil {
		return nil, "", fmt.Errorf("read remote configuration %q: %w", RedactURL(path), err)
	}
	return data, filepath.Ext(u.Path), nil
}

func (l *configLoader) expand(pattern, parent string) ([]string, error) {
	if u, err := url.Parse(pattern); err == nil && u.Scheme != "" {
		if !isHTTPConfigURL(u) {
			return nil, fmt.Errorf("unsupported include scheme %q", u.Scheme)
		}
		if base, err := url.Parse(parent); err == nil && isHTTPConfigURL(base) {
			resolved := base.ResolveReference(u)
			return []string{resolved.String()}, nil
		}
		return []string{u.String()}, nil
	}
	if !filepath.IsAbs(pattern) {
		if parentURL, err := url.Parse(parent); err == nil && isHTTPConfigURL(parentURL) {
			resolved := parentURL.ResolveReference(&url.URL{Path: pattern})
			return []string{resolved.String()}, nil
		}
		pattern = filepath.Join(filepath.Dir(parent), pattern)
	}
	return filepath.Glob(pattern)
}

func canonicalConfigPath(path, parent string) (string, error) {
	u, err := url.Parse(path)
	if err == nil && isHTTPConfigURL(u) {
		return u.String(), nil
	}
	if !filepath.IsAbs(path) {
		if parentURL, parseErr := url.Parse(parent); parseErr == nil && isHTTPConfigURL(parentURL) {
			return parentURL.ResolveReference(&url.URL{Path: path}).String(), nil
		}
		if parent == "" {
			parent = "."
		}
		path = filepath.Join(filepath.Dir(parent), path)
	}
	return filepath.Abs(path)
}

func includePatterns(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		if strings, stringOK := value.([]string); stringOK {
			items = make([]any, len(strings))
			for i := range strings {
				items[i] = strings[i]
			}
		} else {
			return nil, errors.New("include must be an array of strings")
		}
	}
	patterns := make([]string, 0, len(items))
	for _, item := range items {
		pattern, ok := item.(string)
		if !ok || strings.TrimSpace(pattern) == "" {
			return nil, errors.New("include entries must be non-empty strings")
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func mergeConfig(dst, src map[string]any) {
	for key, value := range src {
		if existing, ok := dst[key]; ok {
			existingMap, existingOK := existing.(map[string]any)
			valueMap, valueOK := value.(map[string]any)
			if existingOK && valueOK {
				mergeConfig(existingMap, valueMap)
				continue
			}
			if existingSlice, existingOK := existing.([]any); existingOK {
				if valueSlice, valueOK := value.([]any); valueOK {
					dst[key] = append(existingSlice, valueSlice...)
					continue
				}
			}
			dst[key] = value
			continue
		}
		dst[key] = value
	}
}

func safeConfigPath(path string) string {
	return RedactURL(path)
}

// RedactURL removes userinfo and values from query parameters that may carry
// credentials or tokens before a URL is written to logs or diagnostics.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid configuration URL]"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "REDACTED")
	}
	query := u.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "passwd") || strings.Contains(lower, "key") || strings.Contains(lower, "credential") || strings.Contains(lower, "auth") {
			query.Set(key, "REDACTED")
		}
	}
	u.RawQuery = query.Encode()
	return u.String()
}
