package action

import (
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/config"
)

// e2ePane is a pane driven through the real default key bindings.
func e2ePane(t *testing.T, text string) *BufPane {
	h := specPane(t, text)
	config.ConfigDir = t.TempDir()
	InitBindings()
	InitTabs([]*buffer.Buffer{h.Buf})
	p := MainTab().CurPane()
	p.Resize(80, 24)
	p.Buf.Settings["virtualspace"] = true
	return p
}

func press(h *BufPane, k tcell.Key, mod tcell.ModMask, n int) {
	for i := 0; i < n; i++ {
		h.HandleEvent(tcell.NewEventKey(k, 0, mod, ""))
	}
}

const altShift = tcell.ModAlt | tcell.ModShift

func TestBlockDefaultKeys(t *testing.T) {
	h := e2ePane(t, "abcdef\nghijkl\nmnopqr\nxy")
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})

	// Alt+Shift+arrows select the block
	press(h, tcell.KeyRight, altShift, 2)
	press(h, tcell.KeyDown, altShift, 2)
	assert.Equal(t, []string{"bc", "hi", "no"}, selections(h))

	// Ctrl+C copies, then moving drops the block
	press(h, tcell.KeyCtrlC, tcell.ModCtrl, 1)
	assert.Equal(t, "bc\nhi\nno", blockClipText)
	press(h, tcell.KeyDown, tcell.ModNone, 1)
	assert.Equal(t, 1, h.Buf.NumCursors())
	// the block ended at column 3, so Down goes to column 3 of "xy" (1 beyond its end)
	assert.Equal(t, buffer.Loc{X: 2, Y: 3}, h.Buf.GetActiveCursor().Loc)
	assert.Equal(t, 1, h.Buf.GetActiveCursor().VirtualX())

	// Ctrl+V pastes the column under the cursor (column 5, beyond the end of "xy")
	press(h, tcell.KeyRight, tcell.ModNone, 2)
	press(h, tcell.KeyCtrlV, tcell.ModCtrl, 1)
	assert.Equal(t, "abcdef\nghijkl\nmnopqr\nxy   bc\n     hi\n     no", bufText(h))
	assert.Equal(t, 1, h.Buf.NumCursors())
	assert.Equal(t, buffer.Loc{X: 5, Y: 3}, h.Buf.GetActiveCursor().Loc)

	// Ctrl+Z undoes the paste
	press(h, tcell.KeyCtrlZ, tcell.ModCtrl, 1)
	assert.Equal(t, "abcdef\nghijkl\nmnopqr\nxy", bufText(h))
}

func TestBlockEscapeKey(t *testing.T) {
	h := e2ePane(t, "abcdef\nghijkl")
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	press(h, tcell.KeyRight, altShift, 2)
	press(h, tcell.KeyDown, altShift, 1)
	assert.Equal(t, 2, h.Buf.NumCursors())
	press(h, tcell.KeyEscape, tcell.ModNone, 1)
	assert.Equal(t, 1, h.Buf.NumCursors())
	assert.False(t, h.Buf.GetActiveCursor().HasSelection())
}

func TestBlockTypingKeys(t *testing.T) {
	h := e2ePane(t, "abcdef\nghijkl\nmnopqr")
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	press(h, tcell.KeyRight, altShift, 2)
	press(h, tcell.KeyDown, altShift, 2)
	h.HandleEvent(tcell.NewEventKey(tcell.KeyRune, 'Z', tcell.ModNone, ""))
	h.HandleEvent(tcell.NewEventKey(tcell.KeyRune, 'Q', tcell.ModNone, ""))
	assert.Equal(t, "aZQdef\ngZQjkl\nmZQpqr", bufText(h))
	press(h, tcell.KeyBackspace2, tcell.ModNone, 1)
	assert.Equal(t, "aZdef\ngZjkl\nmZpqr", bufText(h))
}

func TestBlockPageKeys(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "abcdef"
	}
	text := strings.Join(lines, "\n")
	h := e2ePane(t, text)
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 50})
	press(h, tcell.KeyRight, altShift, 2)

	page := h.BufView().Height
	assert.Greater(t, page, 1)
	ctrlShift := tcell.ModCtrl | tcell.ModShift
	press(h, tcell.KeyPgDn, ctrlShift, 1)
	assert.Equal(t, page+1, h.Buf.NumCursors())
	press(h, tcell.KeyPgUp, ctrlShift, 2) // back over the anchor and a page above it
	assert.Equal(t, page+1, h.Buf.NumCursors())
	press(h, tcell.KeyPgUp, ctrlShift, 20) // clamped at the first line
	assert.Equal(t, 51, h.Buf.NumCursors())
	press(h, tcell.KeyPgDn, ctrlShift, 20) // clamped at the last line
	assert.Equal(t, 50, h.Buf.NumCursors())
	assert.Equal(t, text, bufText(h))
}
