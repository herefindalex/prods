package recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"prods/internal/storage/sqlite"
)

type RetentionResult struct {
	Reasons map[string][]string `json:"reasons"`
	Expired []string            `json:"expired"`
	Deleted []string            `json:"deleted"`
}

func ApplyBackupRetention(ctx context.Context, backupRoot string, settings sqlite.BackupSettings, now time.Time) (RetentionResult, error) {
	return backupRetention(ctx, backupRoot, settings, now, true)
}

func EvaluateBackupRetention(ctx context.Context, backupRoot string, settings sqlite.BackupSettings, now time.Time) (RetentionResult, error) {
	return backupRetention(ctx, backupRoot, settings, now, false)
}

func backupRetention(ctx context.Context, backupRoot string, settings sqlite.BackupSettings, now time.Time, deleteExpired bool) (RetentionResult, error) {
	result := RetentionResult{Reasons: make(map[string][]string)}
	manifests, err := ListBackups(backupRoot)
	if err != nil {
		return result, err
	}
	if len(manifests) == 0 {
		return result, nil
	}
	location, err := time.LoadLocation(settings.TimeZone)
	if err != nil {
		return result, err
	}
	daily := dateBuckets(now.In(location), settings.RetentionDaily)
	weekly := weekBuckets(now.In(location), settings.RetentionWeekly)
	monthly := monthBuckets(now.In(location), settings.RetentionMonthly)
	selectedDaily := make(map[string]bool)
	selectedWeekly := make(map[string]bool)
	selectedMonthly := make(map[string]bool)
	preUpgrade := 0
	preRestore := 0
	for _, manifest := range manifests {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		created, err := time.Parse(time.RFC3339Nano, manifest.CreatedUTC)
		if err != nil {
			continue
		}
		if created.After(now) {
			result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "clock-skew")
			continue
		}
		switch manifest.Kind {
		case "scheduled":
			local := created.In(location)
			dateKey := local.Format("2006-01-02")
			if daily[dateKey] && !selectedDaily[dateKey] {
				result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "daily:"+dateKey)
				selectedDaily[dateKey] = true
			}
			weekKey := isoWeekKey(local)
			if weekly[weekKey] && !selectedWeekly[weekKey] {
				result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "weekly:"+weekKey)
				selectedWeekly[weekKey] = true
			}
			monthKey := local.Format("2006-01")
			if monthly[monthKey] && !selectedMonthly[monthKey] {
				result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "monthly:"+monthKey)
				selectedMonthly[monthKey] = true
			}
		case "pre-upgrade":
			if preUpgrade < settings.RetentionPreUpgrade {
				result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "pre-upgrade")
				preUpgrade++
			}
		case "pre-restore":
			if preRestore < settings.RetentionPreRestore {
				result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "pre-restore")
				preRestore++
			}
		default:
			result.Reasons[manifest.ID] = append(result.Reasons[manifest.ID], "manual")
		}
	}
	if len(result.Reasons[manifests[0].ID]) == 0 {
		result.Reasons[manifests[0].ID] = []string{"last-good"}
	}
	for _, manifest := range manifests {
		if len(result.Reasons[manifest.ID]) != 0 {
			sort.Strings(result.Reasons[manifest.ID])
			continue
		}
		result.Expired = append(result.Expired, manifest.ID)
		if !deleteExpired {
			continue
		}
		if err := removeBackupPoint(backupRoot, manifest.ID); err != nil {
			return result, fmt.Errorf("remove expired backup %s: %w", manifest.ID, err)
		}
		result.Deleted = append(result.Deleted, manifest.ID)
	}
	if len(result.Deleted) > 0 {
		if err := syncDirectory(backupRoot); err != nil {
			return result, err
		}
	}
	return result, nil
}

func dateBuckets(now time.Time, count int) map[string]bool {
	result := make(map[string]bool, count)
	for offset := 0; offset < count; offset++ {
		result[now.AddDate(0, 0, -offset).Format("2006-01-02")] = true
	}
	return result
}

func weekBuckets(now time.Time, count int) map[string]bool {
	result := make(map[string]bool, count)
	weekday := (int(now.Weekday()) + 6) % 7
	monday := time.Date(now.Year(), now.Month(), now.Day()-weekday, 12, 0, 0, 0, now.Location())
	for offset := 0; offset < count; offset++ {
		result[isoWeekKey(monday.AddDate(0, 0, -7*offset))] = true
	}
	return result
}

func monthBuckets(now time.Time, count int) map[string]bool {
	result := make(map[string]bool, count)
	month := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, now.Location())
	for offset := 0; offset < count; offset++ {
		result[month.AddDate(0, -offset, 0).Format("2006-01")] = true
	}
	return result
}

func isoWeekKey(value time.Time) string {
	year, week := value.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func removeBackupPoint(root, id string) error {
	id = strings.TrimSpace(id)
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return ErrUnsafeBackupPath
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(filepath.Join(absoluteRoot, id))
	if err != nil {
		return err
	}
	if filepath.Dir(target) != absoluteRoot {
		return ErrUnsafeBackupPath
	}
	if err := filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return os.Chmod(path, 0o600)
	}); err != nil {
		return err
	}
	return os.RemoveAll(target)
}
