package credential

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/KoukeNeko/taiga-cli/internal/atomicfile"
)

// lockPollInterval is how often a waiting process tries the lock again. The
// lock is held for one refresh request, so a short wait is the common case.
const lockPollInterval = 50 * time.Millisecond

// lockFile takes an exclusive lock on path, creating the file if needed, and
// waits for another holder until ctx ends. An empty path locks nothing.
//
// The lock is tried without blocking and retried, rather than waited on in
// the system call, so that an interrupted command stops waiting at once.
func lockFile(ctx context.Context, path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), atomicfile.DirectoryMode); err != nil {
		return nil, fmt.Errorf("create directory for credentials lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, atomicfile.FileMode) // #nosec G304 -- the path is the tool's own configuration directory
	if err != nil {
		return nil, fmt.Errorf("open credentials lock %q: %w", path, err)
	}
	ticker := time.NewTicker(lockPollInterval)
	defer ticker.Stop()
	for {
		locked, err := tryLock(file)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock credentials %q: %w", path, err)
		}
		if locked {
			return func() {
				_ = unlock(file)
				_ = file.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
