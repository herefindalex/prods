package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/importing"
)

func (s *Store) BuildImportPreview(ctx context.Context, actorID string, workbook importing.Workbook, mappings []importing.ColumnMapping, mode importing.IdentityMode) (importing.Preview, error) {
	preview := importing.Preview{
		IdentityMode: mode, Mappings: append([]importing.ColumnMapping(nil), mappings...),
		FullyScanned: workbook.FullyScanned, CheckedRows: workbook.CheckedRows, TotalRows: len(workbook.Rows),
	}
	operationID, err := randomID("imp")
	if err != nil {
		return preview, err
	}
	preview.OperationID = operationID
	if mode != importing.IdentityPartNumber && mode != importing.IdentityComposite {
		return preview, importing.ErrInvalidImport
	}
	for _, issue := range importing.ValidateFinalMapping(workbook.Headers, mappings) {
		preview.Issues = append(preview.Issues, importing.RowIssue{
			Sheet: workbook.Sheet, Code: "invalid_mapping", Column: string(issue.Target), Message: issue.Message,
		})
	}
	if !workbook.FullyScanned {
		preview.Issues = append(preview.Issues, importing.RowIssue{
			Sheet: workbook.Sheet, Row: workbook.CheckedRows, Code: "incomplete_scan",
			Message: "the workbook was not fully scanned and cannot be committed",
		})
	}
	if len(preview.Issues) != 0 {
		return preview, nil
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return preview, err
	}
	defer tx.Rollback()
	if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogImport); err != nil {
		return preview, err
	}

	type preparedRow struct {
		row    importing.Row
		values map[importing.Target]string
		key    string
	}
	prepared := make([]preparedRow, 0, len(workbook.Rows))
	keyRows := make(map[string][]int)
	for _, row := range workbook.Rows {
		values := importing.MappedRow(row, mappings)
		part := strings.TrimSpace(values[importing.TargetPartNumber])
		key := part
		if mode == importing.IdentityComposite {
			key = strings.TrimSpace(values[importing.TargetManufacturerID]) + "\x00" + part
		}
		prepared = append(prepared, preparedRow{row: row, values: values, key: key})
		if part != "" && (mode != importing.IdentityComposite || strings.TrimSpace(values[importing.TargetManufacturerID]) != "") {
			keyRows[key] = append(keyRows[key], row.Number)
		}
	}
	duplicateRows := make(map[int]bool)
	for key, rows := range keyRows {
		if len(rows) < 2 {
			continue
		}
		for _, row := range rows {
			duplicateRows[row] = true
			preview.Issues = append(preview.Issues, importing.RowIssue{
				Sheet: workbook.Sheet, Row: row, Column: string(importing.TargetPartNumber), Value: key,
				Code: "duplicate_file_identity", Message: fmt.Sprintf("multiple rows resolve to the same import identity; affected rows: %v", rows),
			})
		}
	}

	for _, input := range prepared {
		partNumber := input.values[importing.TargetPartNumber]
		if strings.TrimSpace(partNumber) == "" {
			preview.Issues = append(preview.Issues, importing.RowIssue{
				Sheet: workbook.Sheet, Row: input.row.Number, Column: string(importing.TargetPartNumber), Value: partNumber,
				Code: "part_number_required", Message: "Part Number is required",
			})
			continue
		}
		if mode == importing.IdentityComposite && strings.TrimSpace(input.values[importing.TargetManufacturerID]) == "" {
			preview.Issues = append(preview.Issues, importing.RowIssue{
				Sheet: workbook.Sheet, Row: input.row.Number, Column: string(importing.TargetManufacturerID),
				Code: "manufacturer_required_for_identity", Message: "Manufacturer is required by the selected composite identity",
			})
			continue
		}
		if duplicateRows[input.row.Number] {
			continue
		}
		matches, err := currentImportMatches(ctx, tx, mode, input.values)
		if err != nil {
			return preview, err
		}
		if len(matches) > 1 {
			preview.Issues = append(preview.Issues, importing.RowIssue{
				Sheet: workbook.Sheet, Row: input.row.Number, Column: string(importing.TargetPartNumber), Value: partNumber,
				Code: "ambiguous_current_match", Message: "the selected identity matches more than one Current product",
			})
			continue
		}
		var product catalog.Product
		item := importing.PlanItem{Sheet: workbook.Sheet, Row: input.row.Number}
		if len(matches) == 0 {
			product.ID, err = randomID("prd")
			if err != nil {
				return preview, err
			}
			item.Action = importing.ActionCreate
			product.RecordState = catalog.RecordCurrent
			product.Status = catalog.Hidden
			product.Revision = 1
			product.CategoryID = catalog.UncategorizedCategoryID
		} else {
			product = matches[0]
			item.Action = importing.ActionUpdate
			item.ExistingID = product.ID
			item.ExpectedRevision = product.Revision
		}
		applyMappedValues(&product, input.values, len(matches) != 0)
		if err := product.Prepare(); err != nil {
			preview.Issues = append(preview.Issues, importing.RowIssue{
				Sheet: workbook.Sheet, Row: input.row.Number, Code: "invalid_product", Message: err.Error(),
			})
			continue
		}
		if err := validateProductReferences(ctx, tx, &product); err != nil {
			preview.Issues = append(preview.Issues, importing.RowIssue{
				Sheet: workbook.Sheet, Row: input.row.Number, Code: "invalid_reference", Message: err.Error(),
			})
			continue
		}
		if err := product.Prepare(); err != nil {
			preview.Issues = append(preview.Issues, importing.RowIssue{Sheet: workbook.Sheet, Row: input.row.Number, Code: "invalid_product", Message: err.Error()})
			continue
		}
		if len(matches) != 0 && sameImportProduct(matches[0], product) {
			item.Action = importing.ActionNoChange
		}
		item.Product = product
		preview.Items = append(preview.Items, item)
		switch item.Action {
		case importing.ActionCreate:
			preview.CreateCount++
		case importing.ActionUpdate:
			preview.UpdateCount++
		case importing.ActionNoChange:
			preview.NoChangeCount++
		}
	}
	preview.FullyValidated = preview.FullyScanned && len(preview.Issues) == 0 && len(preview.Items) == len(workbook.Rows)
	return preview, nil
}

