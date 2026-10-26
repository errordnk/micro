package action

// Specification of column (block) selection, copy, cut and paste.
//
// Terms: a block is a rectangle of visual columns [vx1, vx2) over a range of
// lines. It may extend past the end of short lines (virtual space).
//
//   - Selecting: Alt+Shift+arrows grow/shrink the rectangle around a fixed
//     anchor corner. The selection of a line is the text inside the columns;
//     short lines contribute "" (their part of the block lies in virtual space).
//   - Copy: text is the per-line selections joined with "\n"; the rectangle's
//     width is remembered.
//   - Cut: like copy, then the selected text is removed (text to the right
//     moves left); the cursor goes to the block's top left corner.
//   - Paste: the copied lines are INSERTED as a column at the cursor's visual
//     column on consecutive lines starting at the cursor's line; text after
//     the insertion point moves right as one column (lines shorter than the
//     block are padded to its width when text follows). Lines that end before
//     the column are padded with spaces up to it, and missing lines are
//     appended. The cursor ends up on the column where the paste started.
//     A block selection is replaced by the pasted block.
//   - Typing, Backspace and Delete act on every line of the block.
//   - Moving the cursor, or Esc, drops the block and leaves one cursor.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/clipboard"
)

func specPane(t *testing.T, text string) *BufPane {
	h := newBlockPane(t, text)
	h.Buf.Settings["virtualspace"] = true
	h.Buf.Settings["tabsize"] = float64(4)
	blockClipText, blockClipLines, blockClipWidth = "", nil, 0
	return h
}

func bufText(h *BufPane) string { return string(h.Buf.Bytes()) }

var specActions = map[string]BufKeyAction{
	"BlockSelectLeft":  (*BufPane).BlockSelectLeft,
	"BlockSelectRight": (*BufPane).BlockSelectRight,
	"BlockSelectUp":    (*BufPane).BlockSelectUp,
	"BlockSelectDown":  (*BufPane).BlockSelectDown,
	"CursorLeft":       (*BufPane).CursorLeft,
	"CursorRight":      (*BufPane).CursorRight,
	"CursorUp":         (*BufPane).CursorUp,
	"CursorDown":       (*BufPane).CursorDown,
	"Copy":             (*BufPane).Copy,
	"Cut":              (*BufPane).Cut,
	"Paste":            (*BufPane).Paste,
	"Backspace":        (*BufPane).Backspace,
	"Delete":           (*BufPane).Delete,
	"Deselect":         (*BufPane).Deselect,
	"Undo":             (*BufPane).Undo,
}

func key(h *BufPane, name string, n int) {
	for i := 0; i < n; i++ {
		run(h, name, specActions[name])
	}
}

func typeText(h *BufPane, s string) {
	for _, r := range s {
		h.DoRuneInsert(r)
	}
}

// selectBlock puts the cursor at (x, y) and drags the block by dx columns and dy lines.
func selectBlock(h *BufPane, x, y, dx, dy int) {
	h.Buf.ClearCursors()
	h.block, h.blockCursors = nil, false
	h.Cursor = h.Buf.GetActiveCursor()
	h.Cursor.ResetSelection()
	h.Cursor.GotoLoc(buffer.Loc{X: x, Y: y})
	if dx > 0 {
		key(h, "BlockSelectRight", dx)
	} else {
		key(h, "BlockSelectLeft", -dx)
	}
	if dy > 0 {
		key(h, "BlockSelectDown", dy)
	} else {
		key(h, "BlockSelectUp", -dy)
	}
}

// cursorAt puts a single cursor at char (x, y), extra columns beyond the line end.
func cursorAt(h *BufPane, x, y, extra int) {
	h.Buf.ClearCursors()
	h.block, h.blockCursors = nil, false
	h.Cursor = h.Buf.GetActiveCursor()
	h.Cursor.ResetSelection()
	h.Cursor.GotoLoc(buffer.Loc{X: x, Y: y})
	key(h, "CursorRight", extra)
}

// setClip makes the editor think a block of the given width was copied.
func setClip(width int, lines ...string) {
	blockClipText = strings.Join(lines, "\n")
	blockClipLines = lines
	blockClipWidth = width
	clipboard.Write(blockClipText, clipboard.ClipboardReg)
}

func selections(h *BufPane) []string { return h.blockLines() }

