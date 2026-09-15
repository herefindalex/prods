package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/searchnotify"
)

const maxSearchSubmissionAttempts = 8

func (s *Store) SearchIntegrationSettings(ctx context.Context) (searchnotify.Settings, error) {
	var settings searchnotify.Settings
	err := s.db.QueryRowContext(ctx, `SELECT revision,indexnow_enabled,indexnow_key,google_enabled,google_site_url,updated_at
		FROM search_integration_settings WHERE singleton=1`).Scan(
		&settings.Revision, &settings.IndexNowEnabled, &settings.IndexNowKey,
		&settings.GoogleEnabled, &settings.GoogleSiteURL, &settings.UpdatedAt,
	)
	return settings, err
}

func (s *Store) UpdateSearchIntegrationSettings(ctx context.Context, actorID string, expectedRevision int64, next searchnotify.Settings) (searchnotify.Settings, error) {
	actorID = strings.TrimSpace(actorID)
	next.GoogleSiteURL = strings.TrimSpace(next.GoogleSiteURL)
	if actorID == "" || expectedRevision < 1 || (next.GoogleEnabled && !validGoogleSiteURL(next.GoogleSiteURL)) {
		return searchnotify.Settings{}, searchnotify.ErrInvalidSettings
	}
	if !next.GoogleEnabled {
		next.GoogleSiteURL = ""
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		var current searchnotify.Settings
		if err := tx.QueryRowContext(ctx, `SELECT revision,indexnow_enabled,indexnow_key,google_enabled,google_site_url,updated_at
			FROM search_integration_settings WHERE singleton=1`).Scan(
			&current.Revision, &current.IndexNowEnabled, &current.IndexNowKey,
			&current.GoogleEnabled, &current.GoogleSiteURL, &current.UpdatedAt,
		); err != nil {
			return err
		}
		if current.Revision != expectedRevision {
			return searchnotify.ErrSettingsConflict
		}
		if next.IndexNowEnabled && current.IndexNowKey == "" {
			key, err := identity.NewToken(24)
			if err != nil {
				return err
			}
			digest := sha256.Sum256([]byte(key))
			current.IndexNowKey = hex.EncodeToString(digest[:])
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE search_integration_settings
			SET revision=revision+1,indexnow_enabled=?,indexnow_key=?,google_enabled=?,google_site_url=?,updated_by=?,updated_at=?
			WHERE singleton=1 AND revision=?`, next.IndexNowEnabled, current.IndexNowKey,
			next.GoogleEnabled, next.GoogleSiteURL, actorID, now, expectedRevision)
		if err != nil {
			return err
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			if err != nil {
				return err
			}
			return searchnotify.ErrSettingsConflict
		}
		if current.IndexNowEnabled != next.IndexNowEnabled {
			if _, err := tx.ExecContext(ctx, `DELETE FROM search_notification_state WHERE provider=?`, searchnotify.ProviderIndexNow); err != nil {
				return err
			}
		}
		if current.GoogleEnabled != next.GoogleEnabled || current.GoogleSiteURL != next.GoogleSiteURL {
			if _, err := tx.ExecContext(ctx, `DELETE FROM search_notification_state WHERE provider=?`, searchnotify.ProviderGoogle); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs SET status='canceled',updated_at=?
				WHERE provider=? AND status IN ('pending','retry_wait')`, now, searchnotify.ProviderGoogle); err != nil {
				return err
			}
		}
		if !next.IndexNowEnabled {
			if _, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs SET status='canceled',updated_at=?
				WHERE provider=? AND status IN ('pending','retry_wait')`, now, searchnotify.ProviderIndexNow); err != nil {
				return err
			}
		}
		if !next.GoogleEnabled {
			if _, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs SET status='canceled',updated_at=?
				WHERE provider=? AND status IN ('pending','retry_wait')`, now, searchnotify.ProviderGoogle); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, actorID, "search_integrations.updated", "site", "search-integrations", map[string]any{
			"revision": expectedRevision + 1, "indexnow_enabled": next.IndexNowEnabled,
			"google_enabled": next.GoogleEnabled, "google_site_url": next.GoogleSiteURL,
		})
	})
	if err != nil {
		return searchnotify.Settings{}, err
	}
	return s.SearchIntegrationSettings(ctx)
}

