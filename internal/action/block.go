package action

import (
	"strings"

	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/clipboard"
	"github.com/zyedidia/micro/v2/internal/util"
)

// Column (block) selection, FAR Manager style. The block is a rectangle in
// visual columns; it is realized as one cursor with a selection per line, so
// all the usual multi-cursor editing (typing, Backspace, Delete) works on it.

type blockSel struct {
	anchorVX, anchorY int
	curVX, curY       int
}

// blockClipText is the clipboard contents last produced by a block copy;
// if the clipboard still holds exactly this text, Paste inserts it as a block.
var blockClipText string

// blockClipLines are the lines of the block copied last
var blockClipLines []string

// blockClipWidth is the visual width of the block copied last. Lines of the
// block that are shorter than that are padded with spaces when pasted in
// front of existing text, so the text after the block moves as one column.
var blockClipWidth int

func (h *BufPane) tabsize() int {
	return int(h.Buf.Settings["tabsize"].(float64))
}

// blockMove moves the active corner of the block by (dx, dy).
func (h *BufPane) blockMove(dx, dy int) bool {
	if h.block == nil {
		h.Buf.ClearCursors()
		c := h.Buf.GetActiveCursor()
		c.ResetSelection()
		vx := c.GetVisualX(false) + c.VirtualX()
		h.block = &blockSel{anchorVX: vx, anchorY: c.Y, curVX: vx, curY: c.Y}
	}
	b := h.block
	b.curVX = util.Max(0, b.curVX+dx)
	b.curY = util.Clamp(b.curY+dy, 0, h.Buf.LinesNum()-1)
	h.applyBlock()
	return true
}

// applyBlock rebuilds the cursors from the block rectangle.
func (h *BufPane) applyBlock() {
	b := h.block
	y1, y2 := util.Min(b.anchorY, b.curY), util.Max(b.anchorY, b.curY)
	vx1, vx2 := util.Min(b.anchorVX, b.curVX), util.Max(b.anchorVX, b.curVX)
	ts := h.tabsize()

	h.Buf.ClearCursors()
	active := 0
	for y := y1; y <= y2; y++ {
		line := h.Buf.LineBytes(y)
		x1 := util.GetCharPosInLine(line, vx1, ts)
		x2 := util.GetCharPosInLine(line, vx2, ts)
		var c *buffer.Cursor
		if y == y1 {
			c = h.Buf.GetActiveCursor()
		} else {
			c = buffer.NewCursor(h.Buf, buffer.Loc{})
		}
		c.CurSelection = [2]buffer.Loc{{X: x1, Y: y}, {X: x2, Y: y}}
		c.OrigSelection = c.CurSelection
		if b.curVX >= b.anchorVX {
			c.Loc = buffer.Loc{X: x2, Y: y}
		} else {
			c.Loc = buffer.Loc{X: x1, Y: y}
		}
		c.LastVisualX = b.curVX
		defer c.SetVirtualSel(vx1, vx2)
		// the side of the block the cursor sits on is shown in virtual space
		// when the line is too short to reach it
		sideVX := vx1
		if b.curVX >= b.anchorVX {
			sideVX = vx2
		}
		if w := util.StringWidth(line, util.CharacterCount(line), ts); c.X == util.CharacterCount(line) && sideVX > w {
			c.SetVirtualX(sideVX - w)
		} else {
			c.ClearVirtual()
		}
		if y != y1 {
			h.Buf.AddCursor(c)
		}
		if y == b.curY {
			active = y - y1
		}
	}
	h.blockCursors = true
	h.Buf.SetCurCursor(active)
	h.Cursor = h.Buf.GetCursor(active)
	h.Relocate()
}

func (h *BufPane) BlockSelectLeft() bool  { return h.blockMove(-1, 0) }
func (h *BufPane) BlockSelectRight() bool { return h.blockMove(1, 0) }
func (h *BufPane) BlockSelectUp() bool    { return h.blockMove(0, -1) }
func (h *BufPane) BlockSelectDown() bool  { return h.blockMove(0, 1) }

