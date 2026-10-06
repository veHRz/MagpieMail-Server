// Package observability provides structured logging and request correlation.
//
// Never log message content, tokens or passwords: log identifiers and counts.
package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// NewLogger returns a logger that writes records to w as JSON ("json") or
// logfmt-like text ("text"), drops records below level, stamps times in UTC,
// and adds the request ID carried by the context, if any.
func NewLogger(w io.Writer, level, format string) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("unknown log level %q", level)
	}

	opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: utcTime}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("unknown log format %q", format)
	}
	return slog.New(contextHandler{handler}), nil
}

func utcTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
		a.Value = slog.TimeValue(a.Value.Time().UTC())
	}
	return a
}

type requestIDKey struct{}

// WithRequestID returns a context carrying the request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID carried by ctx, or "" if there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// contextHandler adds context-scoped attributes to every record.
type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
