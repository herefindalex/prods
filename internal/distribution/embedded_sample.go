package distribution

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

//go:embed prods-sample-data-v1.json.gz
var embeddedSampleGZIP []byte

type EmbeddedSample struct {
	Data      SampleData
	SourceURL string
	SHA256    string
}

var embeddedSampleCache struct {
	sync.Once
	sample EmbeddedSample
	err    error
}

// EmbeddedSampleAvailable reports whether the compiled sample payload matches
// the running binary version and passes the same contract validation used by
// the installer. It performs no network access.
func EmbeddedSampleAvailable(version string) bool {
	_, err := LoadEmbeddedSample(version)
	return err == nil
}

// LoadEmbeddedSample expands and validates the release-bound payload compiled
// into the executable. The gzip is deterministic and adds about 48 KiB to the
// binary while keeping empty installation behavior unchanged.
func LoadEmbeddedSample(version string) (EmbeddedSample, error) {
	embeddedSampleCache.Do(func() {
		embeddedSampleCache.sample, embeddedSampleCache.err = decodeEmbeddedSample()
	})
	if embeddedSampleCache.err != nil {
		return EmbeddedSample{}, embeddedSampleCache.err
	}
	version = strings.TrimSpace(version)
	if version == "" || embeddedSampleCache.sample.Data.ReleaseVersion != version {
		return EmbeddedSample{}, fmt.Errorf("%w: embedded sample version %q does not match binary version %q",
			ErrUnavailable, embeddedSampleCache.sample.Data.ReleaseVersion, version)
	}
	return embeddedSampleCache.sample, nil
}

func decodeEmbeddedSample() (EmbeddedSample, error) {
	reader, err := gzip.NewReader(bytes.NewReader(embeddedSampleGZIP))
	if err != nil {
		return EmbeddedSample{}, fmt.Errorf("open embedded sample data: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(reader, maxSampleBytes+1))
	closeErr := reader.Close()
	if readErr != nil {
		return EmbeddedSample{}, fmt.Errorf("read embedded sample data: %w", readErr)
	}
	if closeErr != nil {
		return EmbeddedSample{}, fmt.Errorf("close embedded sample data: %w", closeErr)
	}
	if len(body) > maxSampleBytes {
		return EmbeddedSample{}, errors.New("embedded sample data exceeds size limit")
	}
	var sample SampleData
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sample); err != nil {
		return EmbeddedSample{}, fmt.Errorf("decode embedded sample data: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return EmbeddedSample{}, errors.New("embedded sample data format is invalid")
	}
	if err := validateSampleData(sample, sample.ReleaseVersion); err != nil {
		return EmbeddedSample{}, err
	}
	digest := sha256.Sum256(body)
	return EmbeddedSample{
		Data:      sample,
		SourceURL: "embedded:sample-data/prods-sample-data-v1.json",
		SHA256:    hex.EncodeToString(digest[:]),
	}, nil
}
