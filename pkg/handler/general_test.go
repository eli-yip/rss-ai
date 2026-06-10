package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneralHandlerPrompt(t *testing.T) {
	h := generalHandler{}
	require.NotEmpty(t, h.Prompt())
	require.Equal(t, defaultPrompt, h.Prompt())
}
