package httputil

type Resp[T any] struct {
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResp struct {
	Message string `json:"message"`
}

func NewResp[T any](message string, data T) Resp[T] {
	return Resp[T]{Message: message, Data: data}
}

func NewMessage(message string) EmptyResp { return EmptyResp{Message: message} }
