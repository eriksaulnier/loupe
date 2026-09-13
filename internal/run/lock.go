package run

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const (
	defaultLockTimeoutMS = 3000
	maxLockTimeoutMS     = 60000
	lockRetryInterval    = 25 * time.Millisecond
)

type Held struct {
	file *os.File
}

// Lock takes the run's exclusive flock. The kernel drops a flock when its holder dies, so a timeout means a live
// contender or a wedged process, never a leftover from a crash.
func Lock(dir, command string, getenv func(string) string) (*Held, error) {
	timeout, err := lockTimeout(getenv)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, heldRefusal(path)
		}
		time.Sleep(lockRetryInterval)
	}
	held := &Held{file: f}
	if err := f.Truncate(0); err != nil {
		_ = held.Unlock()
		return nil, fmt.Errorf("truncate lock %s: %w", path, err)
	}
	if _, err := f.WriteAt([]byte(fmt.Sprintf("%d %s", os.Getpid(), command)), 0); err != nil {
		_ = held.Unlock()
		return nil, fmt.Errorf("write lock %s: %w", path, err)
	}
	return held, nil
}

func (h *Held) Unlock() error {
	unlockErr := syscall.Flock(int(h.file.Fd()), syscall.LOCK_UN)
	closeErr := h.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock %s: %w", h.file.Name(), unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close lock %s: %w", h.file.Name(), closeErr)
	}
	return nil
}

func lockTimeout(getenv func(string) string) (time.Duration, error) {
	v := getenv("LOUPE_LOCK_TIMEOUT_MS")
	if v == "" {
		return defaultLockTimeoutMS * time.Millisecond, nil
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms < 0 || ms > maxLockTimeoutMS {
		return 0, refusal.New(refusal.Usage,
			fmt.Sprintf("LOUPE_LOCK_TIMEOUT_MS must be an integer from 0 to %d, got %q", maxLockTimeoutMS, v),
			fmt.Sprintf("set LOUPE_LOCK_TIMEOUT_MS to an integer from 0 to %d, or unset it for %d", maxLockTimeoutMS, defaultLockTimeoutMS))
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func heldRefusal(path string) error {
	content, err := os.ReadFile(path)
	pid, command, found := strings.Cut(string(content), " ")
	if err != nil || !found || pid == "" {
		return refusal.New(refusal.Lock,
			fmt.Sprintf("run is locked: %s is held by an unknown process", path),
			"wait for the other loupe process to finish, or stop it, then retry")
	}
	return refusal.New(refusal.Lock,
		fmt.Sprintf("run is locked: %s is held by pid %s (%s)", path, pid, command),
		fmt.Sprintf("wait for pid %s (%s) to finish, or stop it, then retry", pid, command))
}
