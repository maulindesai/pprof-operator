package logging

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitFromEnv initializes a structured logger using Zap based on environment variables.
// Supported env vars:
// - LOG_LEVEL: one of debug, info, warn/warning, error, panic
// - DEBUG: if set to "true" enables development config and debug level (backward compatibility)
// Returns the logr.Logger and a cleanup function to flush buffers.
func InitFromEnv() (logr.Logger, func()) {
	// Default to production config with ISO8601 time
	zapConfig := zap.NewProductionConfig()
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	// Backward compatibility: DEBUG=true enables dev config at debug level
	if os.Getenv("DEBUG") == "true" {
		devCfg := zap.NewDevelopmentConfig()
		devCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		devCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		zapConfig = devCfg
	} else {
		// Map LOG_LEVEL if provided
		switch os.Getenv("LOG_LEVEL") {
		case "debug":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		case "info", "":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		case "warn", "warning":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
		case "error":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
		case "panic":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.PanicLevel)
		default:
			// Keep default and print to stderr so it is visible even before logger init
			fmt.Fprintf(os.Stderr, "Unknown LOG_LEVEL: %s, using default (info)\n", os.Getenv("LOG_LEVEL"))
		}
	}

	zapLog, err := zapConfig.Build()
	if err != nil {
		// Fall back to a no-op logger on failure to avoid crashing
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		return logr.Discard(), func() {}
	}

	cleanup := func() {
		_ = zapLog.Sync()
	}

	return zapr.NewLogger(zapLog), cleanup
}