func validGoogleSiteURL(value string) bool {
	if strings.HasPrefix(value, "sc-domain:") {
		host := strings.TrimSpace(strings.TrimPrefix(value, "sc-domain:"))
		return host != "" && !strings.ContainsAny(host, "/?#@")
	}
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func (s *Store) ReconcileSearchSubmissions(ctx context.Context, baseURL string, settings searchnotify.Settings, publications []publishing.ActivePublication) (int, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	currentURLs := make(map[string]string, len(publications))
	for _, publication := range publications {
		if !strings.HasPrefix(publication.Route, "/") {
			return 0, errors.New("public publication route is not absolute")
		}
		publicURL := baseURL + publication.Route
		currentURLs[publicURL] = fmt.Sprintf("%d:%d:%s", publication.SourceRevision, publication.SiteEpoch, publication.ManifestHash)
	}
	created := 0
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if settings.IndexNowEnabled {
			count, err := reconcileIndexNowTx(ctx, tx, currentURLs, now)
			if err != nil {
				return err
			}
			created += count
		}
		if settings.GoogleEnabled {
			var dirty int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_dirty`).Scan(&dirty); err != nil {
				return err
			}
			if dirty != 0 {
				return nil
			}
			sitemapURL := baseURL + "/sitemap.xml"
			fingerprint := searchnotify.SitemapFingerprint(settings, publications)
			var previous string
			err := tx.QueryRowContext(ctx, `SELECT fingerprint FROM search_notification_state WHERE provider=? AND subject=?`,
				searchnotify.ProviderGoogle, sitemapURL).Scan(&previous)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if previous != fingerprint {
				if err := insertSearchJobTx(ctx, tx, searchnotify.ProviderGoogle, sitemapURL, "[]", fingerprint, now); err != nil {
					return err
				}
				created++
				if _, err := tx.ExecContext(ctx, `INSERT INTO search_notification_state(provider,subject,fingerprint,present,updated_at)
					VALUES(?,?,?,?,?) ON CONFLICT(provider,subject) DO UPDATE SET fingerprint=excluded.fingerprint,present=1,updated_at=excluded.updated_at`,
					searchnotify.ProviderGoogle, sitemapURL, fingerprint, 1, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return created, err
}

func reconcileIndexNowTx(ctx context.Context, tx *sql.Tx, current map[string]string, now string) (int, error) {
	previous := make(map[string]struct {
		fingerprint string
		present     bool
	})
	rows, err := tx.QueryContext(ctx, `SELECT subject,fingerprint,present FROM search_notification_state WHERE provider=?`, searchnotify.ProviderIndexNow)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var subject, fingerprint string
		var present bool
		if err := rows.Scan(&subject, &fingerprint, &present); err != nil {
			rows.Close()
			return 0, err
		}
		previous[subject] = struct {
			fingerprint string
			present     bool
		}{fingerprint: fingerprint, present: present}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	type change struct {
		url, fingerprint string
		present          bool
	}
	changes := make([]change, 0)
	for publicURL, fingerprint := range current {
		old, ok := previous[publicURL]
		if !ok || !old.present || old.fingerprint != fingerprint {
			changes = append(changes, change{url: publicURL, fingerprint: fingerprint, present: true})
		}
	}
	for publicURL, old := range previous {
		if old.present {
			if _, ok := current[publicURL]; !ok {
				changes = append(changes, change{url: publicURL, fingerprint: "removed:" + old.fingerprint, present: false})
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].url < changes[j].url })
	for _, item := range changes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_notification_state(provider,subject,fingerprint,present,updated_at)
			VALUES(?,?,?,?,?) ON CONFLICT(provider,subject) DO UPDATE SET fingerprint=excluded.fingerprint,present=excluded.present,updated_at=excluded.updated_at`,
			searchnotify.ProviderIndexNow, item.url, item.fingerprint, item.present, now); err != nil {
			return 0, err
		}
	}
	created := 0
	for start := 0; start < len(changes); start += searchnotify.MaxIndexNowURLs {
		end := start + searchnotify.MaxIndexNowURLs
		if end > len(changes) {
			end = len(changes)
		}
		urls := make([]string, 0, end-start)
		fingerprints := make([]string, 0, end-start)
		for _, item := range changes[start:end] {
			urls = append(urls, item.url)
			fingerprints = append(fingerprints, item.url+"\x00"+item.fingerprint)
		}
		payload, err := searchnotify.EncodeURLs(urls)
		if err != nil {
			return created, err
		}
		if err := insertSearchJobTx(ctx, tx, searchnotify.ProviderIndexNow, fmt.Sprintf("%d URLs", len(urls)), payload, strings.Join(fingerprints, "\n"), now); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func insertSearchJobTx(ctx context.Context, tx *sql.Tx, provider, subject, payload, fingerprint, now string) error {
	id, err := randomID("srch")
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(provider + "\x00" + subject + "\x00" + fingerprint))
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO search_submission_jobs
		(id,provider,subject,payload_json,dedupe_key,status,attempts,next_attempt_at,created_at,updated_at)
		VALUES(?,?,?,?,?,'pending',0,?,?,?)`, id, provider, subject, payload, hex.EncodeToString(digest[:]), now, now, now)
	return err
}

func (s *Store) ResetSearchSubmissionClaims(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs
			SET status='retry_wait',next_attempt_at=?,response_message='interrupted before durable completion receipt',updated_at=?
			WHERE status='processing'`, now, now)
		return err
	})
}

