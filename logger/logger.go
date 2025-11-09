package logger

import (
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	// Log is the global logger instance
	Log zerolog.Logger
	// defaultLevel is the default log level
	defaultLevel = zerolog.InfoLevel
)

// LogLevel represents the available log levels
type LogLevel string

const (
	LevelTrace LogLevel = "trace"
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Init initializes the global logger with the specified level
func Init(level string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// Use console writer for human-readable output
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000-0700",
		NoColor:    false,
	}

	Log = zerolog.New(output).With().Timestamp().Logger()

	// Set the global zerolog logger so all uses of "github.com/rs/zerolog/log" also use our config
	log.Logger = Log

	// Set log level
	SetLevel(level)
}

// SetLevel sets the global log level
func SetLevel(level string) {
	parsedLevel := parseLogLevel(level)
	zerolog.SetGlobalLevel(parsedLevel)
	Log = Log.Level(parsedLevel)
	// Update the global logger as well
	log.Logger = Log
}

// parseLogLevel converts string to zerolog level
func parseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return defaultLevel
	}
}

// GetWriter returns an io.Writer that writes to the logger
func GetWriter() io.Writer {
	return Log
}

// Info logs an info message
func Info(msg string) {
	Log.Info().Msg(msg)
}

// Infof logs a formatted info message
func Infof(format string, args ...interface{}) {
	Log.Info().Msgf(format, args...)
}

// Debug logs a debug message
func Debug(msg string) {
	Log.Debug().Msg(msg)
}

// Debugf logs a formatted debug message
func Debugf(format string, args ...interface{}) {
	Log.Debug().Msgf(format, args...)
}

// Warn logs a warning message
func Warn(msg string) {
	Log.Warn().Msg(msg)
}

// Warnf logs a formatted warning message
func Warnf(format string, args ...interface{}) {
	Log.Warn().Msgf(format, args...)
}

// Error logs an error message
func Error(msg string) {
	Log.Error().Msg(msg)
}

// Errorf logs a formatted error message
func Errorf(format string, args ...interface{}) {
	Log.Error().Msgf(format, args...)
}

// ErrorErr logs an error with an error object
func ErrorErr(msg string, err error) {
	Log.Error().Err(err).Msg(msg)
}

// Fatal logs a fatal message and exits
func Fatal(msg string) {
	Log.Fatal().Msg(msg)
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(format string, args ...interface{}) {
	Log.Fatal().Msgf(format, args...)
}

// FatalErr logs a fatal message with an error and exits
func FatalErr(msg string, err error) {
	Log.Fatal().Err(err).Msg(msg)
}

// With returns a new logger with additional fields
func With() zerolog.Context {
	return Log.With()
}

// WithFields returns a new logger with the given fields
func WithFields(fields map[string]interface{}) zerolog.Logger {
	ctx := Log.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return ctx.Logger()
}
