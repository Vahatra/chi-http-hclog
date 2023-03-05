package httplog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hashicorp/go-hclog"
)

func NewLogger(opts ...Options) hclog.Logger {
	if len(opts) > 0 {
		Configure(opts[0])
	} else {
		Configure(DefaultOptions)
	}
	if !DefaultOptions.Concise && len(DefaultOptions.Tags) > 0 {
		return hclog.L().With("tags", DefaultOptions.Tags)
	} else {
		return hclog.L()
	}
}

// RequestLogger is an http middleware to log http requests and responses.
//
// NOTE: for simplicity, RequestLogger automatically makes use of the chi RequestID and
// Recoverer middleware.
func RequestLogger(logger hclog.Logger) func(next http.Handler) http.Handler {
	return chi.Chain(middleware.RequestID, Handler(logger), middleware.Recoverer).Handler
}

func Handler(logger hclog.Logger) func(next http.Handler) http.Handler {
	var f middleware.LogFormatter = &requestLogger{logger}
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			entry := f.NewLogEntry(r)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			buf := newLimitBuffer(512)
			ww.Tee(buf)

			t1 := time.Now()
			defer func() {
				var respBody []byte
				if ww.Status() >= 400 {
					respBody, _ = io.ReadAll(buf)
				}
				entry.Write(ww.Status(), ww.BytesWritten(), ww.Header(), time.Since(t1), respBody)
			}()

			next.ServeHTTP(ww, middleware.WithLogEntry(r, entry))
		}
		return http.HandlerFunc(fn)
	}
}

type requestLogger struct {
	Logger hclog.Logger
}

func (l *requestLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &RequestLoggerEntry{}
	// msg := fmt.Sprintf("Request: %s %s", r.Method, r.URL.Path)
	// entry.Logger = l.Logger.Info(msg, requestLogFields(r, true))

	entry.Logger = l.Logger.With(requestLogFields(r, true)...)
	// if !DefaultOptions.Concise {
	// 	entry.Logger.Info().Fields(requestLogFields(r, DefaultOptions.Concise)).Msgf(msg)
	// }

	return entry
}

type RequestLoggerEntry struct {
	Logger hclog.Logger
	msg    string
}

func (l *RequestLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	msg := fmt.Sprintf("%d %s", status, statusLabel(status))
	if l.msg != "" {
		msg = fmt.Sprintf("%s - %s", msg, l.msg)
	}

	responseLog := []interface{}{
		"status", status,
		"bytes", bytes,
		"elapsed", float64(elapsed.Nanoseconds()) / 1000000.0, // in milliseconds
	}

	if !DefaultOptions.Concise {
		// Include response header, as well for error status codes (>400) we include
		// the response body so we may inspect the log message sent back to the client.
		if status >= 400 {
			body, _ := extra.([]byte)
			responseLog = append(responseLog, "responseBody", string(body))
		}
		if len(header) > 0 {
			responseLog = append(responseLog, headerLogField(header)...)
		}
	}

	l.Logger.Log(hclog.Level(statusLevel(status)), msg, responseLog...)

	// l.Logger.WithLevel(statusLevel(status)).Fields(map[string]interface{}{
	// 	"httpResponse": responseLog,
	// }).Msgf(msg)
}

func (l *RequestLoggerEntry) Panic(v interface{}, stack []byte) {
	stacktrace := "#"
	if DefaultOptions.JSONFormat {
		stacktrace = string(stack)
	}

	l.Logger = l.Logger.With("stacktrace", stacktrace, "panic", fmt.Sprintf("%+v", v))

	l.msg = fmt.Sprintf("%+v", v)

	if !DefaultOptions.JSONFormat {
		middleware.PrintPrettyStack(v)
	}
}

func requestLogFields(r *http.Request, concise bool) []interface{} {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	requestURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)

	requestFields := []interface{}{
		"requestURL", requestURL,
		"requestMethod", r.Method,
		"requestPath", r.URL.Path,
		"remoteIP", r.RemoteAddr,
		"proto", r.Proto,
	}
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		requestFields = append(requestFields, "requestID", reqID)
	}

	if concise {
		return requestFields
	}

	requestFields = append(requestFields, "scheme", scheme)

	if len(r.Header) > 0 {
		requestFields = append(requestFields, headerLogField(r.Header)...)
	}

	return requestFields
}

func headerLogField(header http.Header) []interface{} {
	var headerField []interface{}
	for k, v := range header {
		k = strings.ToLower(k)
		switch {
		case len(v) == 0:
			continue
		case len(v) == 1:
			headerField = append(headerField, k, v[0])
		default:
			headerField = append(headerField, k, fmt.Sprintf("[%s]", strings.Join(v, "], [")))
		}

		if k == "authorization" || k == "cookie" || k == "set-cookie" {
			headerField = append(headerField, k, "***")
		}

		for _, skip := range DefaultOptions.SkipHeaders {
			if k == skip {
				headerField = append(headerField, k, "***")
				break
			}
		}
	}

	return headerField
}

func statusLevel(status int) hclog.Level {
	switch {
	case status <= 0:
		return hclog.Warn
	case status < 400: // for codes in 100s, 200s, 300s
		return hclog.Info
	case status >= 400 && status < 500:
		return hclog.Warn
	case status >= 500:
		return hclog.Error
	default:
		return hclog.Info
	}
}

func statusLabel(status int) string {
	switch {
	case status >= 100 && status < 300:
		return "OK"
	case status >= 300 && status < 400:
		return "Redirect"
	case status >= 400 && status < 500:
		return "Client Error"
	case status >= 500:
		return "Server Error"
	default:
		return "Unknown"
	}
}

// Helper methods used by the application to get the request-scoped
// logger entry and set additional fields between handlers.
//
// This is a useful pattern to use to set state on the entry as it
// passes through the handler chain, which at any point can be logged
// with a call to .Print(), .Info(), etc.

func LogEntry(ctx context.Context) hclog.Logger {
	entry, ok := ctx.Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry)
	if !ok || entry == nil {
		return hclog.NewNullLogger()
	} else {
		return entry.Logger
	}
}

func LogEntrySetField(ctx context.Context, key, value string) {
	if entry, ok := ctx.Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry); ok {
		entry.Logger = entry.Logger.With(key, value)
	}
}

func LogEntrySetFields(ctx context.Context, fields []interface{}) {
	if entry, ok := ctx.Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry); ok {
		entry.Logger = entry.Logger.With(fields...)
	}
}
