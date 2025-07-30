package logger

import (
	"context"
	"os"

	graylog "github.com/gemnasium/logrus-graylog-hook/v3"
	log "github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

var (
	Log *log.Logger
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableQuote:     true,
		PadLevelText:     true,
		QuoteEmptyFields: true,
		ForceColors:      true,
	})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

	// Initialize Graylog hook
	graylogAddr := os.Getenv("GRAYLOG_ADDR")
	if graylogAddr == "" {
		graylogAddr = "gelf-udp-service:12222"
	}

	hook := graylog.NewGraylogHook(graylogAddr, map[string]interface{}{
		"service": "query-assistant",
	})

	Log = log.StandardLogger()
	Log.AddHook(hook)
}

// NewLogger creates a new logger instance for Uber FX
func NewLogger() *log.Logger {
	return Log
}

// To creates a logger entry with a specific type field
func To(name string) *log.Entry {
	return Log.WithField("type", name)
}

// WithQuery creates a logger entry with query information
func WithQuery(queryID string) *log.Entry {
	return Log.WithField("query_id", queryID)
}

// WithUser creates a logger entry with user information
func WithUser(userID string) *log.Entry {
	return Log.WithField("user_id", userID)
}

// QueryLogger provides structured logging for queries
type QueryLogger struct {
	logger *log.Logger
}

func NewQueryLogger() *QueryLogger {
	return &QueryLogger{
		logger: Log,
	}
}

func (ql *QueryLogger) Info(msg string, fields map[string]interface{}) {
	entry := ql.logger.WithFields(log.Fields(fields))
	entry.Info(msg)
}

func (ql *QueryLogger) Error(msg string, err error, fields map[string]interface{}) {
	entry := ql.logger.WithFields(log.Fields(fields)).WithError(err)
	entry.Error(msg)
}

func (ql *QueryLogger) Debug(msg string, fields map[string]interface{}) {
	entry := ql.logger.WithFields(log.Fields(fields))
	entry.Debug(msg)
}

func (ql *QueryLogger) Warn(msg string, fields map[string]interface{}) {
	entry := ql.logger.WithFields(log.Fields(fields))
	entry.Warn(msg)
}

// FxLogger wraps the logger for use with Uber FX
type FxLogger struct {
	*log.Logger
}

func (l FxLogger) Printf(format string, v ...interface{}) {
	l.Logger.Printf(format, v...)
}

// ProvideLogger provides the logger for dependency injection
func ProvideLogger(lc fx.Lifecycle) *log.Logger {
	logger := Log

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			logger.Info("Logger initialized")
			return nil
		},
		OnStop: func(context.Context) error {
			logger.Info("Logger shutting down")
			return nil
		},
	})

	return logger
}
