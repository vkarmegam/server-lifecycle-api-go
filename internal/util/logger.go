package util

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// globalLogger is your shared logger instance
var globalLogger *zap.Logger

// InitLogger sets up a Zap logger that writes to a log file with rotation, in a human-readable format.
func InitLogger(level string, environment string, maxSize int) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Invalid log level '%s'. Defaulting to 'info'. Error: %v\n", level, err)
		zapLevel = zap.InfoLevel
	}
	var core zapcore.Core

	// Encoder config
	encoderConfig := zap.NewDevelopmentEncoderConfig() // Human-readable
	encoderConfig.CallerKey = "caller"
	encoderConfig.StacktraceKey = "" // Disable stack traces
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	if environment == "production" {

		// Log file writer (with rotation)
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   "./logs/app.log",
			MaxSize:    maxSize, // in MB
			MaxBackups: 3,       // number of rotated files
			MaxAge:     30,      // days
			Compress:   true,    // compress rotated files
		})

		// Console-style log format to file
		consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

		// Single core: logs to file only
		core = zapcore.NewCore(consoleEncoder, fileWriter, zapLevel)
	} else {
		core = zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), zapcore.AddSync(os.Stdout), zapLevel)
	}
	// Build logger
	logger := zap.New(core, zap.AddCaller())
	globalLogger = logger
	return nil
}

// GetLogger returns the global Zap logger.
// This should only be called after InitLogger has been successfully called.
func GetLogger() *zap.Logger {
	if globalLogger == nil {
		// Fallback for when logger is not initialized (shouldn't happen in main)
		// This creates a basic console logger and logs a warning.
		fmt.Fprintf(os.Stderr, "WARNING: GetLogger called before InitLogger. Initializing default development logger.\n")
		// Using a _ for the error here as we can't effectively handle it outside main.
		// In a real app, this scenario should be prevented at startup.
		_ = InitLogger("info", "development", 10)
	}
	return globalLogger
}

// LoggerFromContext retrieves a request-specific logger from the context.
// If no such logger is found, it returns the global logger.
// This is how handlers and services should get their logger for a request.
func LoggerFromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(ctx).(*zap.Logger); ok {
		return logger
	}
	return GetLogger() // Fallback to global logger if not in context
}

// StructuredLogger returns a chi middleware for structured logging with request ID.
func StructuredLogger(baseLogger *zap.Logger) func(next http.Handler) http.Handler {
	return middleware.RequestLogger(&structuredRequestLogger{logger: baseLogger})
}

type structuredRequestLogger struct {
	logger *zap.Logger
}

func (l *structuredRequestLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	// Get or generate a request ID
	reqID := middleware.GetReqID(r.Context())
	if reqID == "" {
		// fallback: manually generate one (shouldn't happen if middleware works)
		reqID = uuid.New().String()
	}
	// Create a new logger instance with request-specific fields
	requestLogger := l.logger.With(
		zap.String("request_id", reqID),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("remote_ip", r.RemoteAddr),
		zap.String("user_agent", r.UserAgent()),
	)

	// Inject the request-specific logger into the request context
	r = r.WithContext(context.WithValue(r.Context(), r, requestLogger))

	// Log the "request started" event
	requestLogger.Info("request started")

	return &structuredLoggerEntry{logger: requestLogger, reqID: reqID}
}

type structuredLoggerEntry struct {
	logger *zap.Logger // This logger now has request-specific fields
	reqID  string
}

func (l *structuredLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	l.logger.Info("request complete",
		zap.Int("status", status),
		zap.Int("bytes", bytes),
		zap.Duration("elapsed_ms", elapsed),
	)
}

func (l *structuredLoggerEntry) Panic(v interface{}, stack []byte) {
	l.logger.Error("panic caught",
		zap.Any("panic", v),
		zap.ByteString("stack_trace", stack), // Renamed field for clarity
	)
}