func currentImportMatches(ctx context.Context, tx *sql.Tx, mode importing.IdentityMode, values map[importing.Target]string) ([]catalog.Product, error) {
	where := `record_state='current' AND identity_part_number=?`
	args := []any{strings.TrimSpace(values[importing.TargetPartNumber])}
	if mode == importing.IdentityComposite {
		where += ` AND identity_manufacturer=?`
		args = append(args, "id:"+strings.TrimSpace(values[importing.TargetManufacturerID]))
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+productColumns+` FROM products WHERE `+where+` ORDER BY id LIMIT 3`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []catalog.Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func applyMappedValues(product *catalog.Product, values map[importing.Target]string, preserveBlank bool) {
	set := func(target importing.Target, destination *string) {
		value, mapped := values[target]
		if !mapped || (preserveBlank && strings.TrimSpace(value) == "") {
			return
		}
		*destination = value
	}
	set(importing.TargetPartNumber, &product.PartNumber)
	set(importing.TargetProductName, &product.Name)
	set(importing.TargetManufacturerID, &product.ManufacturerID)
	set(importing.TargetBrandID, &product.BrandID)
	set(importing.TargetCategoryID, &product.CategoryID)
	set(importing.TargetPackageFormFactor, &product.PackageFormFactor)
	set(importing.TargetDescription, &product.Description)
	set(importing.TargetFeatures, &product.Features)
	set(importing.TargetLifecycleID, &product.LifecycleID)
	if value, mapped := values[importing.TargetApplicationIDs]; mapped && (!preserveBlank || strings.TrimSpace(value) != "") {
		product.ApplicationIDs = importing.ReferenceIDs(value)
	}
}

func sameImportProduct(left, right catalog.Product) bool {
	return left.PartNumber == right.PartNumber && left.Name == right.Name && left.ManufacturerID == right.ManufacturerID &&
		left.BrandID == right.BrandID && left.LifecycleID == right.LifecycleID && left.CategoryID == right.CategoryID &&
		left.PackageFormFactor == right.PackageFormFactor && left.Description == right.Description && left.Features == right.Features &&
		slices.Equal(left.ApplicationIDs, right.ApplicationIDs)
}

func (s *Store) CommitImport(ctx context.Context, actorID string, preview importing.Preview) (importing.Receipt, error) {
	if !preview.FullyScanned || !preview.FullyValidated || len(preview.Issues) != 0 || preview.OperationID == "" {
		return importing.Receipt{}, importing.ErrInvalidImport
	}
	encodedPlan, err := json.Marshal(preview)
	if err != nil {
		return importing.Receipt{}, err
	}
	sum := sha256.Sum256(encodedPlan)
	planHash := hex.EncodeToString(sum[:])
	var receipt importing.Receipt
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogImport); err != nil {
			return err
		}
		var existingHash, encodedReceipt string
		err := tx.QueryRowContext(ctx, `SELECT request_hash,receipt_json FROM import_runs WHERE id=?`, preview.OperationID).Scan(&existingHash, &encodedReceipt)
		if err == nil {
			if existingHash != planHash {
				return importing.ErrImportReceiptConflict
			}
			if err := json.Unmarshal([]byte(encodedReceipt), &receipt); err != nil {
				return err
			}
			receipt.Replay = true
			return completeImportJobFromReceipt(ctx, tx, preview.OperationID, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		validated := make([]importing.PlanItem, len(preview.Items))
		copy(validated, preview.Items)
		identities := make(map[string]string)
		ids := make(map[string]bool)
		for index := range validated {
			item := &validated[index]
			product := item.Product
			if product.ID == "" || ids[product.ID] {
				return fmt.Errorf("%w: duplicate or missing product id", importing.ErrImportConflict)
			}
			ids[product.ID] = true
			switch item.Action {
			case importing.ActionCreate:
				product.RecordState = catalog.RecordCurrent
				product.Status = catalog.Hidden
				product.Revision = 1
				product.CreatedBy = actorID
				product.UpdatedBy = actorID
			case importing.ActionUpdate, importing.ActionNoChange:
				current, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, item.ExistingID))
				if err != nil || current.RecordState != catalog.RecordCurrent || current.Revision != item.ExpectedRevision {
					return fmt.Errorf("%w: product %s changed after preview", importing.ErrImportConflict, item.ExistingID)
				}
				product.ID = current.ID
				product.RecordState = catalog.RecordCurrent
				product.Status = current.Status
				product.Revision = current.Revision
				product.CreatedBy = current.CreatedBy
				product.UpdatedBy = actorID
			default:
				return importing.ErrInvalidImport
			}
			if err := product.Prepare(); err != nil {
				return fmt.Errorf("%w: row %d: %v", importing.ErrImportConflict, item.Row, err)
			}
			if err := validateProductReferences(ctx, tx, &product); err != nil {
				return fmt.Errorf("%w: row %d: %v", importing.ErrImportConflict, item.Row, err)
			}
			if err := product.Prepare(); err != nil {
				return fmt.Errorf("%w: row %d: %v", importing.ErrImportConflict, item.Row, err)
			}
			identityKey := product.IdentityMaker + "\x00" + product.IdentityPart
			if other, exists := identities[identityKey]; exists && other != product.ID {
				return fmt.Errorf("%w: duplicate Current identity in plan", importing.ErrImportConflict)
			}
			identities[identityKey] = product.ID
			if err := ensureCurrentIdentityAvailable(ctx, tx, product, product.ID); err != nil {
				return fmt.Errorf("%w: row %d: %v", importing.ErrImportConflict, item.Row, err)
			}
			item.Product = product
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, item := range validated {
			product := item.Product
			switch item.Action {
			case importing.ActionCreate:
				if err := insertProductTx(ctx, tx, product, now); err != nil {
					return err
				}
				if err := appendPublicationIntent(ctx, tx, product.ID, product.Revision, "import.created", now); err != nil {
					return err
				}
			case importing.ActionUpdate:
				newRevision := product.Revision + 1
				result, err := tx.ExecContext(ctx, `UPDATE products SET
					category_id=?,part_number=?,identity_part_number=?,name=?,manufacturer_id=?,manufacturer=?,identity_manufacturer=?,
					brand_id=?,brand=?,lifecycle_id=?,package_form_factor=?,description=?,features=?,revision=?,updated_by=?,
					search_folded=?,search_projection_version=?,updated_at=?
					WHERE id=? AND revision=? AND record_state='current'`, product.CategoryID, product.PartNumber, product.IdentityPart,
					product.Name, nullable(product.ManufacturerID), product.Manufacturer, product.IdentityMaker, nullable(product.BrandID),
					product.Brand, nullable(product.LifecycleID), product.PackageFormFactor, product.Description, product.Features,
					newRevision, actorID, product.SearchFolded, product.ProjectionVer, now, product.ID, product.Revision)
				if err != nil {
					return err
				}
				if changed, _ := result.RowsAffected(); changed != 1 {
					return importing.ErrImportConflict
				}
				if err := replaceProductApplications(ctx, tx, product.ID, product.ApplicationIDs); err != nil {
					return err
				}
				if err := reconcileProductSpecValues(ctx, tx, product.ID); err != nil {
					return err
				}
				if err := appendPublicationIntent(ctx, tx, product.ID, newRevision, "import.updated", now); err != nil {
					return err
				}
			}
		}
		receipt = importing.Receipt{OperationID: preview.OperationID, Created: preview.CreateCount, Updated: preview.UpdateCount, NoChange: preview.NoChangeCount}
		receiptBytes, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO import_runs(
			id,request_hash,status,actor_id,total_rows,created_count,updated_count,no_change_count,failed_count,receipt_json,created_at,completed_at
		) VALUES(?,?,'committed',?,?,?,?,?,0,?,?,?)`, preview.OperationID, planHash, actorID, preview.TotalRows,
			preview.CreateCount, preview.UpdateCount, preview.NoChangeCount, string(receiptBytes), now, now); err != nil {
			return err
		}
		if err := completeImportJobFromReceipt(ctx, tx, preview.OperationID, now); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "import.committed", "import", preview.OperationID, map[string]any{
			"total_rows": preview.TotalRows, "created": preview.CreateCount, "updated": preview.UpdateCount, "no_change": preview.NoChangeCount,
		})
	})
	return receipt, err
}

func completeImportJobFromReceipt(ctx context.Context, tx *sql.Tx, id, now string) error {
	_, err := tx.ExecContext(ctx, `UPDATE import_jobs
		SET status='committed',phase='completed',error_message='',updated_at=?
		WHERE id=? AND status='committing'`, now, id)
	return err
}

func (s *Store) SaveImportTemplate(ctx context.Context, actorID string, expectedVersion int64, template importing.Template) (importing.Template, error) {
	template.Name = strings.TrimSpace(template.Name)
	if template.Name == "" || template.Snapshot.HeaderRow < 1 ||
		(template.Snapshot.IdentityMode != importing.IdentityPartNumber && template.Snapshot.IdentityMode != importing.IdentityComposite) {
		return importing.Template{}, importing.ErrInvalidMapping
	}
	if len(template.Snapshot.Mappings) == 0 {
		return importing.Template{}, importing.ErrInvalidMapping
	}
	if template.ID == "" {
		id, err := randomID("imt")
		if err != nil {
			return importing.Template{}, err
		}
		template.ID = id
	}
	encoded, err := json.Marshal(template.Snapshot)
	if err != nil {
		return importing.Template{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogImport); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var currentVersion int64
		err := tx.QueryRowContext(ctx, `SELECT current_version FROM import_templates WHERE id=?`, template.ID).Scan(&currentVersion)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if expectedVersion != 0 {
				return catalog.ErrRevisionConflict
			}
			template.Version = 1
			if _, err := tx.ExecContext(ctx, `INSERT INTO import_templates(
				id,name,current_version,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,1,?,?,?,?)`, template.ID, template.Name, actorID, actorID, now, now); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if currentVersion != expectedVersion {
				return catalog.ErrRevisionConflict
			}
			template.Version = currentVersion + 1
			result, err := tx.ExecContext(ctx, `UPDATE import_templates SET name=?,current_version=?,updated_by=?,updated_at=?
				WHERE id=? AND current_version=?`, template.Name, template.Version, actorID, now, template.ID, expectedVersion)
			if err != nil {
				return err
			}
			if changed, _ := result.RowsAffected(); changed != 1 {
				return catalog.ErrRevisionConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO import_template_versions(
			template_id,version,snapshot_json,created_by,created_at
		) VALUES(?,?,?,?,?)`, template.ID, template.Version, string(encoded), actorID, now); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "import_template.saved", "import_template", template.ID, map[string]any{"version": template.Version})
	})
	return template, err
}

func (s *Store) ImportTemplate(ctx context.Context, id string, version int64) (importing.Template, error) {
	var template importing.Template
	var encoded string
	if version == 0 {
		err := s.db.QueryRowContext(ctx, `SELECT t.id,t.name,t.current_version,v.snapshot_json
			FROM import_templates t JOIN import_template_versions v ON v.template_id=t.id AND v.version=t.current_version
			WHERE t.id=?`, id).Scan(&template.ID, &template.Name, &template.Version, &encoded)
		if err != nil {
			return importing.Template{}, err
		}
	} else {
		err := s.db.QueryRowContext(ctx, `SELECT t.id,t.name,v.version,v.snapshot_json
			FROM import_templates t JOIN import_template_versions v ON v.template_id=t.id
			WHERE t.id=? AND v.version=?`, id, version).Scan(&template.ID, &template.Name, &template.Version, &encoded)
		if err != nil {
			return importing.Template{}, err
		}
	}
	if err := json.Unmarshal([]byte(encoded), &template.Snapshot); err != nil {
		return importing.Template{}, err
	}
	return template, nil
}