func TestBlockSelectGeometry(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		x, y   int
		dx, dy int
		want   []string
	}{
		{"down-right", "abcdef\nghijkl\nmnopqr", 1, 0, 2, 2, []string{"bc", "hi", "no"}},
		{"up-left", "abcdef\nghijkl\nmnopqr", 3, 2, -2, -2, []string{"bc", "hi", "no"}},
		{"down-left", "abcdef\nghijkl\nmnopqr", 3, 0, -2, 2, []string{"bc", "hi", "no"}},
		{"up-right", "abcdef\nghijkl\nmnopqr", 1, 2, 2, -2, []string{"bc", "hi", "no"}},
		{"one column", "abc\ndef", 1, 0, 1, 1, []string{"b", "e"}},
		{"zero width", "abc\ndef", 1, 0, 0, 1, []string{"", ""}},
		{"single line", "abcdef", 1, 0, 3, 0, []string{"bcd"}},
		{"short line in the middle", "abcdef\nab\nabcdef", 1, 0, 3, 2, []string{"bcd", "b", "bcd"}},
		{"short line before block", "abcdef\na\nabcdef", 2, 0, 2, 2, []string{"cd", "", "cd"}},
		{"empty line", "abcdef\n\nabcdef", 2, 0, 2, 2, []string{"cd", "", "cd"}},
		{"block fully beyond all lines", "ab\ncd", 2, 0, 3, 1, []string{"", ""}},
		{"clamped at left edge", "abc\ndef", 1, 0, -5, 1, []string{"a", "d"}},
		{"clamped at buffer end", "abc\ndef", 0, 0, 1, 9, []string{"a", "d"}},
		{"clamped at buffer start", "abc\ndef", 0, 1, 1, -9, []string{"a", "d"}},
		{"wide chars take two columns", "日本語abc\n日本語abc", 1, 0, 2, 1, []string{"本", "本"}},
		{"tab selected when fully covered", "\tab\n\tab", 0, 0, 4, 1, []string{"\t", "\t"}},
		{"tab not selected when partly covered", "\tab\n\tab", 0, 0, 2, 1, []string{"", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := specPane(t, tc.text)
			selectBlock(h, tc.x, tc.y, tc.dx, tc.dy)
			assert.Equal(t, tc.want, selections(h))
			assert.Equal(t, len(tc.want), h.Buf.NumCursors())
			assert.Equal(t, tc.text, bufText(h), "selecting never changes the text")
		})
	}
}

func TestBlockSelectShrinkAndReverse(t *testing.T) {
	h := specPane(t, "abcdef\nghijkl\nmnopqr")
	selectBlock(h, 1, 0, 3, 2)
	assert.Equal(t, []string{"bcd", "hij", "nop"}, selections(h))
	key(h, "BlockSelectLeft", 2) // shrink
	assert.Equal(t, []string{"b", "h", "n"}, selections(h))
	key(h, "BlockSelectUp", 1)
	assert.Equal(t, []string{"b", "h"}, selections(h))
	key(h, "BlockSelectLeft", 1) // zero width
	assert.Equal(t, []string{"", ""}, selections(h))
	key(h, "BlockSelectLeft", 1) // flips to the other side of the anchor
	assert.Equal(t, []string{"a", "g"}, selections(h))
	key(h, "BlockSelectUp", 1)
	assert.Equal(t, []string{"a"}, selections(h))
	assert.Equal(t, 1, h.Buf.NumCursors())
}

func TestBlockSelectInVirtualSpaceKeepsRectangle(t *testing.T) {
	h := specPane(t, "abcdef\nab\nabcdef")
	selectBlock(h, 4, 0, 4, 2) // columns 4..8
	assert.Equal(t, []string{"ef", "", "ef"}, selections(h))
	c := h.Buf.GetCursor(1)
	from, to, ok := c.VirtualSel()
	assert.True(t, ok)
	assert.Equal(t, [2]int{4, 8}, [2]int{from, to})
	assert.Equal(t, 6, c.VirtualX()) // active side is column 8, the line ends at 2
}