// BlockSelectPageUp extends the column selection up by a page
func (h *BufPane) BlockSelectPageUp() bool {
	return h.blockMove(0, -h.BufView().Height)
}

// BlockSelectPageDown extends the column selection down by a page
func (h *BufPane) BlockSelectPageDown() bool {
	return h.blockMove(0, h.BufView().Height)
}

// blockLines returns the selected text of every cursor, one entry per line.
func (h *BufPane) blockLines() []string {
	var lines []string
	for _, c := range h.Buf.GetCursors() {
		lines = append(lines, string(c.GetSelection()))
	}
	return lines
}

// blockCopy copies the block (once, from the first cursor). With cut it also
// deletes it and schedules collapsing to a single cursor at the block's top left.
func (h *BufPane) blockCopy(cut bool) bool {
	cursors := h.Buf.GetCursors()
	text := strings.Join(h.blockLines(), "\n")
	if strings.Trim(text, "\n") == "" {
		return false // nothing is selected (the block lies in virtual space)
	}
	clipboard.Write(text, clipboard.ClipboardReg)
	blockClipText = text
	blockClipLines = h.blockLines()
	blockClipWidth = 0
	if b := h.block; b != nil {
		blockClipWidth = util.Max(b.anchorVX, b.curVX) - util.Min(b.anchorVX, b.curVX)
	}
	vx1 := 0
	if b := h.block; b != nil {
		vx1 = util.Min(b.anchorVX, b.curVX)
	}
	h.freshClip = false
	if cut {
		for _, c := range cursors {
			c.DeleteSelection()
			c.ResetSelection()
		}
		h.collapseAt(cursors[0].Y, vx1)
		InfoBar.Message("Cut block")
	} else {
		InfoBar.Message("Copied block")
	}
	h.Relocate()
	return true
}

// blockPaste inserts lines as a column starting at loc, padding short lines
// with spaces and appending new lines at the end of the buffer if needed.
func (h *BufPane) blockPaste(lines []string, loc buffer.Loc, vx int) {
	ts := h.tabsize()
	for i, s := range lines {
		y := loc.Y + i
		if y >= h.Buf.LinesNum() {
			h.Buf.Insert(h.Buf.End(), "\n")
		}
		line := h.Buf.LineBytes(y)
		w := util.StringWidth(line, util.CharacterCount(line), ts)
		pad := ""
		if w < vx {
			pad = strings.Repeat(" ", vx-w)
		}
		x := util.GetCharPosInLine(line, vx, ts)
		if pad == "" && x < util.CharacterCount(line) {
			// text follows: keep it in one column
			if n := blockClipWidth - util.StringWidth([]byte(s), util.CharacterCount([]byte(s)), ts); n > 0 {
				s += strings.Repeat(" ", n)
			}
		}
		h.Buf.Insert(buffer.Loc{X: x, Y: y}, pad+s)
	}
	h.freshClip = false
	InfoBar.Message("Pasted block")
}

