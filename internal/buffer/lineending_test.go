package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLineEndings(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		ff    FileFormat
		lines []string
	}{
		{"unix", "a\nb\nc", FFUnix, []string{"a", "b", "c"}},
		{"dos", "a\r\nb\r\nc", FFDos, []string{"a", "b", "c"}},
		{"mac", "a\rb\rc", FFMac, []string{"a", "b", "c"}},
		{"mac with trailing newline", "a\rb\r", FFMac, []string{"a", "b", ""}},
		{"single line has no ending", "abc", FFUnix, []string{"abc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBufferFromString(tc.text, "", BTDefault)
			if tc.name == "single line has no ending" {
				// a file without any line break gets the default format
				assert.Contains(t, []FileFormat{FFUnix, FFDos}, b.Endings)
			} else {
				assert.Equal(t, tc.ff, b.Endings)
			}
			var lines []string
			for i := 0; i < b.LinesNum(); i++ {
				lines = append(lines, string(b.LineBytes(i)))
			}
			assert.Equal(t, tc.lines, lines)
			if tc.name != "single line has no ending" {
				// saving writes the same bytes back
				assert.Equal(t, tc.text, string(b.Bytes()))
				assert.Equal(t, len(tc.text), b.Size())
			}
		})
	}
}

func TestMacFormatSetting(t *testing.T) {
	b := NewBufferFromString("a\rb", "", BTDefault)
	assert.Equal(t, "mac", b.Settings["fileformat"])
	b.DoSetOptionNative("fileformat", "unix")
	assert.Equal(t, "a\nb", string(b.Bytes()))
	b.DoSetOptionNative("fileformat", "mac")
	assert.Equal(t, "a\rb", string(b.Bytes()))
}
