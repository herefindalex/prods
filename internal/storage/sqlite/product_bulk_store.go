package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"prods/internal/bulkops"
	"prods/internal/catalog"
	"prods/internal/identity"
)

const maxProductBulkSelection = 1000

func (s *Store) PrepareProductBulk(ctx context.Context, actorID string, request bulkops.Request) (bulkops.Run, error) {
	request.Action = bulkops.Action(strings.TrimSpace(string(request.Action)))
	request.TargetID = strings.TrimSpace(request.TargetID)
	if !request.Action.Valid() || len(request.ProductIDs) == 0 || len(request.ProductIDs) > maxProductBulkSelection {
		return bulkops.Run{}, bulkops.ErrInvalidRequest
	}
	needsTarget := request.Action == bulkops.ActionChangeCategory || request.Action == bulkops.ActionChangeLifecycle
	if needsTarget != (request.TargetID != "") {
		return bulkops.Run{}, bulkops.ErrInvalidRequest
	}
	ids := make([]string, 0, len(request.ProductIDs))
	seen := make(map[string]struct{}, len(request.ProductIDs))
	for _, id := range request.ProductIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return bulkops.Run{}, bulkops.ErrInvalidRequest
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return bulkops.Run{}, bulkops.ErrInvalidRequest
	}
	runID, err := randomID("bulk")
	if err != nil {
		return bulkops.Run{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	preview := bulkops.Preview{
		RunID: runID, Action: request.Action, TargetID: request.TargetID,
		SelectionCount: len(ids), CreatedAt: now,
	}
	if request.Action == bulkops.ActionArchive {
		preview.Warning = "Archive makes each successful Product read-only, removes public access, and excludes it from future import updates."
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if request.Action == bulkops.ActionPublish || request.Action == bulkops.ActionHide || request.Action == bulkops.ActionArchive {
			if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
				return err
			}
		}
		switch request.Action {
		case bulkops.ActionChangeCategory:
			if err := requireActiveCategory(ctx, tx, request.TargetID); err != nil {
				return err
			}
		case bulkops.ActionChangeLifecycle:
			var status catalog.EntryStatus
			if err := tx.QueryRowContext(ctx, `SELECT status FROM dictionary_entries WHERE id=? AND kind='lifecycle'`, request.TargetID).Scan(&status); err != nil {
				return err
			}
			if status != catalog.EntryActive {
				return catalog.ErrDisabledReference
			}
		}
		for index, id := range ids {
			product, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, id))
			if err != nil {
				return err
			}
			item := bulkops.PlanItem{
				SelectionIndex: index, ProductID: product.ID, PartNumber: product.PartNumber,
				ExpectedRevision: product.Revision, RecordState: string(product.RecordState),
				PublishingState: string(product.Status), Eligible: product.RecordState == catalog.RecordCurrent,
			}
			if !item.Eligible {
				item.Message = "Archived Products cannot be changed directly."
			}
			preview.Items = append(preview.Items, item)
			if item.Eligible {
				preview.EligibleCount++
			}
		}
		encodedPlan, err := json.Marshal(preview)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(encodedPlan)
		requestHash := hex.EncodeToString(digest[:])
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_bulk_runs(id,actor_id,action,target_id,request_hash,status,plan_json,created_at,updated_at) VALUES(?,?,?,?,?,'prepared',?,?,?)`,
			runID, actorID, request.Action, request.TargetID, requestHash, string(encodedPlan), now, now); err != nil {
			return err
		}
		for _, item := range preview.Items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO product_bulk_items(run_id,product_id,selection_index,expected_revision,status) VALUES(?,?,?,?,'prepared')`,
				runID, item.ProductID, item.SelectionIndex, item.ExpectedRevision); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, actorID, "product_bulk.prepared", "product_bulk", runID, map[string]any{
			"action": request.Action, "target_id": request.TargetID, "selection_count": len(preview.Items),
		})
	})
	if err != nil {
		return bulkops.Run{}, err
	}
	return bulkops.Run{Preview: preview, Status: "prepared"}, nil
}