func TestBlockCopy(t *testing.T) {
	t.Run("copy keeps text and block", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		key(h, "Copy", 1)
		assert.Equal(t, "abcdef\nghijkl\nmnopqr", bufText(h))
		assert.Equal(t, 3, h.Buf.NumCursors())
		assert.Equal(t, "bc\nhi\nno", blockClipText)
		assert.Equal(t, 2, blockClipWidth)
	})
	t.Run("short lines give empty pieces", func(t *testing.T) {
		h := specPane(t, "abcdef\na\nabcdef")
		selectBlock(h, 2, 0, 2, 2)
		key(h, "Copy", 1)
		assert.Equal(t, []string{"cd", "", "cd"}, blockClipLines)
		assert.Equal(t, 2, blockClipWidth)
	})
	t.Run("zero width copies nothing", func(t *testing.T) {
		h := specPane(t, "abc\ndef")
		setClip(1, "keep", "me")
		selectBlock(h, 1, 0, 0, 1)
		key(h, "Copy", 1)
		assert.Equal(t, "keep\nme", blockClipText)
	})
}

func TestBlockCut(t *testing.T) {
	t.Run("text to the right moves left, cursor at top left", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		key(h, "Cut", 1)
		assert.Equal(t, "adef\ngjkl\nmpqr", bufText(h))
		assert.Equal(t, 1, h.Buf.NumCursors())
		assert.Equal(t, buffer.Loc{X: 1, Y: 0}, h.Cursor.Loc)
		assert.False(t, h.Cursor.HasSelection())
		assert.Equal(t, "bc\nhi\nno", blockClipText)
	})
	t.Run("short line is left alone", func(t *testing.T) {
		h := specPane(t, "abcdef\na\nabcdef")
		selectBlock(h, 2, 0, 2, 2)
		key(h, "Cut", 1)
		assert.Equal(t, "abef\na\nabef", bufText(h))
	})
	t.Run("block reaching past line ends", func(t *testing.T) {
		h := specPane(t, "abcdef\nabc\nabcdef")
		selectBlock(h, 2, 0, 6, 2)
		key(h, "Cut", 1)
		assert.Equal(t, "ab\nab\nab", bufText(h))
		assert.Equal(t, []string{"cdef", "c", "cdef"}, blockClipLines)
		assert.Equal(t, 6, blockClipWidth)
	})
	t.Run("cut starting beyond the end of the top line", func(t *testing.T) {
		h := specPane(t, "ab\nabcdef")
		selectBlock(h, 2, 0, 3, 1) // columns 2..5; the top line has nothing there
		key(h, "Cut", 1)
		assert.Equal(t, "ab\nabf", bufText(h))
		assert.Equal(t, 1, h.Buf.NumCursors())
		assert.Equal(t, buffer.Loc{X: 2, Y: 0}, h.Cursor.Loc)
	})
	t.Run("undo restores everything", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		key(h, "Cut", 1)
		key(h, "Undo", 1)
		assert.Equal(t, "abcdef\nghijkl\nmnopqr", bufText(h))
	})
}