func (s *Store) ClaimSearchSubmission(ctx context.Context, now time.Time, indexNowEnabled, googleEnabled bool) (searchnotify.Job, bool, error) {
	var job searchnotify.Job
	found := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var payload string
		err := tx.QueryRowContext(ctx, `SELECT id,provider,subject,payload_json,status,attempts,next_attempt_at,http_status,response_message,created_at,updated_at
			FROM search_submission_jobs
			WHERE status IN ('pending','retry_wait') AND next_attempt_at<=?
			AND ((provider=? AND ?=1) OR (provider=? AND ?=1))
			ORDER BY next_attempt_at,created_at,id LIMIT 1`, now.UTC().Format(time.RFC3339Nano),
			searchnotify.ProviderIndexNow, indexNowEnabled, searchnotify.ProviderGoogle, googleEnabled).Scan(
			&job.ID, &job.Provider, &job.Subject, &payload, &job.Status, &job.Attempts,
			&job.NextAttemptAt, &job.HTTPStatus, &job.ResponseMessage, &job.CreatedAt, &job.UpdatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if job.Provider == searchnotify.ProviderIndexNow {
			urls, err := searchnotify.DecodeURLs(payload)
			if err != nil {
				return err
			}
			job.URLs = urls
		}
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs SET status='processing',attempts=attempts+1,updated_at=?
			WHERE id=? AND status IN ('pending','retry_wait')`, stamp, job.ID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return errors.New("search submission claim was lost")
		}
		job.Status = "processing"
		job.Attempts++
		job.UpdatedAt = stamp
		found = true
		return nil
	})
	return job, found, err
}

func (s *Store) CompleteSearchSubmission(ctx context.Context, jobID string, httpStatus int, message string) error {
	message = searchnotify.SafeResponseMessage(message)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs
			SET status='accepted',http_status=?,response_message=?,accepted_at=?,updated_at=? WHERE id=? AND status='processing'`,
			httpStatus, message, now, now, jobID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			if err != nil {
				return err
			}
			return errors.New("search submission completion did not match a processing job")
		}
		return nil
	})
}

func (s *Store) FailSearchSubmission(ctx context.Context, jobID string, httpStatus int, message string, retryable bool, now time.Time) error {
	message = searchnotify.SafeResponseMessage(message)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var attempts int
		if err := tx.QueryRowContext(ctx, `SELECT attempts FROM search_submission_jobs WHERE id=? AND status='processing'`, jobID).Scan(&attempts); err != nil {
			return err
		}
		status := "failed"
		next := now.UTC()
		if retryable && attempts < maxSearchSubmissionAttempts {
			status = "retry_wait"
			next = next.Add(searchnotify.RetryDelay(attempts))
		}
		_, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs
			SET status=?,next_attempt_at=?,http_status=?,response_message=?,updated_at=? WHERE id=? AND status='processing'`,
			status, next.Format(time.RFC3339Nano), httpStatus, message, now.UTC().Format(time.RFC3339Nano), jobID)
		return err
	})
}

func (s *Store) RecentSearchSubmissionJobs(ctx context.Context, limit int) ([]searchnotify.Job, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,provider,subject,payload_json,status,attempts,next_attempt_at,http_status,response_message,created_at,updated_at
		FROM search_submission_jobs ORDER BY updated_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]searchnotify.Job, 0)
	for rows.Next() {
		var job searchnotify.Job
		var payload string
		if err := rows.Scan(&job.ID, &job.Provider, &job.Subject, &payload, &job.Status, &job.Attempts,
			&job.NextAttemptAt, &job.HTTPStatus, &job.ResponseMessage, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		if job.Provider == searchnotify.ProviderIndexNow {
			job.URLs, _ = searchnotify.DecodeURLs(payload)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) RetrySearchSubmission(ctx context.Context, actorID, jobID string) (searchnotify.Job, error) {
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE search_submission_jobs
			SET status='pending',next_attempt_at=?,http_status=0,response_message='',updated_at=? WHERE id=? AND status='failed'`, now, now, jobID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return searchnotify.ErrJobNotRetryable
		}
		return appendAudit(ctx, tx, actorID, "search_submission.retry_requested", "search_submission", jobID, map[string]any{})
	})
	if err != nil {
		return searchnotify.Job{}, err
	}
	jobs, err := s.RecentSearchSubmissionJobs(ctx, 100)
	if err != nil {
		return searchnotify.Job{}, err
	}
	for _, job := range jobs {
		if job.ID == jobID {
			return job, nil
		}
	}
	return searchnotify.Job{}, sql.ErrNoRows
}
