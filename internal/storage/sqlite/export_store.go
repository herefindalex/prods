package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"prods/internal/identity"
)

type ProductExportRow struct {
	ID                string
	PartNumber        string
	Manufacturer      string
	Brand             string
	Name              string
	CategoryID        string
	Lifecycle         string
	PackageFormFactor string
	Description       string
	Features          string
	Specification     string
	DocumentURL       string
	RecordState       string
	Status            string
	Revision          int64
	CreatedAt         string
	UpdatedAt         string
}

type RawSpecExportRow struct {
	ProductID      string
	SpecID         string
	SpecName       string
	RawValue       string
	SourceLocale   string
	SourceRevision int64
	Active         bool
	UpdatedAt      string
}

type RFQExportRow struct {
	RFQID          string
	Status         string
	Revision       int64
	Name           string
	Email          string
	Company        string
	Phone          string
	Country        string
	GeneralMessage string
	Recipients     string
	PrivacyState   string
	PrivacyAt      string
	CreatedAt      string
	UpdatedAt      string
	ItemIndex      int
	ItemKind       string
	ProductID      string
	Requested      string
	RawQuery       string
	Quantity       string
	Notes          string
	PublicSnapshot string
}

func (s *Store) ForEachProductExport(ctx context.Context, actorID string,
	product func(ProductExportRow) error, rawSpec func(RawSpecExportRow) error) error {
	if strings.TrimSpace(actorID) == "" || product == nil || rawSpec == nil {
		return ErrPermissionDenied
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogExport); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.id,p.part_number,
		COALESCE(m.name,p.manufacturer),COALESCE(b.name,p.brand),p.name,p.category_id,
		COALESCE(l.name,''),p.package_form_factor,p.description,p.features,p.specification,p.document_url,
		p.record_state,p.status,p.revision,p.created_at,p.updated_at
		FROM products p
		LEFT JOIN dictionary_entries m ON m.id=p.manufacturer_id
		LEFT JOIN dictionary_entries b ON b.id=p.brand_id
		LEFT JOIN dictionary_entries l ON l.id=p.lifecycle_id
		ORDER BY p.part_number,p.id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var row ProductExportRow
		if err := rows.Scan(&row.ID, &row.PartNumber, &row.Manufacturer, &row.Brand, &row.Name, &row.CategoryID,
			&row.Lifecycle, &row.PackageFormFactor, &row.Description, &row.Features, &row.Specification,
			&row.DocumentURL, &row.RecordState, &row.Status, &row.Revision, &row.CreatedAt, &row.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		if err := product(row); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT v.product_id,v.spec_id,d.name,v.raw_value,v.source_locale,v.source_revision,v.active,v.updated_at
		FROM product_spec_values v JOIN spec_definitions d ON d.id=v.spec_id
		ORDER BY v.product_id,d.name,v.spec_id,v.source_locale`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var row RawSpecExportRow
		if err := rows.Scan(&row.ProductID, &row.SpecID, &row.SpecName, &row.RawValue, &row.SourceLocale,
			&row.SourceRevision, &row.Active, &row.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		if err := rawSpec(row); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ForEachRFQExport(ctx context.Context, actorID string, visit func(RFQExportRow) error) error {
	if strings.TrimSpace(actorID) == "" || visit == nil {
		return ErrPermissionDenied
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityRFQExport); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.status,r.revision,r.name,r.email,r.company,r.phone,r.country,r.general_message,
		COALESCE((SELECT group_concat(address,'; ') FROM (
			SELECT CASE WHEN rr.kind='user' THEN COALESCE(u.email,'') ELSE rr.email END AS address
			FROM rfq_recipients rr LEFT JOIN users u ON u.id=rr.user_id
			WHERE rr.rfq_id=r.id ORDER BY rr.recipient_index
		)),''),r.privacy_state,COALESCE(r.privacy_processed_at,''),r.created_at,r.updated_at,
		i.item_index,i.kind,COALESCE(i.product_id,''),i.requested,i.raw_query,i.quantity,i.notes,COALESCE(i.public_snapshot_json,'')
		FROM rfqs r JOIN rfq_items i ON i.rfq_id=r.id
		ORDER BY r.created_at,r.id,i.item_index`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var row RFQExportRow
		if err := rows.Scan(&row.RFQID, &row.Status, &row.Revision, &row.Name, &row.Email, &row.Company,
			&row.Phone, &row.Country, &row.GeneralMessage, &row.Recipients, &row.PrivacyState, &row.PrivacyAt,
			&row.CreatedAt, &row.UpdatedAt,
			&row.ItemIndex, &row.ItemKind, &row.ProductID, &row.Requested, &row.RawQuery, &row.Quantity,
			&row.Notes, &row.PublicSnapshot); err != nil {
			rows.Close()
			return err
		}
		if err := visit(row); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordExportAudit(ctx context.Context, actorID, kind, format string, rows int) error {
	actorID = strings.TrimSpace(actorID)
	kind = strings.TrimSpace(kind)
	format = strings.TrimSpace(format)
	if actorID == "" || rows < 0 || (kind != "product" && kind != "rfq") || (format != "xlsx" && format != "csv") {
		return ErrPermissionDenied
	}
	capability := identity.CapabilityCatalogExport
	if kind == "rfq" {
		capability = identity.CapabilityRFQExport
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, capability); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, kind+".export_generated", kind+"_export", format, map[string]any{
			"format": format, "rows": rows, "contains_personal_data": kind == "rfq",
		})
	})
}
