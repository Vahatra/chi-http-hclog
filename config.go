package httplog

import (
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
)

var DefaultOptions = Options{
	Name:        "",
	Level:       "info",
	JSONFormat:  false,
	TimeFormat:  time.RFC3339Nano,
	Concise:     false,
	Tags:        nil,
	SkipHeaders: nil,
}

type Options struct {
	// hclog: Name of the subsystem to prefix logs with
	Name string

	// hclog: The threshold for the logger. Anything less severe is suppressed
	// Must be one of: ["trace", "debug", "info", "warn", "error", "none", "off" ]
	Level string

	// hclog: Control if the output should be in JSON.
	JSONFormat bool

	// hclog: The time format to use instead of the default
	TimeFormat string

	// Concise mode includes fewer log details during the request flow. For example
	// excluding details like request content length, user-agent and other details.
	// This is useful if during development your console is too noisy.
	Concise bool

	// Tags are additional fields included at the root level of all logs.
	// These can be useful for example the commit hash of a build, or an environment
	// name like prod/stg/dev
	Tags map[string]string

	// SkipHeaders are additional headers which are redacted from the logs
	SkipHeaders []string
}

// Configure will set new global/default options for the httplog and behaviour
// of underlying zerolog pkg and its global logger.
func Configure(opts Options) {
	if opts.Level == "" {
		opts.Level = "info"
	}

	if opts.TimeFormat == "" {
		opts.TimeFormat = time.RFC3339Nano
	}

	// Pre-downcase all SkipHeaders
	for i, header := range opts.SkipHeaders {
		opts.SkipHeaders[i] = strings.ToLower(header)
	}

	DefaultOptions = opts

	hclog.DefaultOptions = &hclog.LoggerOptions{
		Name:       opts.Name,
		Level:      hclog.LevelFromString(opts.Level),
		TimeFormat: opts.TimeFormat,
		JSONFormat: opts.JSONFormat,
	}
}
