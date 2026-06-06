package logstore

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
)

// Handler is a custom slog.Handler that writes records to both the underlying
// text handler (stderr) and the Store's ring buffer. When an OTel exporter is
// configured, it also forwards records to OpenTelemetry.
type Handler struct {
	inner  slog.Handler
	store  *Store
	otelFn func(context.Context, slog.Record)
	attrs  []slog.Attr
	group  string
}

// NewHandler creates a new Handler that wraps inner (the stderr handler) and
// stores records in the given store. If otelFn is non-nil, it is called
// asynchronously for each log record.
func NewHandler(inner slog.Handler, store *Store, otelFn func(context.Context, slog.Record)) *Handler {
	return &Handler{
		inner:  inner,
		store:  store,
		otelFn: otelFn,
	}
}

// Enabled reports whether the handler is enabled for the given level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.store.Level()
}

// Handle processes a log record.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Send to the underlying handler (stderr).
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}

	// Store in the ring buffer.
	entry := h.recordToEntry(r)
	h.store.Append(entry)

	// Forward to OTel exporter if configured.
	if h.otelFn != nil {
		go h.otelFn(ctx, r.Clone())
	}

	return nil
}

// WithAttrs returns a new handler with the given attributes attached.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		inner:  h.inner.WithAttrs(attrs),
		store:  h.store,
		otelFn: h.otelFn,
		attrs:  append(h.attrs, attrs...),
		group:  h.group,
	}
}

// WithGroup returns a new handler with the given group name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{
		inner:  h.inner.WithGroup(name),
		store:  h.store,
		otelFn: h.otelFn,
		attrs:  h.attrs,
		group:  h.group + name + ".",
	}
}

// recordToEntry converts an slog.Record to a LogEntry.
func (h *Handler) recordToEntry(r slog.Record) LogEntry {
	level := LevelName(r.Level)

	// Collect attributes.
	var attrs []slog.Attr
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Determine source from PC if available.
	source := h.group
	if source == "" {
		source = guessSource(r.PC)
	} else {
		source = strings.TrimSuffix(source, ".")
	}

	return LogEntry{
		Timestamp:  r.Time,
		Level:      level,
		Source:     source,
		Message:    r.Message,
		Attributes: AttrsToMap(attrs),
	}
}

// guessSource attempts to determine the source package from the PC.
func guessSource(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return ""
	}
	name := fn.Name()
	// Extract just the package name, not the full function path.
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.Index(name, "."); idx >= 0 {
		name = name[:idx]
	}
	return name
}

