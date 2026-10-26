package display

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	luar "layeh.com/gopher-luar"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
	lua "github.com/yuin/gopher-lua"
	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/config"
	ulua "github.com/zyedidia/micro/v2/internal/lua"
	"github.com/zyedidia/micro/v2/internal/screen"
	"github.com/zyedidia/micro/v2/internal/util"
)

// StatusLine represents the information line at the bottom
// of each window
// It gives information such as filename, whether the file has been
// modified, filetype, cursor location
type StatusLine struct {
	Info map[string]func(*buffer.Buffer) string

	win *BufWindow
}

var statusInfo = map[string]func(*buffer.Buffer) string{
	"filename": func(b *buffer.Buffer) string {
		return b.GetName()
	},
	"line": func(b *buffer.Buffer) string {
		return strconv.Itoa(b.GetActiveCursor().Y + 1)
	},
	"col": func(b *buffer.Buffer) string {
		return strconv.Itoa(b.GetActiveCursor().X + 1)
	},
	"modified": func(b *buffer.Buffer) string {
		if b.Modified() {
			return "+ "
		}
		if b.Type.Readonly {
			return "[ro] "
		}
		return ""
	},
	// eol shows the line ending of files that do not use plain "\n"
	"eol": func(b *buffer.Buffer) string {
		// only a file that already contains a line break counts: for an empty or
		// one-line file the format is just the default for new files
		if b.LinesNum() < 2 {
			return ""
		}
		switch b.Endings {
		case buffer.FFDos:
			return " [win]"
		case buffer.FFMac:
			return " [mac]"
		}
		return ""
	},
	"overwrite": func(b *buffer.Buffer) string {
		if b.OverwriteMode && !b.Type.Readonly {
			return "[ovwr] "
		}
		return ""
	},
	"lines": func(b *buffer.Buffer) string {
		return strconv.Itoa(b.LinesNum())
	},
	"percentage": func(b *buffer.Buffer) string {
		return strconv.Itoa((b.GetActiveCursor().Y + 1) * 100 / b.LinesNum())
	},
}

func SetStatusInfoFnLua(fn string) {
	luaFn := strings.Split(fn, ".")
	if len(luaFn) <= 1 {
		return
	}
	plName, plFn := luaFn[0], luaFn[1]
	pl := config.FindPlugin(plName)
	if pl == nil {
		return
	}
	statusInfo[fn] = func(b *buffer.Buffer) string {
		if pl == nil || !pl.IsLoaded() {
			return ""
		}
		val, err := pl.Call(plFn, luar.New(ulua.L, b))
		if err == nil {
			if v, ok := val.(lua.LString); !ok {
				screen.TermMessage(plFn, "should return a string")
				return ""
			} else {
				return string(v)
			}
		}
		return ""
	}
}

// NewStatusLine returns a statusline bound to a window
func NewStatusLine(win *BufWindow) *StatusLine {
	s := new(StatusLine)
	s.win = win
	return s
}

// FindOpt finds a given option in the current buffer's settings
func (s *StatusLine) FindOpt(opt string) any {
	if val, ok := s.win.Buf.Settings[opt]; ok {
		return val
	}
	return "null"
}

// displayMessage draws the editor's message (what used to go to the info bar)
// centered in the status line of the active window.
func (s *StatusLine) displayMessage() {
	msg, isErr, ok := statusMessage()
	if !ok || !s.win.IsActive() {
		return
	}
	style := config.DefStyle.Reverse(true)
	if st, ok := config.Colorscheme["statusline"]; ok {
		style = st
	}
	if isErr {
		style = config.DefStyle.Foreground(tcell.ColorBlack).Background(tcell.ColorMaroon)
		if st, ok := config.Colorscheme["error-message"]; ok {
			style = st
		}
	}

	text := " " + strings.Join(strings.Fields(msg), " ") + " "
	width := runewidth.StringWidth(text)
	for width > s.win.Width && len(text) > 0 {
		r := []rune(text)
		text = string(r[:len(r)-1])
		width = runewidth.StringWidth(text)
	}
	y := s.win.Height + s.win.Y - 1
	x := (s.win.Width - width) / 2
	for _, r := range text {
		screen.SetContent(s.win.X+x, y, r, nil, style)
		x += runewidth.RuneWidth(r)
	}
}