func TestBlockPaste(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		x, y  int // char position of the cursor
		extra int // columns beyond the end of the line
		width int
		clip  []string
		want  string
		cur   buffer.Loc
	}{
		{"start of line: text moves right", "abc\ndef\nghi", 0, 0, 0, 2, []string{"XY", "ZW"},
			"XYabc\nZWdef\nghi", buffer.Loc{X: 0, Y: 0}},
		{"middle: text moves right as a column", "abcd\nefgh\nijkl", 2, 0, 0, 2, []string{"12", "34", "56"},
			"ab12cd\nef34gh\nij56kl", buffer.Loc{X: 2, Y: 0}},
		{"middle of the buffer, not the first line", "abcd\nefgh\nijkl", 1, 1, 0, 1, []string{"1", "2"},
			"abcd\ne1fgh\ni2jkl", buffer.Loc{X: 1, Y: 1}},
		{"end of line: appended, longer lines pushed", "ab\nabcd\nabcd", 2, 0, 0, 2, []string{"12", "34", "56"},
			"ab12\nab34cd\nab56cd", buffer.Loc{X: 2, Y: 0}},
		{"end of line: shorter lines padded", "abcd\nx\nabcd", 4, 0, 0, 2, []string{"12", "34", "56"},
			"abcd12\nx   34\nabcd56", buffer.Loc{X: 4, Y: 0}},
		{"beyond end of line: gap filled with spaces", "ab\nabcdef\nab", 2, 0, 3, 2, []string{"12", "34", "56"},
			"ab   12\nabcde34f\nab   56", buffer.Loc{X: 5, Y: 0}},
		{"beyond end of line: every line starts under the cursor", "a\nabcdefgh\n", 1, 0, 4, 2, []string{"12", "34", "56"},
			"a    12\nabcde34fgh\n     56", buffer.Loc{X: 5, Y: 0}},
		{"below the end of the buffer: lines are appended", "abc\ndef", 1, 1, 0, 1, []string{"1", "2", "3"},
			"abc\nd1ef\n 2\n 3", buffer.Loc{X: 1, Y: 1}},
		{"below the end, beyond the line end", "abc\nxy", 2, 1, 2, 1, []string{"1", "2"},
			"abc\nxy  1\n    2", buffer.Loc{X: 4, Y: 1}},
		{"empty cursor line", "\nabcd\nabcd", 0, 0, 0, 1, []string{"1", "2", "3"},
			"1\n2abcd\n3abcd", buffer.Loc{X: 0, Y: 0}},
		{"empty cursor line, beyond", "\nabcd", 0, 0, 2, 1, []string{"1", "2"},
			"  1\nab2cd", buffer.Loc{X: 2, Y: 0}},
		{"ragged block: short pieces padded when text follows", "abcd\nabcd\nabcd", 1, 0, 0, 3, []string{"123", "", "7"},
			"a123bcd\na   bcd\na7  bcd", buffer.Loc{X: 1, Y: 0}},
		{"ragged block: no trailing spaces at the end of a line", "ab\nab\nab", 2, 0, 0, 3, []string{"123", "", "7"},
			"ab123\nab\nab7", buffer.Loc{X: 2, Y: 0}},
		{"after a tab", "\tab\n\tcd", 1, 0, 0, 1, []string{"1", "2"},
			"\t1ab\n\t2cd", buffer.Loc{X: 1, Y: 0}},
		{"after wide characters", "日本ab\n日本cd", 2, 0, 0, 1, []string{"1", "2"},
			"日本1ab\n日本2cd", buffer.Loc{X: 2, Y: 0}},
		{"one line block is plain text: cursor ends after it", "abcd", 2, 0, 0, 2, []string{"XY"},
			"abXYcd", buffer.Loc{X: 4, Y: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := specPane(t, tc.text)
			setClip(tc.width, tc.clip...)
			cursorAt(h, tc.x, tc.y, tc.extra)
			key(h, "Paste", 1)
			assert.Equal(t, tc.want, bufText(h))
			assert.Equal(t, 1, h.Buf.NumCursors())
			assert.False(t, h.Cursor.HasSelection())
			assert.Equal(t, tc.cur, h.Cursor.Loc, "cursor stays on the column of the paste")
			assert.Equal(t, 0, h.Cursor.VirtualX())
		})
	}
}

func TestBlockPasteUndo(t *testing.T) {
	h := specPane(t, "ab\nabcdef")
	setClip(2, "12", "34", "56")
	cursorAt(h, 2, 0, 3)
	key(h, "Paste", 1)
	assert.Equal(t, "ab   12\nabcde34f\n     56", bufText(h))
	key(h, "Undo", 1)
	assert.Equal(t, "ab\nabcdef", bufText(h))
}

func TestBlockPasteReplacesBlockSelection(t *testing.T) {
	h := specPane(t, "abcdef\nghijkl\nmnopqr")
	selectBlock(h, 1, 0, 2, 2)
	key(h, "Copy", 1)

	selectBlock(h, 3, 0, 2, 1) // "de", "jk"
	key(h, "Paste", 1)
	assert.Equal(t, "abcbcf\nghihil\nmnonopqr", bufText(h))
	assert.Equal(t, 1, h.Buf.NumCursors())
	assert.Equal(t, buffer.Loc{X: 3, Y: 0}, h.Cursor.Loc)
}

