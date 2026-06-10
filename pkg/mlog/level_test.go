package mlog_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

func TestLogLevelFromString(t *testing.T) {
	lvl, err := mlog.LogLevelFromString("debug")
	require.NoError(t, err)
	require.Equal(t, zapcore.DebugLevel, lvl)

	lvl, err = mlog.LogLevelFromString("info")
	require.NoError(t, err)
	require.Equal(t, zapcore.InfoLevel, lvl)

	_, err = mlog.LogLevelFromString("bogus")
	require.Error(t, err)
}
