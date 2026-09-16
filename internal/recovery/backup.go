package recovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"prods/internal/platform"
	"prods/internal/storage/sqlite"

	_ "modernc.org/sqlite"
)

const (
	ManifestVersion        = 3
	MinimumManifestVersion = 1
)

var (
	ErrInvalidBackup    = errors.New("invalid backup")
	ErrBackupExists     = errors.New("backup already exists")
	ErrUnsafeBackupPath = errors.New("unsafe backup path")
)

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type AssetFile struct {
	ID string `json:"id"`
	ManifestFile
}

type Manifest struct {
	ManifestVersion       int                       `json:"manifest_version"`
	ID                    string                    `json:"id"`
	Kind                  string                    `json:"kind,omitempty"`
	RunID                 string                    `json:"run_id,omitempty"`
	CreatedUTC            string                    `json:"created_utc"`
	ApplicationVersion    string                    `json:"application_version"`
	SQLiteVersion         string                    `json:"sqlite_version,omitempty"`
	SchemaVersion         int                       `json:"schema_version"`
	Database              ManifestFile              `json:"database"`
	Assets                []AssetFile               `json:"assets"`
	Roots                 map[string][]ManifestFile `json:"roots"`
	Files                 map[string]ManifestFile   `json:"files,omitempty"`
	ContentVerified       bool                      `json:"content_verified"`
	ReadOnlyApplied       bool                      `json:"read_only_applied"`
	ContainsSensitiveData bool                      `json:"contains_sensitive_data,omitempty"`
	ExternalRequirements  []string                  `json:"external_requirements,omitempty"`
}

type BackupConfig struct {
	BackupDir            string
	AssetDir             string
	ApplicationVersion   string
	Kind                 string
	RunID                string
	Roots                map[string]string
	Files                map[string]string
	ResourceGate         *platform.ResourceGate
	RequiredBytes        uint64
	RequiredInodes       uint64
	ByteHeadroom         uint64
	InodeHeadroom        uint64
	MinFreePercent       uint8
	ApplyReadOnly        bool
	ExternalRequirements []string
}

type Snapshotter interface {
	Snapshot(context.Context, string) error
}

type pinnedSnapshotter interface {
	SnapshotWithAssetPins(context.Context, string, string) error
	ReleaseAssetPins(context.Context, string) error
}

