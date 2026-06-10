package mlog

import (
	"fmt"

	"go.uber.org/zap/zapcore"
)

// LogLevelFromString maps a config string to a zap level.
func LogLevelFromString(level string) (zapcore.Level, error) {
	levels := map[string]zapcore.Level{
		"debug": zapcore.DebugLevel,
		"info":  zapcore.InfoLevel,
		"error": zapcore.ErrorLevel,
	}
	if lvl, ok := levels[level]; ok {
		return lvl, nil
	}
	return zapcore.InfoLevel, fmt.Errorf("invalid log level: %s", level)
}
