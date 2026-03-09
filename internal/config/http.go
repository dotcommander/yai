package config

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewHTTPClient returns an HTTP client with the project's standard transport
// timeouts and optional proxy configuration.
func NewHTTPClient(httpProxy string) (*http.Client, error) {
	tr, err := NewHTTPTransport(httpProxy)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: tr}, nil
}

// NewHTTPTransport clones http.DefaultTransport and applies the transport
// defaults used for provider and remote role loading.
func NewHTTPTransport(httpProxy string) (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not *http.Transport")
	}

	tr := base.Clone()
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.IdleConnTimeout = 90 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second

	if httpProxy != "" {
		proxyURL, err := url.Parse(httpProxy)
		if err != nil {
			return nil, fmt.Errorf("parse proxy: %w", err)
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	}

	return tr, nil
}
