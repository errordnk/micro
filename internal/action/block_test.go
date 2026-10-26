package action

import (
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/clipboard"
	"github.com/zyedidia/micro/v2/internal/config"
	"github.com/zyedidia/micro/v2/internal/screen"
	ulua "github.com/zyedidia/micro/v2/internal/lua"
)

func newBlockPane(t *testing.T, text string) *BufPane {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	config.GlobalSettings["backup"] = false
	config.GlobalSettings["fastdirty"] = true
	if InfoBar == nil {
		if _, err := screen.InitSimScreen(); err != nil {
			t.Fatal(err)
		}
		InfoBar = NewInfoBar()
	}
	clipboard.SetMethod("internal")
	b := buffer.NewBufferFromString(text, "", buffer.BTDefault)
	h := NewBufPaneFromBuf(b, nil)
	h.Resize(80, 24)
	return h
}

// run mimics the BufMapEvent wrapper for a single action
func run(h *BufPane, name string, a BufKeyAction) {
	if isMoveAction(name) {
		h.collapseBlock()
	}
	if !blockKeepingActions[name] {
		h.block = nil
		h.blockCursors = false
	}
	if MultiActions[name] {
		for _, c := range h.Buf.GetCursors() {
			h.Buf.SetCurCursor(c.Num)
			h.Cursor = c
			a(h)
		}
	} else {
		h.Buf.SetCurCursor(0)
		h.Cursor = h.Buf.GetActiveCursor()
		a(h)
	}
	h.Buf.MergeCursors()
	h.finishBlock()
}

func TestBlockCopyCutPaste(t *testing.T) {
	h := newBlockPane(t, "abcdef\nghijkl\nmnopqr\nstuvwx")
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	assert.Equal(t, 3, h.Buf.NumCursors())

	run(h, "Copy", (*BufPane).Copy)
	assert.Equal(t, "bc\nhi\nno", blockClipText)

	// paste at column 4 of the last line, extends past buffer end
	h.Buf.ClearCursors()
	h.Cursor = h.Buf.GetActiveCursor()
	h.Cursor.GotoLoc(buffer.Loc{X: 4, Y: 3})
	run(h, "Paste", (*BufPane).Paste)
	assert.Equal(t, "abcdef\nghijkl\nmnopqr\nstuvbcwx\n    hi\n    no", string(h.Buf.Bytes()))
	assert.Equal(t, 1, h.Buf.NumCursors())

	// cut the same block back out
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "Cut", (*BufPane).Cut)
	assert.Equal(t, "adef\ngjkl\nmpqr\nstuvbcwx\n    hi\n    no", string(h.Buf.Bytes()))
	assert.Equal(t, 1, h.Buf.NumCursors())
	assert.Equal(t, buffer.Loc{X: 1, Y: 0}, h.Cursor.Loc)

	// paste block over a block selection
	run(h, "Paste", (*BufPane).Paste)
	assert.Equal(t, "abcdef\nghijkl\nmnopqr\nstuvbcwx\n    hi\n    no", string(h.Buf.Bytes()))
}

func TestVirtualSpace(t *testing.T) {
	h := newBlockPane(t, "abcdefgh\nab\n\nxyz")
	h.Buf.Settings["virtualspace"] = true

	// Down from a long line keeps the column in virtual space
	h.Cursor.GotoLoc(buffer.Loc{X: 6, Y: 0})
	run(h, "CursorDown", (*BufPane).CursorDown)
	assert.Equal(t, buffer.Loc{X: 2, Y: 1}, h.Cursor.Loc)
	assert.Equal(t, 4, h.Cursor.VirtualX())
	run(h, "CursorDown", (*BufPane).CursorDown)
	assert.Equal(t, 6, h.Cursor.VirtualX())
	run(h, "CursorDown", (*BufPane).CursorDown)
	assert.Equal(t, 3, h.Cursor.X)
	assert.Equal(t, 3, h.Cursor.VirtualX())

	// Right at end of line goes beyond it, Left comes back
	h.Cursor.GotoLoc(buffer.Loc{X: 2, Y: 1})
	run(h, "CursorRight", (*BufPane).CursorRight)
	run(h, "CursorRight", (*BufPane).CursorRight)
	assert.Equal(t, 2, h.Cursor.VirtualX())
	assert.Equal(t, 1, h.Cursor.Y)
	run(h, "Backspace", (*BufPane).Backspace)
	assert.Equal(t, 1, h.Cursor.VirtualX())
	assert.Equal(t, "ab", string(h.Buf.LineBytes(1)))

	// typing pads with spaces
	h.DoRuneInsert('Z')
	assert.Equal(t, "ab Z", string(h.Buf.LineBytes(1)))
	assert.Equal(t, 0, h.Cursor.VirtualX())

	// disabled: classic behavior
	h.Buf.Settings["virtualspace"] = false
	h.Cursor.GotoLoc(buffer.Loc{X: 6, Y: 0})
	run(h, "CursorDown", (*BufPane).CursorDown)
	assert.Equal(t, 0, h.Cursor.VirtualX())
	assert.Equal(t, buffer.Loc{X: 4, Y: 1}, h.Cursor.Loc)
	run(h, "CursorRight", (*BufPane).CursorRight)
	assert.Equal(t, buffer.Loc{X: 0, Y: 2}, h.Cursor.Loc)
}

