// Package log provides a minimal logger interface used by the framework.
//
// Consumers wire in their own logger (e.g. a structured logger with secret
// redaction). The default is a no-op so the module is silent when no logger
// is supplied, which is the common case in libraries.
package log

import (
	"fmt"
	"sync/atomic"
)

// Logger is the minimal logging surface the framework needs.
// All methods accept printf-style format strings.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// NoopLogger discards all log output.
type NoopLogger struct{}

func (NoopLogger) Debugf(string, ...any) {}
func (NoopLogger) Infof(string, ...any)  {}
func (NoopLogger) Warnf(string, ...any)  {}
func (NoopLogger) Errorf(string, ...any) {}

// StderrLogger writes log lines to stderr via fmt.Fprintf. Useful for
// development; consumers in production should plug in their own logger.
type StderrLogger struct{}

func (StderrLogger) Debugf(format string, args ...any) { fmt.Printf("[DEBUG] "+format+"\n", args...) }
func (StderrLogger) Infof(format string, args ...any)  { fmt.Printf("[INFO]  "+format+"\n", args...) }
func (StderrLogger) Warnf(format string, args ...any)  { fmt.Printf("[WARN]  "+format+"\n", args...) }
func (StderrLogger) Errorf(format string, args ...any) { fmt.Printf("[ERROR] "+format+"\n", args...) }

// active is the framework-wide logger, swappable via Set.
var active atomic.Value // Logger

func init() {
	active.Store(Logger(NoopLogger{}))
}

// Set installs the logger used by all framework packages.
// Safe to call from init() in the consumer.
func Set(l Logger) {
	if l == nil {
		l = NoopLogger{}
	}
	active.Store(l)
}

// L returns the currently-installed logger.
func L() Logger {
	v := active.Load()
	if v == nil {
		return NoopLogger{}
	}
	return v.(Logger)
}
