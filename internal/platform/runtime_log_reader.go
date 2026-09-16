package platform

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	DefaultRuntimeLogTailLines = 200
	MaxRuntimeLogTailLines     = 500
	MaxRuntimeLogTailBytes     = int64(256 << 10)
)

type RuntimeLogTail struct {
	Generation int
	FileName   string
	SizeBytes  int64
	Truncated  bool
	Lines      []string
}

type RuntimeLogContent struct {
	Generation int
	FileName   string
	SizeBytes  int64
	Body       string
}

// ReadRuntimeLogTail reads only a bounded tail from the configured runtime log
// family. The base path is trusted host configuration; generation is numeric,
// so this cannot be used to select an arbitrary file.
func ReadRuntimeLogTail(path string, generation, maxFiles, lineLimit int) (RuntimeLogTail, error) {
	if strings.TrimSpace(path) == "" {
		return RuntimeLogTail{}, os.ErrNotExist
	}
	if maxFiles <= 0 {
		maxFiles = DefaultRuntimeLogFiles
	}
	if generation < 0 || generation > maxFiles {
		return RuntimeLogTail{}, fmt.Errorf("runtime log generation must be between 0 and %d", maxFiles)
	}
	if lineLimit <= 0 {
		lineLimit = DefaultRuntimeLogTailLines
	}
	if lineLimit > MaxRuntimeLogTailLines {
		lineLimit = MaxRuntimeLogTailLines
	}
	target := path
	if generation > 0 {
		target = fmt.Sprintf("%s.%d", path, generation)
	}
	info, err := os.Lstat(target)
	if err != nil {
		return RuntimeLogTail{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return RuntimeLogTail{}, errors.New("runtime log target is not a regular private file")
	}
	file, err := os.Open(target)
	if err != nil {
		return RuntimeLogTail{}, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return RuntimeLogTail{}, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return RuntimeLogTail{}, errors.New("runtime log changed while it was being opened")
	}
	info = openedInfo

	start := info.Size() - MaxRuntimeLogTailBytes
	truncated := start > 0
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return RuntimeLogTail{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, MaxRuntimeLogTailBytes+1))
	if err != nil {
		return RuntimeLogTail{}, err
	}
	if int64(len(body)) > MaxRuntimeLogTailBytes {
		body = body[:MaxRuntimeLogTailBytes]
		truncated = true
	}
	if start > 0 {
		if newline := strings.IndexByte(string(body), '\n'); newline >= 0 {
			body = body[newline+1:]
		} else {
			body = nil
		}
	}
	if !utf8.Valid(body) {
		body = []byte(strings.ToValidUTF8(string(body), "�"))
	}
	lines := strings.Split(string(body), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > lineLimit {
		lines = lines[len(lines)-lineLimit:]
		truncated = true
	}
	fileName := "prods.log"
	if generation > 0 {
		fileName = fmt.Sprintf("prods.log.%d", generation)
	}
	return RuntimeLogTail{
		Generation: generation, FileName: fileName, SizeBytes: info.Size(),
		Truncated: truncated, Lines: lines,
	}, nil
}

// ReadRuntimeLogContent reads one complete bounded runtime log generation.
// RuntimeLog rotation caps normal files at DefaultRuntimeLogMaxBytes; the
// explicit limit also rejects unexpectedly large pre-existing files.
func ReadRuntimeLogContent(path string, generation, maxFiles int, maxBytes int64) (RuntimeLogContent, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return RuntimeLogContent{}, os.ErrNotExist
	}
	if maxFiles <= 0 {
		maxFiles = DefaultRuntimeLogFiles
	}
	if generation < 0 || generation > maxFiles {
		return RuntimeLogContent{}, fmt.Errorf("runtime log generation must be 0 through %d", maxFiles)
	}
	if maxBytes <= 0 || maxBytes > DefaultRuntimeLogMaxBytes {
		maxBytes = DefaultRuntimeLogMaxBytes
	}
	target := path
	if generation > 0 {
		target = fmt.Sprintf("%s.%d", path, generation)
	}
	info, err := os.Lstat(target)
	if err != nil {
		return RuntimeLogContent{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return RuntimeLogContent{}, errors.New("runtime log target is not a regular private file")
	}
	if info.Size() > maxBytes {
		return RuntimeLogContent{}, fmt.Errorf("runtime log exceeds %d-byte copy limit", maxBytes)
	}
	file, err := os.Open(target)
	if err != nil {
		return RuntimeLogContent{}, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return RuntimeLogContent{}, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return RuntimeLogContent{}, errors.New("runtime log changed while it was being opened")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return RuntimeLogContent{}, err
	}
	if int64(len(body)) > maxBytes {
		return RuntimeLogContent{}, fmt.Errorf("runtime log exceeds %d-byte copy limit", maxBytes)
	}
	if !utf8.Valid(body) {
		body = []byte(strings.ToValidUTF8(string(body), "�"))
	}
	fileName := "prods.log"
	if generation > 0 {
		fileName = fmt.Sprintf("prods.log.%d", generation)
	}
	return RuntimeLogContent{Generation: generation, FileName: fileName, SizeBytes: openedInfo.Size(), Body: string(body)}, nil
}
