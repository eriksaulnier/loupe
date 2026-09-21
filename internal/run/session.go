package run

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Session is a running review's shared flock on the run. The kernel drops it when the process ends by any route, so
// an open session is always a live one; several reviews of one run can hold it at once.
type Session struct {
	file *os.File
}

func sessionPath(dir string) string { return filepath.Join(dir, ".review") }

func HoldSession(dir string) (*Session, error) {
	path := sessionPath(dir)
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open review session %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("hold review session %s: %w", path, err)
	}
	return &Session{file: f}, nil
}

func (s *Session) Release() error {
	unlockErr := syscall.Flock(int(s.file.Fd()), syscall.LOCK_UN)
	closeErr := s.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release review session %s: %w", s.file.Name(), unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close review session %s: %w", s.file.Name(), closeErr)
	}
	return nil
}

// SessionOpen reports whether any review of the run is running: an exclusive flock fails only while one holds it.
func SessionOpen(dir string) (bool, error) {
	path := sessionPath(dir)
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open review session %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch {
	case errors.Is(err, syscall.EWOULDBLOCK):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("probe review session %s: %w", path, err)
	}
	return false, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
