package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrAlreadyOwned = errors.New("Prods data is already owned by another process")

type Ownership struct {
	path string
	file *os.File
	once sync.Once
	err  error
}

// AcquireDatabase locks a file beside the canonical database target. The lock
// file is intentionally retained after release: deleting advisory lock files
// can create two independently locked inodes during a startup race.
func AcquireDatabase(databasePath string) (*Ownership, error) {
	canonical, err := canonicalResourcePath(databasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve database ownership path: %w", err)
	}
	lockPath := canonical + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open ownership file %s: %w", lockPath, err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrAlreadyOwned) {
			return nil, fmt.Errorf("%w: %s", ErrAlreadyOwned, canonical)
		}
		return nil, fmt.Errorf("lock database ownership %s: %w", canonical, err)
	}
	if err := writeOwnershipEvidence(file); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("write ownership evidence %s: %w", lockPath, err)
	}
	return &Ownership{path: canonical, file: file}, nil
}

func (o *Ownership) ResourcePath() string {
	if o == nil {
		return ""
	}
	return o.path
}

func (o *Ownership) Close() error {
	if o == nil || o.file == nil {
		return nil
	}
	o.once.Do(func() {
		if err := unlockFile(o.file); err != nil {
			o.err = err
		}
		if err := o.file.Close(); err != nil && o.err == nil {
			o.err = err
		}
	})
	return o.err
}

func canonicalResourcePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func writeOwnershipEvidence(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.WriteAt([]byte(fmt.Sprintf("pid=%d\n", os.Getpid())), 0); err != nil {
		return err
	}
	return file.Sync()
}
