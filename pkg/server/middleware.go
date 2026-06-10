package server

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

// traceIDMiddleware assigns a random trace id to each request's context so all
// downstream logs (including detached background work) can be correlated.
func traceIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), mlog.TraceIDCtxKey, rand.Uint64())
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// requestLoggerMiddleware emits one structured access line per request.
func requestLoggerMiddleware(logger *mlog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)

			l := mlog.L(c.Request().Context(), logger)
			resp, _ := echo.UnwrapResponse(c.Response())
			fields := []zap.Field{
				zap.String("method", c.Request().Method),
				zap.String("uri", c.Request().RequestURI),
				zap.Int("status", resp.Status),
				zap.Duration("latency", time.Since(start)),
			}
			if err != nil {
				fields = append(fields, zap.Error(err))
			}
			if resp.Status >= 500 {
				l.Error("request", fields...)
			} else {
				l.Info("request", fields...)
			}
			return err
		}
	}
}