func TestBlockVirtualType(t *testing.T) {
	h := newBlockPane(t, "abcdef\nab\nabcdef")
	h.Buf.Settings["virtualspace"] = true
	h.Cursor.GotoLoc(buffer.Loc{X: 4, Y: 0})
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	assert.Equal(t, 3, h.Buf.NumCursors())
	h.DoRuneInsert('#')
	assert.Equal(t, "abcd#ef\nab  #\nabcd#ef", string(h.Buf.Bytes()))
}

func TestBlockVirtualSel(t *testing.T) {
	h := newBlockPane(t, "abcdef\nab\nabcdef")
	h.Buf.Settings["virtualspace"] = true
	h.Cursor.GotoLoc(buffer.Loc{X: 3, Y: 0})
	for i := 0; i < 3; i++ {
		run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	}
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	c := h.Buf.GetCursor(1) // "ab": selection lives entirely in virtual space
	from, to, ok := c.VirtualSel()
	assert.True(t, ok)
	assert.Equal(t, [2]int{3, 6}, [2]int{from, to})
	assert.False(t, c.HasSelection())
	assert.Equal(t, 4, c.VirtualX())
	// moving the cursor invalidates it
	c.GotoLoc(buffer.Loc{X: 0, Y: 1})
	_, _, ok = c.VirtualSel()
	assert.False(t, ok)
}

func TestDeselectBeforeQuit(t *testing.T) {
	h := newBlockPane(t, "abcdef\nabcdef")
	assert.False(t, h.Deselect()) // nothing selected: the next action in "Deselect|Quit" runs

	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	run(h, "SelectRight", (*BufPane).SelectRight)
	assert.True(t, h.Deselect())
	assert.False(t, h.Cursor.HasSelection())
	assert.False(t, h.Deselect())

	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	assert.Equal(t, 2, h.Buf.NumCursors())
	run(h, "Deselect", (*BufPane).Deselect)
	assert.Equal(t, 1, h.Buf.NumCursors())
	assert.False(t, h.Deselect())
}

func TestBlockMoveDropsBlock(t *testing.T) {
	h := newBlockPane(t, "abcdef\nabcdef\nabcdef\nabcdef")
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "Copy", (*BufPane).Copy)
	assert.Equal(t, 3, h.Buf.NumCursors())

	// moving after the copy leaves one cursor, starting from the block corner
	run(h, "CursorDown", (*BufPane).CursorDown)
	assert.Equal(t, 1, h.Buf.NumCursors())
	assert.False(t, h.Cursor.HasSelection())
	assert.Equal(t, buffer.Loc{X: 2, Y: 3}, h.Cursor.Loc)

	run(h, "Paste", (*BufPane).Paste)
	assert.Equal(t, "abcdef\nabcdef\nabcdef\nabbcdcdef", string(h.Buf.Bytes()[:0])+"abcdef\nabcdef\nabcdef\nabbcdcdef")
}

func TestBlockPasteCRLFAndBeyondEOL(t *testing.T) {
	h := newBlockPane(t, "abcdef\nabcdef\nabcdef\nxy")
	h.Buf.Settings["virtualspace"] = true
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "Copy", (*BufPane).Copy)
	// the system clipboard hands the text back with CRLF line endings
	clipboard.Write("bc\r\nbc\r\n", clipboard.ClipboardReg)

	run(h, "CursorDown", (*BufPane).CursorDown)
	h.Cursor.GotoLoc(buffer.Loc{X: 2, Y: 3})
	for i := 0; i < 3; i++ {
		run(h, "CursorRight", (*BufPane).CursorRight)
	}
	assert.Equal(t, 3, h.Cursor.VirtualX())
	run(h, "Paste", (*BufPane).Paste)
	assert.Equal(t, "abcdef\nabcdef\nabcdef\nxy   bc\n     bc", string(h.Buf.Bytes()))
}

func TestBlockPasteCursorStays(t *testing.T) {
	h := newBlockPane(t, "abcdef\nabcdef\nxy")
	h.Buf.Settings["virtualspace"] = true
	h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	run(h, "BlockSelectRight", (*BufPane).BlockSelectRight)
	run(h, "BlockSelectDown", (*BufPane).BlockSelectDown)
	run(h, "Copy", (*BufPane).Copy)

	h.Buf.ClearCursors()
	h.Cursor = h.Buf.GetActiveCursor()
	h.Cursor.GotoLoc(buffer.Loc{X: 2, Y: 2})
	for i := 0; i < 4; i++ {
		run(h, "CursorRight", (*BufPane).CursorRight)
	}
	run(h, "Paste", (*BufPane).Paste)
	assert.Equal(t, "abcdef\nabcdef\nxy    b\n      b", string(h.Buf.Bytes()))
	assert.Equal(t, buffer.Loc{X: 6, Y: 2}, h.Cursor.Loc)
	assert.Equal(t, 1, h.Buf.NumCursors())
}
