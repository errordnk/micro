package action

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/screen"
)

// With no reserved info bar the window owns the whole screen height.
func TestWindowUsesWholeScreen(t *testing.T) {
	h := e2ePane(t, "abc")
	_, sh := screen.Screen.Size()
	assert.Equal(t, sh, h.GetView().Height)
	Tabs.Resize()
	assert.Equal(t, sh, MainTab().CurPane().GetView().Height)
	_ = buffer.Loc{}
}
