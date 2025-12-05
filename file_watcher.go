package flux

import (
	"context"
	"fmt"
	"os"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches a file for changes and emits its contents.
type FileWatcher struct {
	path string
}

// NewFileWatcher creates a new FileWatcher for the given file path.
func NewFileWatcher(path string) *FileWatcher {
	return &FileWatcher{path: path}
}

// Watch begins watching the file and returns a channel that emits the file
// contents whenever the file is written. The current file contents are
// emitted immediately to support initial configuration loading.
func (w *FileWatcher) Watch(ctx context.Context) (<-chan []byte, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	if err := watcher.Add(w.path); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch file %s: %w", w.path, err)
	}

	out := make(chan []byte)

	go func() {
		defer close(out)
		defer watcher.Close()

		// Emit initial contents
		if data, err := os.ReadFile(w.path); err == nil {
			select {
			case out <- data:
			case <-ctx.Done():
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Only emit on write or create events
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}

				data, err := os.ReadFile(w.path)
				if err != nil {
					continue
				}

				select {
				case out <- data:
				case <-ctx.Done():
					return
				}

			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
				// Continue watching despite errors
			}
		}
	}()

	return out, nil
}
