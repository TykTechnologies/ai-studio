package logger

import (
	"fmt"
	"io"
	"log"

	"github.com/hashicorp/go-hclog"
	"github.com/rs/zerolog"
)

// HCLogAdapter adapts our zerolog logger to the hclog.Logger interface
type HCLogAdapter struct {
	logger zerolog.Logger
	name   string
}

// NewHCLogAdapter creates a new hclog adapter
func NewHCLogAdapter(name string) hclog.Logger {
	return &HCLogAdapter{
		logger: Log.With().Str("component", name).Logger(),
		name:   name,
	}
}

// Log emits the message at the given level
func (h *HCLogAdapter) Log(level hclog.Level, msg string, args ...interface{}) {
	h.log(level, msg, args...)
}

// Trace emits a trace level log
func (h *HCLogAdapter) Trace(msg string, args ...interface{}) {
	h.log(hclog.Trace, msg, args...)
}

// Debug emits a debug level log
func (h *HCLogAdapter) Debug(msg string, args ...interface{}) {
	h.log(hclog.Debug, msg, args...)
}

// Info emits an info level log
func (h *HCLogAdapter) Info(msg string, args ...interface{}) {
	h.log(hclog.Info, msg, args...)
}

// Warn emits a warn level log
func (h *HCLogAdapter) Warn(msg string, args ...interface{}) {
	h.log(hclog.Warn, msg, args...)
}

// Error emits an error level log
func (h *HCLogAdapter) Error(msg string, args ...interface{}) {
	h.log(hclog.Error, msg, args...)
}

// IsTrace returns true if trace level is enabled
func (h *HCLogAdapter) IsTrace() bool {
	return h.logger.GetLevel() <= zerolog.TraceLevel
}

// IsDebug returns true if debug level is enabled
func (h *HCLogAdapter) IsDebug() bool {
	return h.logger.GetLevel() <= zerolog.DebugLevel
}

// IsInfo returns true if info level is enabled
func (h *HCLogAdapter) IsInfo() bool {
	return h.logger.GetLevel() <= zerolog.InfoLevel
}

// IsWarn returns true if warn level is enabled
func (h *HCLogAdapter) IsWarn() bool {
	return h.logger.GetLevel() <= zerolog.WarnLevel
}

// IsError returns true if error level is enabled
func (h *HCLogAdapter) IsError() bool {
	return h.logger.GetLevel() <= zerolog.ErrorLevel
}

// ImpliedArgs returns any implied arguments
func (h *HCLogAdapter) ImpliedArgs() []interface{} {
	return nil
}

// With creates a sublogger with additional fields
func (h *HCLogAdapter) With(args ...interface{}) hclog.Logger {
	newLogger := h.logger
	ctx := newLogger.With()
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key := fmt.Sprintf("%v", args[i])
			ctx = ctx.Interface(key, args[i+1])
		}
	}
	return &HCLogAdapter{
		logger: ctx.Logger(),
		name:   h.name,
	}
}

// Name returns the logger name
func (h *HCLogAdapter) Name() string {
	return h.name
}

// Named creates a named sublogger
func (h *HCLogAdapter) Named(name string) hclog.Logger {
	return &HCLogAdapter{
		logger: h.logger.With().Str("subsystem", name).Logger(),
		name:   h.name + "." + name,
	}
}

// ResetNamed creates a logger with the given name
func (h *HCLogAdapter) ResetNamed(name string) hclog.Logger {
	return &HCLogAdapter{
		logger: Log.With().Str("component", name).Logger(),
		name:   name,
	}
}

// SetLevel sets the log level
func (h *HCLogAdapter) SetLevel(level hclog.Level) {
	// Convert hclog level to zerolog level
	var zlevel zerolog.Level
	switch level {
	case hclog.Trace:
		zlevel = zerolog.TraceLevel
	case hclog.Debug:
		zlevel = zerolog.DebugLevel
	case hclog.Info:
		zlevel = zerolog.InfoLevel
	case hclog.Warn:
		zlevel = zerolog.WarnLevel
	case hclog.Error:
		zlevel = zerolog.ErrorLevel
	default:
		zlevel = zerolog.InfoLevel
	}
	h.logger = h.logger.Level(zlevel)
}

// GetLevel returns the current log level
func (h *HCLogAdapter) GetLevel() hclog.Level {
	level := h.logger.GetLevel()
	switch level {
	case zerolog.TraceLevel:
		return hclog.Trace
	case zerolog.DebugLevel:
		return hclog.Debug
	case zerolog.InfoLevel:
		return hclog.Info
	case zerolog.WarnLevel:
		return hclog.Warn
	case zerolog.ErrorLevel:
		return hclog.Error
	default:
		return hclog.Info
	}
}

// StandardLogger returns a standard library logger
func (h *HCLogAdapter) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	return log.New(h.StandardWriter(opts), "", 0)
}

// StandardWriter returns an io.Writer that writes to the logger
func (h *HCLogAdapter) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return h.logger
}

// log is the internal logging function
func (h *HCLogAdapter) log(level hclog.Level, msg string, args ...interface{}) {
	// Parse args as key-value pairs
	evt := h.logger.WithLevel(hclogToZerolog(level))

	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key := fmt.Sprintf("%v", args[i])
			evt = evt.Interface(key, args[i+1])
		}
	}

	evt.Msg(msg)
}

// hclogToZerolog converts hclog level to zerolog level
func hclogToZerolog(level hclog.Level) zerolog.Level {
	switch level {
	case hclog.Trace:
		return zerolog.TraceLevel
	case hclog.Debug:
		return zerolog.DebugLevel
	case hclog.Info:
		return zerolog.InfoLevel
	case hclog.Warn:
		return zerolog.WarnLevel
	case hclog.Error:
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