var formatParser = regexp.MustCompile(`\$\(.+?\)`)

// Display draws the statusline to the screen
func (s *StatusLine) Display() {
	// We'll draw the line at the lowest line in the window
	y := s.win.Height + s.win.Y - 1

	winX := s.win.X

	b := s.win.Buf
	// autocomplete suggestions (for the buffer, not for the infowindow)
	if b.HasSuggestions && len(b.Suggestions) > 1 {
		statusLineStyle := config.DefStyle.Reverse(true)
		if style, ok := config.Colorscheme["statusline.suggestions"]; ok {
			statusLineStyle = style
		} else if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
		x := 0
		for j, sug := range b.Suggestions {
			style := statusLineStyle
			if b.CurSuggestion == j {
				style = style.Reverse(true)
			}
			for _, r := range sug {
				screen.SetContent(winX+x, y, r, nil, style)
				x++
				if x >= s.win.Width {
					return
				}
			}
			screen.SetContent(winX+x, y, ' ', nil, statusLineStyle)
			x++
			if x >= s.win.Width {
				return
			}
		}

		for x < s.win.Width {
			screen.SetContent(winX+x, y, ' ', nil, statusLineStyle)
			x++
		}
		return
	}

	formatter := func(match []byte) []byte {
		name := match[2 : len(match)-1]
		if bytes.HasPrefix(name, []byte("opt")) {
			option := name[4:]
			return fmt.Append(nil, s.FindOpt(string(option)))
		} else if bytes.HasPrefix(name, []byte("bind")) {
			binding := string(name[5:])
			for k, v := range config.Bindings["buffer"] {
				if v == binding {
					return []byte(k)
				}
			}
			return []byte("null")
		} else {
			if fn, ok := statusInfo[string(name)]; ok {
				return []byte(fn(s.win.Buf))
			}
			return []byte{}
		}
	}

	leftText := []byte(s.win.Buf.Settings["statusformatl"].(string))
	leftText = formatParser.ReplaceAllFunc(leftText, formatter)
	rightFormat := s.win.Buf.Settings["statusformatr"].(string)
	rightText := formatParser.ReplaceAllFunc([]byte(rightFormat), formatter)

	// The line ending marker ([win] or [mac]; nothing for plain "\n") is always
	// shown at the right edge, unless the format places it itself with $(eol).
	if !strings.Contains(rightFormat, "$(eol)") {
		if eol := strings.TrimSpace(statusInfo["eol"](s.win.Buf)); eol != "" {
			rightText = append(bytes.TrimRight(rightText, " "), []byte(" "+eol+" ")...)
		}
	}

	statusLineStyle := config.DefStyle.Reverse(true)
	if s.win.IsActive() {
		if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
	} else {
		if style, ok := config.Colorscheme["statusline.inactive"]; ok {
			statusLineStyle = style
		} else if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
	}

	leftLen := util.StringWidth(leftText, util.CharacterCount(leftText), 1)
	rightLen := util.StringWidth(rightText, util.CharacterCount(rightText), 1)

	defer s.displayMessage()

	for x := 0; x < s.win.Width; x++ {
		if x < leftLen {
			r, combc, size := util.DecodeCharacter(leftText)
			leftText = leftText[size:]
			rw := runewidth.RuneWidth(r)
			for j := 0; j < rw; j++ {
				c := r
				if j > 0 {
					c = ' '
					combc = nil
					x++
				}
				screen.SetContent(winX+x, y, c, combc, statusLineStyle)
			}
		} else if x >= s.win.Width-rightLen && x < rightLen+s.win.Width-rightLen {
			r, combc, size := util.DecodeCharacter(rightText)
			rightText = rightText[size:]
			rw := runewidth.RuneWidth(r)
			for j := 0; j < rw; j++ {
				c := r
				if j > 0 {
					c = ' '
					combc = nil
					x++
				}
				screen.SetContent(winX+x, y, c, combc, statusLineStyle)
			}
		} else {
			screen.SetContent(winX+x, y, ' ', nil, statusLineStyle)
		}
	}
}
