package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultRuntimeLogMaxBytes = int64(10 << 20)
	DefaultRuntimeLogFiles    = 5
)

var oversizedRuntimeLogRecord = []byte("{\"level\":\"ERROR\",\"msg\":\"runtime log record exceeded size limit\"}\n")

// RuntimeLog is a size-bounded rotating writer for process diagnostics. It is
// intentionally separate from the SQLite Admin Log and from backup roots.
type RuntimeLog struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	file     *os.File
	size     int64
}

func OpenRuntimeLog(directory string, maxBytes int64, maxFiles int) (*RuntimeLog, error) {
	if directory == "" {
		return nil, errors.New("runtime log directory is empty")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultRuntimeLogMaxBytes
	}
	if maxFiles <= 0 {
		maxFiles = DefaultRuntimeLogFiles
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime log directory: %w", err)
	}
	logger := &RuntimeLog{
		path: filepath.Join(directory, "prods.log"), maxBytes: maxBytes, maxFiles: maxFiles,
	}
	if err := logger.openAppend(); err != nil {
		return nil, err
	}
	return logger, nil
}

func (logger *RuntimeLog) Write(body []byte) (int, error) {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.file == nil {
		return 0, errors.New("runtime log is closed")
	}
	originalLength := len(body)
	if int64(len(body)) > logger.maxBytes {
		body = oversizedRuntimeLogRecord
	}
	if logger.size > 0 && logger.size+int64(len(body)) > logger.maxBytes {
		if err := logger.rotate(); err != nil {
			return 0, err
		}
	}
	written, err := logger.file.Write(body)
	logger.size += int64(written)
	if err != nil {
		return written, err
	}
	if written != len(body) {
		return written, ioErrShortWrite
	}
	return originalLength, nil
}

func (logger *RuntimeLog) Close() error {
	if logger == nil {
		return nil
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.file == nil {
		return nil
	}
	err := logger.file.Close()
	logger.file = nil
	return err
}

func (logger *RuntimeLog) openAppend() error {
	file, err := openPrivateRuntimeLog(logger.path)
	if err != nil {
		return fmt.Errorf("open runtime log: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("inspect runtime log: %w", err)
	}
	logger.file = file
	logger.size = info.Size()
	return nil
}

func openPrivateRuntimeLog(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY, 0o600)
		if createErr != nil {
			return nil, createErr
		}
		return file, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("runtime log target is not a regular private file")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("runtime log changed while it was being opened")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("restrict runtime log permissions: %w", err)
	}
	return file, nil
}

func (logger *RuntimeLog) rotate() error {
	if err := logger.file.Sync(); err != nil {
		return fmt.Errorf("sync runtime log before rotation: %w", err)
	}
	if err := logger.file.Close(); err != nil {
		return fmt.Errorf("close runtime log before rotation: %w", err)
	}
	logger.file = nil
	oldest := fmt.Sprintf("%s.%d", logger.path, logger.maxFiles)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove oldest runtime log: %w", err)
	}
	for generation := logger.maxFiles - 1; generation >= 1; generation-- {
		source := fmt.Sprintf("%s.%d", logger.path, generation)
		target := fmt.Sprintf("%s.%d", logger.path, generation+1)
		if err := replaceRename(source, target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("rotate runtime log generation %d: %w", generation, err)
		}
	}
	if err := replaceRename(logger.path, logger.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rotate active runtime log: %w", err)
	}
	if err := logger.openAppend(); err != nil {
		return err
	}
	return nil
}

func replaceRename(source, target string) error {
	if _, err := os.Stat(source); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, target)
}

var ioErrShortWrite = errors.New("short runtime log write")
