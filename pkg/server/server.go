package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/httputil"
	"github.com/eli-yip/rss-ai/pkg/mlog"
)

// Pinger reports whether a downstream dependency (the DB) is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	echo       *echo.Echo
	addr       string
	pinger     Pinger
	httpServer *http.Server
	logger     *mlog.Logger
}

func New(addr string, pinger Pinger, logger *mlog.Logger) *Server {
	svcLogger := mlog.S(logger, "server")

	e := echo.New()
	e.HTTPErrorHandler = httputil.NewEchoHTTPErrorHandler(svcLogger).DefaultEchoHTTPErrorHandler
	e.Use(
		traceIDMiddleware(),
		requestLoggerMiddleware(svcLogger),
		middleware.Recover(),
	)

	s := &Server{echo: e, addr: addr, pinger: pinger, logger: svcLogger}
	e.GET("/healthz", s.handleHealthz)
	e.GET("/readyz", s.handleReadyz)
	return s
}

// Echo exposes the router for in-process testing.
func (s *Server) Echo() *echo.Echo { return s.echo }

func (s *Server) handleHealthz(c *echo.Context) error {
	return c.JSON(http.StatusOK, httputil.NewMessage("ok"))
}

func (s *Server) handleReadyz(c *echo.Context) error {
	if err := s.pinger.Ping(c.Request().Context()); err != nil {
		return httputil.NewHTTPError(http.StatusServiceUnavailable, "database not ready")
	}
	return c.JSON(http.StatusOK, httputil.NewMessage("ready"))
}

// Start binds the listener and serves in a background goroutine.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.httpServer = &http.Server{Handler: s.echo}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("http serve error", zap.Error(err))
		}
	}()

	s.logger.Info("http server started", zap.String("addr", s.addr))
	return nil
}

// Stop gracefully shuts the server down.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
