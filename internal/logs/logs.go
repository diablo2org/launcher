// Package logs writes the launcher's log file and catches crashes, so a
// player's problem can be looked at afterwards. Both live in the logs folder
// inside the data folder, and nothing leaves the machine unless the player
// makes a bug report and sends it.
package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sync"
)

// File names in the logs folder.
const (
	// Current is the log being written. Older ones are Current with .1, .2
	// and so on before the extension.
	Current = "launcher.log"
	// Crash collects the trace of any crash.
	Crash = "crash.log"
)

const (
	maxSize = 1 << 20
	keep    = 3
)

// Dir is the logs folder inside a data folder.
func Dir(dataDir string) string {
	return filepath.Join(dataDir, "logs")
}

// Setup sends the default slog logger, and the standard log package with it,
// to the log file, and sends crash traces to the crash file. The returned
// function closes the log file.
func Setup(dataDir, version string) (func() error, error) {
	dir := Dir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	w, err := Open(filepath.Join(dir, Current), maxSize, keep)
	if err != nil {
		return nil, err
	}

	// A crash's trace goes to stderr, which a windowed app has nowhere to
	// show, so keep it in a file. The runtime appends to it.
	crash, err := os.OpenFile(filepath.Join(dir, Crash), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		w.Close()
		return nil, err
	}
	if err := debug.SetCrashOutput(crash, debug.CrashOptions{}); err != nil {
		slog.Warn("crash output", "err", err)
	}
	// SetCrashOutput keeps its own copy of the file.
	crash.Close()

	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("launcher started", "version", version, "os", runtime.GOOS, "arch", runtime.GOARCH, "go", runtime.Version())

	return w.Close, nil
}

// Writer is a log file that starts a new file when it grows past a size,
// keeping a few old ones.
type Writer struct {
	path    string
	maxSize int64
	keep    int

	mu   sync.Mutex
	f    *os.File
	size int64
	// limit is the size that starts a new file: maxSize, or after a failed
	// rotation, another maxSize on from where it failed, so a file held open
	// elsewhere isn't retried on every entry.
	limit int64
}

var _ io.WriteCloser = (*Writer)(nil)

// Open appends to the log file at path. When it passes maxSize it becomes
// the first of keep old files and a new one is started.
func Open(path string, maxSize int64, keep int) (*Writer, error) {
	w := &Writer{path: path, maxSize: maxSize, keep: keep, limit: maxSize}
	if err := w.open(); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}

	w.f, w.size = f, info.Size()
	return nil
}

// Write writes one log entry. An entry is never split across files.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.f == nil {
		return 0, os.ErrClosed
	}

	if w.size > 0 && w.size+int64(len(p)) > w.limit {
		if err := w.rotate(); err != nil {
			if w.f == nil {
				return 0, err
			}
			// The old files couldn't be shifted, so this one grows past the
			// limit, which is better than losing the entry. Try again later.
			n, _ := fmt.Fprintf(w.f, "log rotation failed: %v\n", err)
			w.size += int64(n)
			w.limit = w.size + w.maxSize
		} else {
			w.limit = w.maxSize
		}
	}

	n, err := w.f.Write(p)
	w.size += int64(n)

	return n, err
}

// rotate shifts launcher.log to launcher.1.log and so on, dropping the
// oldest, and starts a new file.
func (w *Writer) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	w.f = nil

	var shiftErr error
	for i := w.keep; i >= 1; i-- {
		to := Old(w.path, i)
		if i == w.keep {
			os.Remove(to)
		}
		if err := os.Rename(Old(w.path, i-1), to); err != nil && !os.IsNotExist(err) {
			shiftErr = err
			break
		}
	}

	if err := w.open(); err != nil {
		return err
	}

	return shiftErr
}

// Close closes the file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.f == nil {
		return nil
	}

	err := w.f.Close()
	w.f = nil

	return err
}

// Old is the name of the nth older log file: launcher.1.log for 1. Old(path,
// 0) is path itself.
func Old(path string, n int) string {
	if n == 0 {
		return path
	}

	ext := filepath.Ext(path)
	return fmt.Sprintf("%s.%d%s", path[:len(path)-len(ext)], n, ext)
}

// AtLeast passes on only records at level or above, for a library that logs
// more than the launcher wants to keep.
func AtLeast(h slog.Handler, level slog.Level) slog.Handler {
	return atLeast{h, level}
}

type atLeast struct {
	slog.Handler
	min slog.Level
}

func (h atLeast) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.min && h.Handler.Enabled(ctx, l)
}

func (h atLeast) WithAttrs(attrs []slog.Attr) slog.Handler {
	return atLeast{h.Handler.WithAttrs(attrs), h.min}
}

func (h atLeast) WithGroup(name string) slog.Handler {
	return atLeast{h.Handler.WithGroup(name), h.min}
}

// urlQuery matches a URL's query and fragment.
var urlQuery = regexp.MustCompile(`(?i)(https?://[^\s?#"']*)[?#][^\s"']*`)

// RedactURLs drops the query and fragment from every URL in s. They can
// carry tokens, and logs end up in bug reports.
func RedactURLs(s string) string {
	return urlQuery.ReplaceAllString(s, "$1")
}
