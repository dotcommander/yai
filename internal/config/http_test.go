package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHTTPTransportRejectsBadProxy(t *testing.T) {
	_, err := NewHTTPTransport("://bad-proxy")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse proxy")
}
