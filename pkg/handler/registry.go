package handler

import "fmt"

// registry maps an enabled-prefix key to its specialized handler. Specialized
// handlers self-register from init(); the resolver queries it with the prefix
// key that config matched.
var registry = map[string]Handler{}

// Register binds a specialized handler to an exact prefix key. It is meant to be
// called from a handler package's init(). It panics on an empty prefix or a
// duplicate registration — both are programming errors caught at startup.
func Register(prefix string, h Handler) {
	if prefix == "" {
		panic("handler.Register: empty prefix")
	}
	if _, exists := registry[prefix]; exists {
		panic(fmt.Sprintf("handler.Register: duplicate prefix %q", prefix))
	}
	registry[prefix] = h
}

// lookup returns the specialized handler registered for an exact prefix key.
func lookup(prefix string) (Handler, bool) {
	h, ok := registry[prefix]
	return h, ok
}