// normClip makes clipboard text comparable: terminals and the system
// clipboard may change line endings or add a trailing newline.
func normClip(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// isBlockClip reports whether the clipboard holds a block copied by this editor.
func isBlockClip(clip string) bool {
	return len(blockClipLines) > 1 && normClip(clip) == normClip(blockClipText)
}

// tryBlockPaste handles Paste for a block clipboard; it reports whether it did.
func (h *BufPane) tryBlockPaste() bool {
	clip, err := clipboard.Read(clipboard.ClipboardReg)
	if err != nil || !isBlockClip(clip) {
		return false
	}
	if h.Cursor.Num != 0 {
		return true // handled once, from the first cursor
	}
	h.pasteBlock()
	return true
}

// pasteBlock inserts the last copied block as a column at the first cursor,
// replacing any block selection.
func (h *BufPane) pasteBlock() {
	cursors := h.Buf.GetCursors()
	// visual column of the block's top left corner
	c0 := cursors[0]
	vx := c0.GetVisualX(false) + c0.VirtualX()
	if c0.HasSelection() {
		x := util.Min(c0.CurSelection[0].X, c0.CurSelection[1].X)
		vx = util.StringWidth(h.Buf.LineBytes(c0.Y), x, h.tabsize())
	}
	for _, c := range cursors {
		if c.HasSelection() {
			c.DeleteSelection()
			c.ResetSelection()
		}
	}
	y := c0.Y
	h.blockPaste(blockClipLines, buffer.Loc{X: c0.X, Y: y}, vx)
	// the cursor stays at the column where the block was pasted
	h.collapseAt(y, vx)
}

// collapseAt schedules collapsing to a single cursor at the visual column vx
// of line y (beyond the end of the line if it is too short).
func (h *BufPane) collapseAt(y, vx int) {
	line := h.Buf.LineBytes(y)
	ts := h.tabsize()
	loc := buffer.Loc{X: util.GetCharPosInLine(line, vx, ts), Y: y}
	h.collapseTo = &loc
	h.collapseVX = 0
	if w := util.StringWidth(line, util.CharacterCount(line), ts); vx > w {
		h.collapseVX = vx - w
	}
}

// finishBlock collapses the cursors after a block cut/paste.
func (h *BufPane) finishBlock() {
	if h.collapseTo == nil {
		return
	}
	loc := *h.collapseTo
	h.collapseTo = nil
	h.Buf.ClearCursors()
	h.Cursor = h.Buf.GetActiveCursor()
	h.Cursor.ResetSelection()
	h.Cursor.GotoLoc(loc)
	h.Cursor.SetVirtualX(h.collapseVX)
	h.Cursor.StoreVisualX()
	h.blockCursors = false
	h.block = nil
	h.Relocate()
}

// mouseVirtual places the cursor in virtual space when the mouse was pressed
// beyond the end of a line.
func (h *BufPane) mouseVirtual(mx int) {
	c := h.Cursor
	c.ClearVirtual()
	if !c.VirtualEnabled() || c.HasSelection() {
		return
	}
	line := h.Buf.LineBytes(c.Y)
	if c.X != util.CharacterCount(line) {
		return
	}
	if vx := h.VisualXFromScreen(mx) - util.StringWidth(line, c.X, h.tabsize()); vx > 0 {
		c.SetVirtualX(vx)
	}
}

// isMoveAction reports whether the action moves the cursor (or extends a plain
// selection). Moving drops a column selection instead of moving all its cursors.
func isMoveAction(name string) bool {
	if name == "MouseMultiCursor" || strings.Contains(name, "MultiCursor") {
		return false
	}
	for _, p := range []string{"Cursor", "Select", "Mouse", "StartOf", "EndOf", "Word", "SubWord",
		"Paragraph", "JumpTo", "Find", "Page", "GotoLoc"} {
		if strings.HasPrefix(name, p) {
			return !strings.HasPrefix(name, "BlockSelect")
		}
	}
	return false
}

// collapseBlock drops a column selection, leaving a single cursor where the
// block's active corner was.
func (h *BufPane) collapseBlock() {
	if !h.blockCursors || h.block == nil {
		return
	}
	active := h.Buf.GetCursor(0)
	for _, c := range h.Buf.GetCursors() {
		if c.Y == h.block.curY {
			active = c
		}
	}
	loc, vx := active.Loc, active.VirtualX()
	lastVX := h.block.curVX
	h.Buf.ClearCursors()
	c := h.Buf.GetActiveCursor()
	c.ResetSelection()
	c.GotoLoc(loc)
	c.SetVirtualX(vx)
	c.LastVisualX = lastVX
	h.Cursor = c
	h.block, h.blockCursors = nil, false
}