func (s *Store) ProductBulkRun(ctx context.Context, actorID, runID string) (bulkops.Run, error) {
	var storedActor, status, encodedPlan, encodedReceipt string
	if err := s.db.QueryRowContext(ctx, `SELECT actor_id,status,plan_json,receipt_json FROM product_bulk_runs WHERE id=?`, runID).
		Scan(&storedActor, &status, &encodedPlan, &encodedReceipt); err != nil {
		return bulkops.Run{}, err
	}
	if storedActor != actorID {
		return bulkops.Run{}, ErrPermissionDenied
	}
	run := bulkops.Run{Status: status}
	if err := json.Unmarshal([]byte(encodedPlan), &run.Preview); err != nil {
		return bulkops.Run{}, err
	}
	if encodedReceipt != "" {
		var receipt bulkops.Receipt
		if err := json.Unmarshal([]byte(encodedReceipt), &receipt); err != nil {
			return bulkops.Run{}, err
		}
		run.Receipt = &receipt
	}
	return run, nil
}

func (s *Store) StartProductBulk(ctx context.Context, actorID, runID string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE product_bulk_runs SET status='running',updated_at=? WHERE id=? AND actor_id=? AND status IN ('prepared','running')`,
			time.Now().UTC().Format(time.RFC3339Nano), runID, actorID)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			var status string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM product_bulk_runs WHERE id=? AND actor_id=?`, runID, actorID).Scan(&status); err != nil {
				return err
			}
			if status == "completed" {
				return nil
			}
			return bulkops.ErrRunConflict
		}
		return nil
	})
}

func (s *Store) ProductBulkItemResult(ctx context.Context, runID, productID string) (bulkops.ItemResult, bool, error) {
	var status, encoded string
	err := s.db.QueryRowContext(ctx, `SELECT status,result_json FROM product_bulk_items WHERE run_id=? AND product_id=?`, runID, productID).Scan(&status, &encoded)
	if err != nil {
		return bulkops.ItemResult{}, false, err
	}
	if status == string(bulkops.ItemPrepared) {
		return bulkops.ItemResult{}, false, nil
	}
	var result bulkops.ItemResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		return bulkops.ItemResult{}, false, err
	}
	return result, true, nil
}

