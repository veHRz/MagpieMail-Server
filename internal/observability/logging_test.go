package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/observability"
)

// decodeLines parses JSON log output, one object per line.
func decodeLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for line := range strings.Lines(out) {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), "log line is not JSON: %q", line)
		records = append(records, record)
	}
	return records
}

func TestNewLogger_WritesOneJSONObjectPerRecord(t *testing.T) {
	var out bytes.Buffer
	logger, err := observability.NewLogger(&out, "info", "json")
	require.NoError(t, err)

	logger.Info("server started", "listen", ":8080")

	records := decodeLines(t, out.String())
	require.Len(t, records, 1)
	assert.Equal(t, "INFO", records[0]["level"])
	assert.Equal(t, "server started", records[0]["msg"])
	assert.Equal(t, ":8080", records[0]["listen"])
}

func TestNewLogger_TimestampsAreUTC(t *testing.T) {
	var out bytes.Buffer
	logger, err := observability.NewLogger(&out, "info", "json")
	require.NoError(t, err)

	logger.Info("tick")

	records := decodeLines(t, out.String())
	require.Len(t, records, 1)
	ts, ok := records[0]["time"].(string)
	require.True(t, ok)
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	require.NoError(t, err)
	_, offset := parsed.Zone()
	assert.Zero(t, offset, "timestamp %q is not UTC", ts)
	assert.True(t, strings.HasSuffix(ts, "Z"), "timestamp %q is not UTC", ts)
}

func TestNewLogger_TextFormat(t *testing.T) {
	var out bytes.Buffer
	logger, err := observability.NewLogger(&out, "info", "text")
	require.NoError(t, err)

	logger.Info("server started")

	assert.Contains(t, out.String(), `msg="server started"`)
}

func TestNewLogger_FiltersBelowLevel(t *testing.T) {
	var out bytes.Buffer
	logger, err := observability.NewLogger(&out, "warn", "json")
	require.NoError(t, err)

	logger.Info("ignored")
	logger.Warn("kept")

	records := decodeLines(t, out.String())
	require.Len(t, records, 1)
	assert.Equal(t, "kept", records[0]["msg"])
}

func TestNewLogger_RejectsUnknownSettings(t *testing.T) {
	_, err := observability.NewLogger(&bytes.Buffer{}, "verbose", "json")
	require.Error(t, err)

	_, err = observability.NewLogger(&bytes.Buffer{}, "info", "xml")
	require.Error(t, err)
}

func TestLogger_AddsRequestIDFromContext(t *testing.T) {
	var out bytes.Buffer
	logger, err := observability.NewLogger(&out, "info", "json")
	require.NoError(t, err)

	ctx := observability.WithRequestID(context.Background(), "0192f3a4-0000-7000-8000-000000000001")
	logger.InfoContext(ctx, "inside a request")
	logger.InfoContext(context.Background(), "outside a request")
	logger.With("component", "http").InfoContext(ctx, "derived logger")

	records := decodeLines(t, out.String())
	require.Len(t, records, 3)
	assert.Equal(t, "0192f3a4-0000-7000-8000-000000000001", records[0]["request_id"])
	assert.NotContains(t, records[1], "request_id")
	assert.Equal(t, "0192f3a4-0000-7000-8000-000000000001", records[2]["request_id"])
	assert.Equal(t, "http", records[2]["component"])
}

func TestRequestID_EmptyWhenAbsent(t *testing.T) {
	assert.Empty(t, observability.RequestID(context.Background()))
	assert.Equal(t, "abc", observability.RequestID(observability.WithRequestID(context.Background(), "abc")))
}