func CreateBackup(ctx context.Context, store Snapshotter, config BackupConfig) (Manifest, error) {
	if store == nil || strings.TrimSpace(config.BackupDir) == "" || strings.TrimSpace(config.AssetDir) == "" {
		return Manifest{}, ErrInvalidBackup
	}
	externalRequirements, err := normalizeExternalRequirements(config.ExternalRequirements)
	if err != nil {
		return Manifest{}, err
	}
	if config.ResourceGate != nil {
		reservation, _, err := config.ResourceGate.Admit(ctx, platform.ResourceRequest{
			Operation:      "backup",
			Path:           config.BackupDir,
			RequiredBytes:  config.RequiredBytes,
			ByteHeadroom:   config.ByteHeadroom,
			MinFreePercent: config.MinFreePercent,
			RequiredInodes: config.RequiredInodes,
			InodeHeadroom:  config.InodeHeadroom,
		})
		if err != nil {
			return Manifest{}, err
		}
		defer reservation.Release()
	}
	id, err := backupID()
	if err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(config.BackupDir, 0o700); err != nil {
		return Manifest{}, err
	}
	if err := validateBackupTopology(config); err != nil {
		return Manifest{}, err
	}
	staging := filepath.Join(config.BackupDir, "."+id+".incomplete")
	destination := filepath.Join(config.BackupDir, id)
	if _, err := os.Stat(destination); err == nil {
		return Manifest{}, ErrBackupExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return Manifest{}, err
	}
	if err := os.Mkdir(staging, 0o700); err != nil {
		return Manifest{}, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.WriteFile(filepath.Join(staging, "INCOMPLETE"), []byte("backup did not complete\n"), 0o600)
		}
	}()

	databasePath := filepath.Join(staging, "database", "prods.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return Manifest{}, err
	}
	if pinned, ok := store.(pinnedSnapshotter); ok {
		if err := pinned.SnapshotWithAssetPins(ctx, databasePath, id); err != nil {
			return Manifest{}, err
		}
		defer func() { _ = pinned.ReleaseAssetPins(context.Background(), id) }()
	} else if err := store.Snapshot(ctx, databasePath); err != nil {
		return Manifest{}, err
	}
	databaseFile, err := inspectFile(databasePath, "database/prods.db")
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{
		ManifestVersion: ManifestVersion, ID: id, CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		ApplicationVersion: strings.TrimSpace(config.ApplicationVersion), Kind: strings.TrimSpace(config.Kind), RunID: strings.TrimSpace(config.RunID), Database: databaseFile,
		Roots: make(map[string][]ManifestFile), Files: make(map[string]ManifestFile), ContainsSensitiveData: true,
	}
	manifest.ExternalRequirements = externalRequirements
	if err := copySnapshotAssets(ctx, databasePath, config.AssetDir, filepath.Join(staging, "assets"), &manifest); err != nil {
		return Manifest{}, err
	}
	for name, source := range config.Roots {
		name = strings.TrimSpace(name)
		_, duplicate := config.Files[name]
		if name == "" || name == "database" || name == "assets" || !safeManifestRootName(name) || duplicate {
			return Manifest{}, ErrUnsafeBackupPath
		}
		files, err := copyManifestTree(ctx, source, filepath.Join(staging, "roots", name))
		if err != nil {
			return Manifest{}, fmt.Errorf("copy backup root %s: %w", name, err)
		}
		manifest.Roots[name] = files
	}
	for name, source := range config.Files {
		name = strings.TrimSpace(name)
		_, duplicate := config.Roots[name]
		if name == "" || name == "database" || name == "assets" || !safeManifestRootName(name) || duplicate {
			return Manifest{}, ErrUnsafeBackupPath
		}
		destination := filepath.Join(staging, "files", name)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return Manifest{}, err
		}
		if err := copyBackupFile(source, destination, 0o600); err != nil {
			return Manifest{}, fmt.Errorf("copy backup file %s: %w", name, err)
		}
		file, err := inspectFile(destination, filepath.ToSlash(filepath.Join("files", name)))
		if err != nil {
			return Manifest{}, err
		}
		manifest.Files[name] = file
	}
	manifest.ContentVerified = true
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := writeDurableFile(filepath.Join(staging, "manifest.json"), append(encoded, '\n'), 0o600); err != nil {
		return Manifest{}, err
	}
	if err := syncDirectory(staging); err != nil {
		return Manifest{}, err
	}
	if config.ApplyReadOnly {
		manifest.ReadOnlyApplied = applyBackupReadOnly(staging, &manifest)
		if err := syncDirectory(staging); err != nil {
			return Manifest{}, err
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		return Manifest{}, err
	}
	if err := syncDirectory(config.BackupDir); err != nil {
		return Manifest{}, err
	}
	complete = true
	return manifest, nil
}

func copySnapshotAssets(ctx context.Context, databasePath, assetRoot, destination string, manifest *Manifest) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(databasePath)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.QueryRowContext(ctx, `SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&manifest.SchemaVersion); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&manifest.SQLiteVersion); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT id,storage_path,checksum,size_bytes FROM assets ORDER BY id`)
	if err != nil {
		return err
	}
	type assetRow struct {
		id, path, checksum string
		size               int64
	}
	var assets []assetRow
	for rows.Next() {
		var asset assetRow
		if err := rows.Scan(&asset.id, &asset.path, &asset.checksum, &asset.size); err != nil {
			rows.Close()
			return err
		}
		assets = append(assets, asset)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, asset := range assets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !safeRelativePath(asset.path) {
			return fmt.Errorf("%w: asset %s", ErrUnsafeBackupPath, asset.id)
		}
		source := filepath.Join(assetRoot, filepath.FromSlash(asset.path))
		if err := requireContainedRegularFile(assetRoot, source); err != nil {
			return fmt.Errorf("asset %s: %w", asset.id, err)
		}
		target := filepath.Join(destination, filepath.FromSlash(asset.path))
		if err := copyBackupFile(source, target, 0o600); err != nil {
			return fmt.Errorf("copy asset %s: %w", asset.id, err)
		}
		file, err := inspectFile(target, filepath.ToSlash(asset.path))
		if err != nil {
			return err
		}
		if file.SHA256 != asset.checksum || file.Size != asset.size {
			return fmt.Errorf("%w: asset %s checksum or size mismatch", ErrInvalidBackup, asset.id)
		}
		manifest.Assets = append(manifest.Assets, AssetFile{ID: asset.id, ManifestFile: file})
	}
	return nil
}

