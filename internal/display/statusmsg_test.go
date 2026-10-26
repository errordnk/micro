package display

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
	"github.com/zyedidia/micro/v2/internal/buffer"
	"github.com/zyedidia/micro/v2/internal/config"
	"github.com/zyedidia/micro/v2/internal/info"
	ulua "github.com/zyedidia/micro/v2/internal/lua"
	"github.com/zyedidia/micro/v2/internal/screen"
)

func rowText(y, width int) string {
	var sb strings.Builder
	for x := 0; x < width; x++ {
		r, _, _, _ := screen.Screen.GetContent(x, y)
		sb.WriteRune(r)
	}
	return sb.String()
}

func TestMessageCenteredInStatusLine(t *testing.T) {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	config.GlobalSettings["backup"] = false
	sim, err := screen.InitSimScreen()
	assert.NoError(t, err)
	sim.SetSize(40, 10)

	b := buffer.NewBufferFromString("hello", "", buffer.BTDefault)
	b.Settings["statusformatl"] = "L"
	b.Settings["statusformatr"] = "R"
	w := NewBufWindow(0, 0, 40, 9, b)
	w.SetActive(true)
	ib := info.NewBuffer()
	iw := NewInfoWindow(ib)

	ib.Message("Copied block")
	w.Display()
	iw.Display()
	status := rowText(8, 40) // the status line is the last row of the window
	assert.Equal(t, "L", status[:1])
	assert.Equal(t, "R", status[39:])
	assert.Contains(t, status, " Copied block ")
	idx := strings.Index(status, "Copied block")
	assert.InDelta(t, (40-len(" Copied block "))/2+1, idx, 1)
	assert.Equal(t, strings.Repeat(" ", 40), rowText(9, 40), "info bar stays empty")

	// a prompt still uses the info bar
	ib.Message("")
	ib.Prompt("> ", "", "Command", nil, nil)
	w.Display()
	iw.Display()
	assert.NotContains(t, rowText(8, 40), "Copied")
	assert.Contains(t, rowText(9, 40), ">")
}

func TestYesNoQuestionCenteredInStatusLine(t *testing.T) {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	sim, err := screen.InitSimScreen()
	assert.NoError(t, err)
	sim.SetSize(60, 10)

	b := buffer.NewBufferFromString("hello", "", buffer.BTDefault)
	w := NewBufWindow(0, 0, 60, 9, b)
	w.SetActive(true)
	ib := info.NewBuffer()
	iw := NewInfoWindow(ib)

	ib.YNPrompt("Save changes? (y,n,esc)", func(bool, bool) {})
	w.Display()
	iw.Display()
	assert.Contains(t, rowText(8, 60), " Save changes? (y,n,esc) ")
	assert.Equal(t, strings.Repeat(" ", 60), rowText(9, 60))
}

func TestPromptReplacesStatusLine(t *testing.T) {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	assert.Equal(t, false, config.GlobalSettings["infobar"], "no reserved line by default")
	assert.Equal(t, 0, config.GetInfoBarOffset())
	sim, err := screen.InitSimScreen()
	assert.NoError(t, err)
	sim.SetSize(40, 10)

	b := buffer.NewBufferFromString("hello", "", buffer.BTDefault)
	b.Settings["statusformatl"] = "STATUS"
	w := NewBufWindow(0, 0, 40, 10, b) // the window owns the last row
	w.SetActive(true)
	ib := info.NewBuffer()
	iw := NewInfoWindow(ib)
	iw.Resize(40, 9)

	w.Display()
	iw.Display()
	assert.Contains(t, rowText(9, 40), "STATUS")

	ib.Prompt("> ", "", "Command", nil, nil)
	w.Display()
	iw.Display()
	assert.Contains(t, rowText(9, 40), ">")
	assert.NotContains(t, rowText(9, 40), "STATUS")

	// done: the status line is back
	ib.DonePrompt(false)
	w.Display()
	iw.Display()
	assert.Contains(t, rowText(9, 40), "STATUS")
}

func TestEolStatusInfo(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"a\nb", ""},
		{"a\r\nb", " [win]"},
		{"a\rb", " [mac]"},
	} {
		b := buffer.NewBufferFromString(tc.text, "", buffer.BTDefault)
		assert.Equal(t, tc.want, statusInfo["eol"](b), tc.text)
	}
}

func TestEolMarkerAtTheRightEdge(t *testing.T) {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	sim, err := screen.InitSimScreen()
	assert.NoError(t, err)
	sim.SetSize(40, 10)

	for _, tc := range []struct{ text, want string }{
		{"a\nb", "Ln 1 "},
		{"a\r\nb", "Ln 1 [win] "},
		{"a\rb", "Ln 1 [mac] "},
	} {
		b := buffer.NewBufferFromString(tc.text, "", buffer.BTDefault)
		b.Settings["statusformatl"] = "L"
		b.Settings["statusformatr"] = "Ln 1 " // no $(eol) in the settings
		w := NewBufWindow(0, 0, 40, 9, b)
		w.SetActive(true)
		w.Display()
		row := rowText(8, 40)
		assert.True(t, strings.HasSuffix(row, tc.want), "%q: %q", tc.text, row)
	}
}

func TestEolHiddenWithoutLineBreaks(t *testing.T) {
	for _, text := range []string{"", "abc"} {
		b := buffer.NewBufferFromString(text, "", buffer.BTDefault)
		b.Endings = buffer.FFDos // e.g. the default for new files on Windows
		assert.Equal(t, "", statusInfo["eol"](b), "%q", text)
	}
}