func (s *Store) RecordProductBulkItem(ctx context.Context, actorID, runID string, result bulkops.ItemResult) (bulkops.ItemResult, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return bulkops.ItemResult{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var storedActor string
		if err := tx.QueryRowContext(ctx, `SELECT actor_id FROM product_bulk_runs WHERE id=?`, runID).Scan(&storedActor); err != nil {
			return err
		}
		if storedActor != actorID {
			return ErrPermissionDenied
		}
		update, err := tx.ExecContext(ctx, `UPDATE product_bulk_items SET status=?,result_json=?,completed_at=? WHERE run_id=? AND product_id=? AND status='prepared'`,
			result.Status, string(encoded), time.Now().UTC().Format(time.RFC3339Nano), runID, result.ProductID)
		if err != nil {
			return err
		}
		if changed, _ := update.RowsAffected(); changed == 1 {
			return nil
		}
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT result_json FROM product_bulk_items WHERE run_id=? AND product_id=?`, runID, result.ProductID).Scan(&existing); err != nil {
			return err
		}
		return json.Unmarshal([]byte(existing), &result)
	})
	return result, err
}

func (s *Store) CompleteProductBulk(ctx context.Context, actorID, runID string) (bulkops.Receipt, error) {
	var receipt bulkops.Receipt
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var storedActor, status, encodedReceipt string
		if err := tx.QueryRowContext(ctx, `SELECT actor_id,status,receipt_json FROM product_bulk_runs WHERE id=?`, runID).Scan(&storedActor, &status, &encodedReceipt); err != nil {
			return err
		}
		if storedActor != actorID {
			return ErrPermissionDenied
		}
		if status == "completed" {
			if err := json.Unmarshal([]byte(encodedReceipt), &receipt); err != nil {
				return err
			}
			receipt.Replay = true
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT status,result_json FROM product_bulk_items WHERE run_id=? ORDER BY selection_index`, runID)
		if err != nil {
			return err
		}
		defer rows.Close()
		receipt = bulkops.Receipt{RunID: runID}
		for rows.Next() {
			var status, encoded string
			if err := rows.Scan(&status, &encoded); err != nil {
				return err
			}
			if status == string(bulkops.ItemPrepared) {
				return bulkops.ErrRunConflict
			}
			var item bulkops.ItemResult
			if err := json.Unmarshal([]byte(encoded), &item); err != nil {
				return err
			}
			receipt.Results = append(receipt.Results, item)
			switch item.Status {
			case bulkops.ItemSucceeded:
				receipt.Succeeded++
			case bulkops.ItemNoChange:
				receipt.NoChange++
			case bulkops.ItemConflict:
				receipt.Conflicts++
			case bulkops.ItemInvalid:
				receipt.Invalid++
			default:
				receipt.Failed++
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE product_bulk_runs SET status='completed',receipt_json=?,updated_at=? WHERE id=? AND status='running'`, string(encoded), now, runID); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "product_bulk.completed", "product_bulk", runID, receipt)
	})
	return receipt, err
}

func IsProductBulkError(err error) bool {
	return errors.Is(err, bulkops.ErrInvalidRequest) || errors.Is(err, bulkops.ErrRunConflict) ||
		errors.Is(err, catalog.ErrDisabledReference) || errors.Is(err, catalog.ErrInvalidCategory)
}

func (s *Store) ApplyProductBulkEdit(ctx context.Context, actorID, productID string, expectedRevision int64, action bulkops.Action, targetID string) (catalog.Product, bool, error) {
	var updated catalog.Product
	changed := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if action == bulkops.ActionPublish {
			if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
				return err
			}
		}
		product, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, productID))
		if err != nil {
			return err
		}
		if product.Revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		if product.RecordState != catalog.RecordCurrent {
			return catalog.ErrArchivedProduct
		}
		column := ""
		value := ""
		auditAction := ""
		switch action {
		case bulkops.ActionPublish:
			if product.Status == catalog.Published {
				updated = product
				return nil
			}
			product.Status = catalog.Published
			if err := ensureActiveRouteAvailable(ctx, tx, product, product.ID); err != nil {
				return err
			}
			column, value, auditAction = "status", string(catalog.Published), "product.bulk_published"
		case bulkops.ActionChangeCategory:
			if err := requireActiveCategory(ctx, tx, targetID); err != nil {
				return err
			}
			if product.CategoryID == targetID {
				updated = product
				return nil
			}
			product.CategoryID = targetID
			if product.Status == catalog.Published {
				if err := ensureActiveRouteAvailable(ctx, tx, product, product.ID); err != nil {
					return err
				}
			}
			column, value, auditAction = "category_id", targetID, "product.bulk_category_changed"
		case bulkops.ActionChangeLifecycle:
			var status catalog.EntryStatus
			if err := tx.QueryRowContext(ctx, `SELECT status FROM dictionary_entries WHERE id=? AND kind='lifecycle'`, targetID).Scan(&status); err != nil {
				return err
			}
			if status != catalog.EntryActive {
				return catalog.ErrDisabledReference
			}
			if product.LifecycleID == targetID {
				updated = product
				return nil
			}
			product.LifecycleID = targetID
			column, value, auditAction = "lifecycle_id", targetID, "product.bulk_lifecycle_changed"
		default:
			return bulkops.ErrInvalidRequest
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		query := `UPDATE products SET ` + column + `=?,revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=? AND record_state='current'`
		result, err := tx.ExecContext(ctx, query, value, actorID, now, product.ID, expectedRevision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return catalog.ErrRevisionConflict
		}
		if action == bulkops.ActionChangeCategory {
			if err := reconcileProductSpecValues(ctx, tx, product.ID); err != nil {
				return err
			}
		}
		changed = true
		newRevision := expectedRevision + 1
		if err := appendAudit(ctx, tx, actorID, auditAction, "product", product.ID, map[string]any{
			"bulk_action": action, "target_id": targetID, "revision": newRevision,
		}); err != nil {
			return err
		}
		if err := appendPublicationIntent(ctx, tx, product.ID, newRevision, auditAction, now); err != nil {
			return err
		}
		updated, err = scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, product.ID))
		return err
	})
	return updated, changed, err
}