func copyManifestTree(ctx context.Context, source, destination string) ([]ManifestFile, error) {
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return []ManifestFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrInvalidBackup
	}
	var paths []string
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return ErrUnsafeBackupPath
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || !safeRelativePath(relative) {
			return ErrUnsafeBackupPath
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	files := make([]ManifestFile, 0, len(paths))
	for _, relative := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		target := filepath.Join(destination, relative)
		if err := copyBackupFile(filepath.Join(source, relative), target, 0o600); err != nil {
			return nil, err
		}
		file, err := inspectFile(target, filepath.ToSlash(relative))
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func LoadManifest(backupPath string) (Manifest, error) {
	body, err := os.ReadFile(filepath.Join(backupPath, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidBackup, err)
	}
	if manifest.ManifestVersion < MinimumManifestVersion || manifest.ManifestVersion > ManifestVersion ||
		manifest.ID == "" || manifest.Database.Path != "database/prods.db" {
		return Manifest{}, ErrInvalidBackup
	}
	if manifest.SchemaVersion < 1 || manifest.Database.Size <= 0 || !validManifestFileMetadata(manifest.Database) {
		return Manifest{}, ErrInvalidBackup
	}
	if manifest.ManifestVersion < 2 && len(manifest.Files) != 0 {
		return Manifest{}, ErrInvalidBackup
	}
	if manifest.ManifestVersion < 3 && len(manifest.ExternalRequirements) != 0 {
		return Manifest{}, ErrInvalidBackup
	}
	if _, err := normalizeExternalRequirements(manifest.ExternalRequirements); err != nil {
		return Manifest{}, err
	}
	if manifest.ManifestVersion >= 2 && (strings.TrimSpace(manifest.SQLiteVersion) == "" || !manifest.ContainsSensitiveData) {
		return Manifest{}, ErrInvalidBackup
	}
	seenAssets := make(map[string]struct{}, len(manifest.Assets))
	seenAssetPaths := make(map[string]struct{}, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if asset.ID == "" {
			return Manifest{}, ErrInvalidBackup
		}
		if !safeRelativePath(asset.Path) {
			return Manifest{}, ErrUnsafeBackupPath
		}
		if !validManifestFileMetadata(asset.ManifestFile) {
			return Manifest{}, ErrInvalidBackup
		}
		if _, exists := seenAssets[asset.ID]; exists {
			return Manifest{}, ErrInvalidBackup
		}
		if _, exists := seenAssetPaths[asset.Path]; exists {
			return Manifest{}, ErrInvalidBackup
		}
		seenAssets[asset.ID] = struct{}{}
		seenAssetPaths[asset.Path] = struct{}{}
	}
	for name, files := range manifest.Roots {
		if !safeManifestRootName(name) {
			return Manifest{}, ErrUnsafeBackupPath
		}
		if _, exists := manifest.Files[name]; exists {
			return Manifest{}, ErrInvalidBackup
		}
		seen := make(map[string]struct{}, len(files))
		for _, file := range files {
			if !safeRelativePath(file.Path) {
				return Manifest{}, ErrUnsafeBackupPath
			}
			if !validManifestFileMetadata(file) {
				return Manifest{}, ErrInvalidBackup
			}
			if _, exists := seen[file.Path]; exists {
				return Manifest{}, ErrInvalidBackup
			}
			seen[file.Path] = struct{}{}
		}
	}
	for name, file := range manifest.Files {
		if !safeManifestRootName(name) || !safeRelativePath(file.Path) ||
			file.Path != pathpkg.Join("files", name) {
			return Manifest{}, ErrUnsafeBackupPath
		}
		if !validManifestFileMetadata(file) {
			return Manifest{}, ErrInvalidBackup
		}
	}
	return manifest, nil
}

func validManifestFileMetadata(file ManifestFile) bool {
	if len(file.SHA256) != sha256.Size*2 || file.Size < 0 {
		return false
	}
	_, err := hex.DecodeString(file.SHA256)
	return err == nil
}

func ListBackups(root string) ([]Manifest, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var manifests []Manifest
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		manifest, err := LoadManifest(filepath.Join(root, entry.Name()))
		if err == nil && manifest.ID == entry.Name() {
			manifests = append(manifests, manifest)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].CreatedUTC > manifests[j].CreatedUTC })
	return manifests, nil
}