func TestBlockTyping(t *testing.T) {
	t.Run("typing replaces the selection on every line", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		typeText(h, "XY")
		assert.Equal(t, "aXYdef\ngXYjkl\nmXYpqr", bufText(h))
	})
	t.Run("typing at a zero width block inserts on every line", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 2, 0, 0, 2)
		typeText(h, "-")
		assert.Equal(t, "ab-cdef\ngh-ijkl\nmn-opqr", bufText(h))
	})
	t.Run("typing beyond the end of short lines pads them", func(t *testing.T) {
		h := specPane(t, "abcdef\nab\nabcdef")
		selectBlock(h, 4, 0, 0, 2)
		typeText(h, "#")
		assert.Equal(t, "abcd#ef\nab  #\nabcd#ef", bufText(h))
	})
	t.Run("backspace deletes the selection", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		key(h, "Backspace", 1)
		assert.Equal(t, "adef\ngjkl\nmpqr", bufText(h))
	})
	t.Run("delete deletes the selection", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		key(h, "Delete", 1)
		assert.Equal(t, "adef\ngjkl\nmpqr", bufText(h))
	})
	t.Run("backspace at a zero width block deletes the char before it", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 2, 0, 0, 2)
		key(h, "Backspace", 1)
		assert.Equal(t, "acdef\ngijkl\nmopqr", bufText(h))
	})
}

func TestBlockDropped(t *testing.T) {
	for _, name := range []string{"CursorUp", "CursorDown", "CursorLeft", "CursorRight", "Deselect"} {
		t.Run(name, func(t *testing.T) {
			h := specPane(t, "abcdef\nghijkl\nmnopqr")
			selectBlock(h, 1, 0, 2, 2)
			key(h, name, 1)
			assert.Equal(t, 1, h.Buf.NumCursors())
			assert.False(t, h.Cursor.HasSelection())
			assert.Equal(t, "abcdef\nghijkl\nmnopqr", bufText(h))
		})
	}
	t.Run("movement starts at the active corner", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2) // active corner: column 3 of line 2
		key(h, "CursorLeft", 1)
		assert.Equal(t, buffer.Loc{X: 2, Y: 2}, h.Cursor.Loc)
	})
	t.Run("block stays through copy and can be copied again", func(t *testing.T) {
		h := specPane(t, "abcdef\nghijkl\nmnopqr")
		selectBlock(h, 1, 0, 2, 2)
		key(h, "Copy", 1)
		key(h, "Copy", 1)
		assert.Equal(t, 3, h.Buf.NumCursors())
	})
}

func TestBlockRoundTrip(t *testing.T) {
	// cut a block and paste it back where it was: the text is unchanged
	h := specPane(t, "abcdef\na\nabcdef\nxyz")
	selectBlock(h, 1, 0, 3, 3)
	key(h, "Cut", 1)
	cursorAt(h, 1, 0, 0)
	key(h, "Paste", 1)
	assert.Equal(t, "abcdef\na\nabcdef\nxyz", bufText(h))
}

func TestBlockClipboardVariants(t *testing.T) {
	h := specPane(t, "abcd\nefgh")
	setClip(1, "1", "2")
	// the system clipboard may return CRLF and a trailing newline
	clipboard.Write("1\r\n2\r\n", clipboard.ClipboardReg)
	cursorAt(h, 1, 0, 0)
	key(h, "Paste", 1)
	assert.Equal(t, "a1bcd\ne2fgh", bufText(h))

	// something else on the clipboard is pasted as plain text
	h = specPane(t, "abcd\nefgh")
	setClip(1, "1", "2")
	clipboard.Write("1\n3", clipboard.ClipboardReg)
	cursorAt(h, 1, 0, 0)
	key(h, "Paste", 1)
	assert.Equal(t, "a1\n3bcd\nefgh", bufText(h))
}

// With virtualspace off the block still covers whole columns, but the cursors
// never leave the text: nothing is shown or padded beyond line ends while selecting.
func TestBlockWithoutVirtualSpace(t *testing.T) {
	h := specPane(t, "abcdef\nab\nabcdef")
	h.Buf.Settings["virtualspace"] = false

	selectBlock(h, 4, 0, 4, 2)
	assert.Equal(t, []string{"ef", "", "ef"}, selections(h))
	for _, c := range h.Buf.GetCursors() {
		assert.Equal(t, 0, c.VirtualX())
	}

	// copy and paste still work as a column; short lines are padded
	key(h, "Copy", 1)
	cursorAt(h, 2, 1, 0)
	key(h, "Paste", 1)
	assert.Equal(t, "abcdef\nabef\nab    cdef\n  ef", bufText(h))
}

func TestBlockPasteTwiceAndUndoRedo(t *testing.T) {
	h := specPane(t, "ab\ncd")
	setClip(1, "1", "2")
	cursorAt(h, 1, 0, 0)
	key(h, "Paste", 1)
	key(h, "Paste", 1)
	assert.Equal(t, "a11b\nc22d", bufText(h))
}
