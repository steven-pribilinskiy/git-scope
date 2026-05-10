// Package clipboard wraps system clipboard writes with WSL-aware behaviour.
//
// On WSL, the underlying X11/Wayland clipboard isn't connected to Windows by
// default, and the cross-platform clipboard libraries (atotto/clipboard,
// golang.design/x/clipboard) either fail outright or write to a buffer no
// Windows app can read. We detect WSL and pipe through `clip.exe` with a
// UTF-16LE encoding so unicode survives the Win32 clipboard format.
package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
)

var (
	detectOnce sync.Once
	isWSLCache bool
)

// Copy writes text to the system clipboard, choosing the right backend for
// the current platform.
func Copy(text string) error {
	if isWSL() {
		return copyWSL(text)
	}
	if clipboard.Unsupported {
		return errors.New("clipboard not supported on this platform")
	}
	return clipboard.WriteAll(text)
}

// isWSL reports whether we're running inside Windows Subsystem for Linux.
// Cached after first call.
func isWSL() bool {
	detectOnce.Do(func() {
		if os.Getenv("WSL_DISTRO_NAME") != "" {
			isWSLCache = true
			return
		}
		if data, err := os.ReadFile("/proc/version"); err == nil {
			isWSLCache = strings.Contains(strings.ToLower(string(data)), "microsoft")
		}
	})
	return isWSLCache
}

// copyWSL pipes text to clip.exe with UTF-16LE encoding via iconv. clip.exe
// expects UTF-16LE on stdin for non-ASCII content; ASCII passes through
// unchanged either way.
func copyWSL(text string) error {
	clipExe, err := exec.LookPath("clip.exe")
	if err != nil {
		return fmt.Errorf("clip.exe not found on PATH (WSL interop disabled?): %w", err)
	}
	if iconv, err := exec.LookPath("iconv"); err == nil {
		conv := exec.Command(iconv, "-t", "utf-16le")
		conv.Stdin = strings.NewReader(text)
		clip := exec.Command(clipExe)
		var buf bytes.Buffer
		conv.Stdout = &buf
		if err := conv.Run(); err != nil {
			return fmt.Errorf("iconv: %w", err)
		}
		clip.Stdin = &buf
		return clip.Run()
	}
	// Fallback: raw bytes. ASCII-safe; non-ASCII may render as mojibake.
	clip := exec.Command(clipExe)
	clip.Stdin = strings.NewReader(text)
	return clip.Run()
}