func inspectFile(path, manifestPath string) (ManifestFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return ManifestFile{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return ManifestFile{}, err
	}
	return ManifestFile{Path: filepath.ToSlash(manifestPath), SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size}, nil
}

func copyBackupFile(source, destination string, mode fs.FileMode) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrUnsafeBackupPath
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func applyBackupReadOnly(root string, manifest *Manifest) bool {
	manifestPath := filepath.Join(root, "manifest.json")
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if path == manifestPath {
			return nil
		}
		return os.Chmod(path, 0o400)
	})
	if err != nil {
		return false
	}
	for index := len(directories) - 1; index >= 1; index-- {
		if err := os.Chmod(directories[index], 0o500); err != nil {
			return false
		}
	}
	manifest.ReadOnlyApplied = true
	if err := replaceManifestFile(manifestPath, *manifest); err != nil {
		manifest.ReadOnlyApplied = false
		return false
	}
	if err := os.Chmod(manifestPath, 0o400); err != nil {
		manifest.ReadOnlyApplied = false
		_ = replaceManifestFile(manifestPath, *manifest)
		return false
	}
	if err := os.Chmod(root, 0o500); err != nil {
		manifest.ReadOnlyApplied = false
		_ = os.Chmod(manifestPath, 0o600)
		_ = replaceManifestFile(manifestPath, *manifest)
		return false
	}
	return true
}

func replaceManifestFile(path string, manifest Manifest) error {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func requireContainedRegularFile(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !pathContains(resolvedRoot, resolvedPath) {
		return ErrUnsafeBackupPath
	}
	info, err := os.Lstat(resolvedPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrUnsafeBackupPath
	}
	return nil
}

func writeDurableFile(path string, body []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		// Windows does not support flushing directory handles. Callers sync
		// every durable file before the rename that reaches this boundary.
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func normalizeExternalRequirements(values []string) ([]string, error) {
	if len(values) > 64 {
		return nil, ErrInvalidBackup
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 200 || strings.ContainsAny(value, "\r\n\x00") {
			return nil, ErrInvalidBackup
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func safeRelativePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `\:`) {
		return false
	}
	clean := pathpkg.Clean(value)
	return clean == value && clean != "." && !pathpkg.IsAbs(clean) && clean != ".." && !strings.HasPrefix(clean, "../")
}

func safeManifestRootName(name string) bool {
	return safeRelativePath(name) && pathpkg.Base(name) == name
}

func validateBackupTopology(config BackupConfig) error {
	backupRoot, err := canonicalBackupBoundary(config.BackupDir)
	if err != nil {
		return err
	}
	directorySources := make([]string, 0, len(config.Roots)+1)
	directorySources = append(directorySources, config.AssetDir)
	for _, source := range config.Roots {
		directorySources = append(directorySources, source)
	}
	for _, source := range directorySources {
		resolved, err := canonicalBackupBoundary(source)
		if err != nil {
			return err
		}
		if pathContains(resolved, backupRoot) || pathContains(backupRoot, resolved) {
			return ErrUnsafeBackupPath
		}
	}
	for _, source := range config.Files {
		resolved, err := canonicalBackupBoundary(source)
		if err != nil {
			return err
		}
		if pathContains(backupRoot, resolved) {
			return ErrUnsafeBackupPath
		}
	}
	return nil
}

func canonicalBackupBoundary(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	probe := filepath.Clean(absolute)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = parent
	}
}

func backupID() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random), nil
}

var _ Snapshotter = (*sqlite.Store)(nil)
