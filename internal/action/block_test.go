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
