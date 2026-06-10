package httputil

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

type ResponseError struct {
	Code    int
	Message string
}

func (e *ResponseError) Error() string { return e.Message }

func NewHTTPError(code int, message string) *ResponseError {
	return &ResponseError{Code: code, Message: message}
}

type EchoHTTPErrorHandler struct{ logger *mlog.Logger }

func NewEchoHTTPErrorHandler(logger *mlog.Logger) *EchoHTTPErrorHandler {
	return &EchoHTTPErrorHandler{logger: logger}
}

func (e *EchoHTTPErrorHandler) DefaultEchoHTTPErrorHandler(c *echo.Context, err error) {
	resp, _ := echo.UnwrapResponse(c.Response())
	if resp.Committed {
		return
	}

	logger := mlog.L(c.Request().Context(), e.logger)
	defer logger.Error("failed to handle request", zap.Error(err))

	code := http.StatusInternalServerError
	if responseErr, ok := errors.AsType[*ResponseError](err); ok {
		code = responseErr.Code
		_ = c.JSON(code, NewMessage(responseErr.Message))
		return
	}

	_ = c.JSON(code, NewMessage(http.StatusText(code)))
}
