package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/site"
)

func (s *Store) ResetPublicationClaims(ctx context.Context) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='pending',updated_at=?
			WHERE status='processing'`, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) ClaimPublicationIntent(ctx context.Context) (publishing.Intent, bool, error) {
	var intent publishing.Intent
	found := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT id,entity_id,desired_revision,cause FROM publication_intents
			WHERE entity_type='product' AND status='pending' ORDER BY created_at,id LIMIT 1`).
			Scan(&intent.ID, &intent.ProductID, &intent.DesiredRevision, &intent.Cause)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='processing',updated_at=?
			WHERE id=? AND status='pending'`, time.Now().UTC().Format(time.RFC3339Nano), intent.ID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		found = changed == 1
		return nil
	})
	return intent, found, err
}

func (s *Store) PublicationSource(ctx context.Context, productID string) (publishing.Source, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return publishing.Source{}, err
	}
	defer tx.Rollback()
	product, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, productID))
	if err != nil {
		return publishing.Source{}, err
	}
	var source publishing.Source
	source.Product = product
	var routeConfig publishing.SiteRouteConfig
	if err := tx.QueryRowContext(ctx, `SELECT active_epoch,product_prefix,url_pattern FROM public_site_state WHERE singleton=1`).
		Scan(&source.SiteEpoch, &routeConfig.ProductPrefix, &routeConfig.Pattern); err != nil {
		return publishing.Source{}, err
	}
	var supportedLocalesJSON string
	if err := tx.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json,content_multilingual_enabled FROM site_settings WHERE singleton=1`).Scan(&source.Language, &supportedLocalesJSON, &source.ContentMultilingualEnabled); err != nil {
		return publishing.Source{}, err
	}
	if err := json.Unmarshal([]byte(supportedLocalesJSON), &source.SupportedLocales); err != nil {
		return publishing.Source{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT source_locale FROM product_content_metadata WHERE product_id=?`, productID).Scan(&source.SourceLocale); err != nil {
		return publishing.Source{}, err
	}
	translationRows, err := tx.QueryContext(ctx, `SELECT locale,name,description,features,specification,revision,COALESCE(updated_by,''),updated_at FROM product_translations WHERE product_id=? ORDER BY locale`, productID)
	if err != nil {
		return publishing.Source{}, err
	}
	for translationRows.Next() {
		var item catalog.ProductTranslation
		if err := translationRows.Scan(&item.Locale, &item.Name, &item.Description, &item.Features, &item.Specification, &item.Revision, &item.UpdatedBy, &item.UpdatedAt); err != nil {
			translationRows.Close()
			return publishing.Source{}, err
		}
		source.Translations = append(source.Translations, item)
	}
	if err := translationRows.Close(); err != nil {
		return publishing.Source{}, err
	}
	if err := translationRows.Err(); err != nil {
		return publishing.Source{}, err
	}
	source.Site, _, err = websiteConfigurationAtEpoch(ctx, tx, source.SiteEpoch)
	if err != nil {
		return publishing.Source{}, err
	}
	source.Category, source.CategoryPath, source.CategoryTrail, err = publicCategoryTrailQuery(ctx, tx, product.CategoryID)
	if err != nil {
		return publishing.Source{}, err
	}
	if product.ManufacturerID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.ManufacturerID).Scan(&source.ManufacturerSlug); err != nil {
			return publishing.Source{}, err
		}
	}
	if product.BrandID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.BrandID).Scan(&source.BrandSlug); err != nil {
			return publishing.Source{}, err
		}
	}
	var routeReason string
	source.Route, routeReason = resolvedProductRoute(source, routeConfig)
	if routeReason != "" {
		return publishing.Source{}, errors.New(routeReason)
	}
	specRows, err := tx.QueryContext(ctx, `SELECT sd.id,sd.name,pv.raw_value,sd.preferred_unit,pv.source_locale
		FROM product_spec_values pv JOIN spec_definitions sd ON sd.id=pv.spec_id
		WHERE pv.product_id=? AND pv.active=1 AND sd.status='active'
		ORDER BY sd.name,sd.id,pv.source_locale`, productID)
	if err != nil {
		return publishing.Source{}, err
	}
	for specRows.Next() {
		var spec publishing.SourceSpec
		if err := specRows.Scan(&spec.ID, &spec.Name, &spec.RawValue, &spec.PreferredUnit, &spec.Language); err != nil {
			specRows.Close()
			return publishing.Source{}, err
		}
		source.Specs = append(source.Specs, spec)
	}
	if err := specRows.Close(); err != nil {
		return publishing.Source{}, err
	}
	if err := specRows.Err(); err != nil {
		return publishing.Source{}, err
	}
	documentRows, err := tx.QueryContext(ctx, `SELECT pd.id,pd.label,dt.name,COALESCE(pd.asset_id,''),
		COALESCE(pd.external_url,''),pd.language
		FROM product_documents pd JOIN dictionary_entries dt ON dt.id=pd.document_type_id
		WHERE pd.product_id=? ORDER BY pd.sort_order,pd.id`, productID)
	if err != nil {
		return publishing.Source{}, err
	}
	defer documentRows.Close()
	for documentRows.Next() {
		var document publishing.SourceDocument
		if err := documentRows.Scan(&document.ID, &document.Label, &document.Type, &document.AssetID, &document.URL, &document.Language); err != nil {
			return publishing.Source{}, err
		}
		source.Documents = append(source.Documents, document)
	}
	if err := documentRows.Close(); err != nil {
		return publishing.Source{}, err
	}
	if err := documentRows.Err(); err != nil {
		return publishing.Source{}, err
	}
	imageRows, err := tx.QueryContext(ctx, `SELECT id,COALESCE(asset_id,''),COALESCE(external_url,''),alt_text,sort_order,is_primary
		FROM product_images WHERE product_id=? ORDER BY is_primary DESC,sort_order,id`, productID)
	if err != nil {
		return publishing.Source{}, err
	}
	for imageRows.Next() {
		var image publishing.SourceImage
		if err := imageRows.Scan(&image.ID, &image.AssetID, &image.ExternalURL, &image.AltText, &image.SortOrder, &image.Primary); err != nil {
			imageRows.Close()
			return publishing.Source{}, err
		}
		source.Images = append(source.Images, image)
	}
	if err := imageRows.Close(); err != nil {
		return publishing.Source{}, err
	}
	if err := imageRows.Err(); err != nil {
		return publishing.Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return publishing.Source{}, err
	}
	return source, nil
}

type publicationRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func publicCategoryIdentityQuery(ctx context.Context, queryer publicationRowQueryer, categoryID string) (string, string, error) {
	current := categoryID
	seen := make(map[string]struct{})
	var displayName string
	var segments []string
	for current != "" {
		if _, exists := seen[current]; exists || len(seen) >= 256 {
			return "", "", catalog.ErrCategoryCycle
		}
		seen[current] = struct{}{}
		var parentID sql.NullString
		var systemKey sql.NullString
		var name, slug string
		if err := queryer.QueryRowContext(ctx, `SELECT parent_id,system_key,name,slug FROM categories WHERE id=?`, current).
			Scan(&parentID, &systemKey, &name, &slug); err != nil {
			return "", "", err
		}
		if displayName == "" {
			displayName = name
			if systemKey.Valid && systemKey.String == "root" {
				return displayName, "", nil
			}
		}
		if !systemKey.Valid || systemKey.String == "uncategorized" {
			segments = append(segments, slug)
		}
		if !parentID.Valid {
			break
		}
		current = parentID.String
	}
	for left, right := 0, len(segments)-1; left < right; left, right = left+1, right-1 {
		segments[left], segments[right] = segments[right], segments[left]
	}
	return displayName, strings.Join(segments, "/"), nil
}

type publicCategoryTrailNode struct {
	id        string
	parentID  string
	name      string
	slug      string
	systemKey string
}

func publicCategoryTrailQuery(ctx context.Context, queryer publicationRowQueryer, categoryID string) (string, string, []publishing.SourceCategory, error) {
	current := categoryID
	seen := make(map[string]struct{})
	nodes := make([]publicCategoryTrailNode, 0, 8)
	for current != "" {
		if _, exists := seen[current]; exists || len(seen) >= 256 {
			return "", "", nil, catalog.ErrCategoryCycle
		}
		seen[current] = struct{}{}
		var parentID, systemKey sql.NullString
		node := publicCategoryTrailNode{id: current}
		if err := queryer.QueryRowContext(ctx, `SELECT parent_id,system_key,name,slug FROM categories WHERE id=?`, current).
			Scan(&parentID, &systemKey, &node.name, &node.slug); err != nil {
			return "", "", nil, err
		}
		if parentID.Valid {
			node.parentID = parentID.String
		}
		if systemKey.Valid {
			node.systemKey = systemKey.String
		}
		nodes = append(nodes, node)
		current = node.parentID
	}
	if len(nodes) == 0 {
		return "", "", nil, nil
	}
	leafName := nodes[0].name
	for left, right := 0, len(nodes)-1; left < right; left, right = left+1, right-1 {
		nodes[left], nodes[right] = nodes[right], nodes[left]
	}
	segments := make([]string, 0, len(nodes))
	trail := make([]publishing.SourceCategory, 0, len(nodes))
	for _, node := range nodes {
		if node.systemKey == "root" {
			continue
		}
		if node.systemKey == "" || node.systemKey == "uncategorized" {
			segments = append(segments, node.slug)
		}
		path := strings.Join(segments, "/")
		if path != "" {
			trail = append(trail, publishing.SourceCategory{ID: node.id, Name: node.name, Path: path})
		}
	}
	return leafName, strings.Join(segments, "/"), trail, nil
}

func resolvedProductRoute(source publishing.Source, config publishing.SiteRouteConfig) (string, string) {
	if source.Product.CustomPath != "" {
		if !catalog.ValidCustomPath(source.Product.CustomPath) {
			return "", "invalid custom product path"
		}
		return source.Product.CustomPath, ""
	}
	prefix := strings.TrimRight(strings.TrimSpace(config.ProductPrefix), "/")
	if !validProductPrefix(prefix) {
		return "", "invalid product prefix"
	}
	var namespace string
	switch config.Pattern {
	case publishing.RouteCompact:
	case publishing.RouteManufacturer:
		namespace = source.ManufacturerSlug
		if namespace == "" {
			return "", "manufacturer namespace is required"
		}
	case publishing.RouteBrand:
		namespace = source.BrandSlug
		if namespace == "" {
			return "", "brand namespace is required"
		}
	case publishing.RouteCategory:
		namespace = source.CategoryPath
		if namespace == "" {
			return "", "category namespace is required"
		}
	default:
		return "", "unsupported URL pattern"
	}
	if namespace != "" {
		return prefix + "/" + namespace + "/" + source.Product.Slug, ""
	}
	return prefix + "/" + source.Product.Slug, ""
}

func validProductPrefix(prefix string) bool {
	if len(prefix) < 2 || len(prefix) > 64 || prefix[0] != '/' || strings.Count(prefix, "/") != 1 {
		return false
	}
	segment := strings.TrimPrefix(prefix, "/")
	if !catalog.ValidSlug(segment) {
		return false
	}
	reserved := map[string]struct{}{
		"admin": {}, "api": {}, "assets": {}, "static": {}, "health": {}, "search": {}, "rfq": {},
		"categories": {}, "manufacturers": {}, "brands": {}, "applications": {}, "set-password": {}, "sitemap.xml": {}, "site.css": {},
	}
	_, blocked := reserved[strings.ToLower(segment)]
	return !blocked
}

func (s *Store) PublicSiteRouteConfig(ctx context.Context) (int64, publishing.SiteRouteConfig, error) {
	var epoch int64
	var config publishing.SiteRouteConfig
	err := s.db.QueryRowContext(ctx, `SELECT active_epoch,product_prefix,url_pattern FROM public_site_state WHERE singleton=1`).
		Scan(&epoch, &config.ProductPrefix, &config.Pattern)
	return epoch, config, err
}

func (s *Store) PrepareSiteRoutes(ctx context.Context, config publishing.SiteRouteConfig) (publishing.SiteRoutePreview, []publishing.Source, error) {
	preview := publishing.SiteRoutePreview{Missing: make([]publishing.SiteRouteIssue, 0), Conflicts: make([]publishing.SiteRouteIssue, 0)}
	if !validProductPrefix(strings.TrimRight(strings.TrimSpace(config.ProductPrefix), "/")) {
		return preview, nil, publishing.ErrSiteRouteInvalid
	}
	if config.Pattern != publishing.RouteCompact && config.Pattern != publishing.RouteManufacturer && config.Pattern != publishing.RouteBrand && config.Pattern != publishing.RouteCategory {
		return preview, nil, publishing.ErrSiteRouteInvalid
	}
	if err := s.db.QueryRowContext(ctx, `SELECT active_epoch FROM public_site_state WHERE singleton=1`).Scan(&preview.CurrentEpoch); err != nil {
		return preview, nil, err
	}
	var working site.Configuration
	var workingJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT revision,config_json FROM website_working WHERE singleton=1`).Scan(&preview.WorkingRevision, &workingJSON); err != nil {
		return preview, nil, err
	}
	if err := json.Unmarshal([]byte(workingJSON), &working); err != nil {
		return preview, nil, err
	}
	if err := working.Prepare(); err != nil {
		return preview, nil, err
	}
	preview.CandidateEpoch = preview.CurrentEpoch + 1
	rows, err := s.db.QueryContext(ctx, `SELECT `+productColumns+` FROM products WHERE record_state='current' AND status='published' ORDER BY id`)
	if err != nil {
		return preview, nil, err
	}
	var products []catalog.Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			rows.Close()
			return preview, nil, err
		}
		products = append(products, product)
	}
	if err := rows.Close(); err != nil {
		return preview, nil, err
	}
	if err := rows.Err(); err != nil {
		return preview, nil, err
	}
	preview.Affected = len(products)
	sources := make([]publishing.Source, 0, len(products))
	owners := make(map[string]int)
	for _, product := range products {
		source, err := s.PublicationSource(ctx, product.ID)
		if err != nil {
			return preview, nil, err
		}
		source.SiteEpoch = preview.CandidateEpoch
		source.Site = working
		route, reason := resolvedProductRoute(source, config)
		if reason != "" {
			preview.Missing = append(preview.Missing, publishing.SiteRouteIssue{ProductID: product.ID, PartNumber: product.PartNumber, Reason: reason})
			continue
		}
		source.Route = route
		if first, exists := owners[route]; exists {
			preview.Conflicts = append(preview.Conflicts,
				publishing.SiteRouteIssue{ProductID: sources[first].Product.ID, PartNumber: sources[first].Product.PartNumber, Route: route, Reason: "duplicate target route"},
				publishing.SiteRouteIssue{ProductID: product.ID, PartNumber: product.PartNumber, Route: route, Reason: "duplicate target route"})
		} else {
			owners[route] = len(sources)
		}
		sources = append(sources, source)
	}
	return preview, sources, nil
}

func (s *Store) ActivateSiteRoutes(ctx context.Context, request publishing.SiteRouteRequest, activations []publishing.ActivePublication, coordinator publishing.VisibilityCoordinator) error {
	type preparedActivation struct {
		activation publishing.ActivePublication
		viewJSON   string
	}
	return s.withVisibilityTransaction(ctx, coordinator, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, request.ActorID, identity.CapabilityCatalogPublish); err != nil {
			return err
		}
		if err := requireActorCapability(ctx, tx, request.ActorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		var currentEpoch int64
		if err := tx.QueryRowContext(ctx, `SELECT active_epoch FROM public_site_state WHERE singleton=1`).Scan(&currentEpoch); err != nil {
			return err
		}
		if currentEpoch != request.ExpectedEpoch {
			return publishing.ErrSiteRouteInvalid
		}
		var workingRevision int64
		var workingJSON string
		if err := tx.QueryRowContext(ctx, `SELECT revision,config_json FROM website_working WHERE singleton=1`).Scan(&workingRevision, &workingJSON); err != nil {
			return err
		}
		if workingRevision != request.ExpectedWorkingRevision {
			return publishing.ErrSiteRouteInvalid
		}
		var working site.Configuration
		if err := json.Unmarshal([]byte(workingJSON), &working); err != nil {
			return err
		}
		if err := working.Prepare(); err != nil {
			return err
		}
		normalizedWorking, err := json.Marshal(working)
		if err != nil {
			return err
		}
		candidateEpoch := currentEpoch + 1
		var publishedCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE record_state='current' AND status='published'`).Scan(&publishedCount); err != nil {
			return err
		}
		if publishedCount != len(activations) {
			return publishing.ErrSiteRouteInvalid
		}
		seenProducts := make(map[string]struct{}, len(activations))
		seenRoutes := make(map[string]struct{}, len(activations))
		prepared := make([]preparedActivation, 0, len(activations))
		for _, activation := range activations {
			activationSite, err := json.Marshal(activation.View.Site)
			if err != nil || string(activationSite) != string(normalizedWorking) {
				return publishing.ErrSiteRouteInvalid
			}
			if _, exists := seenProducts[activation.ProductID]; exists {
				return publishing.ErrSiteRouteInvalid
			}
			if _, exists := seenRoutes[activation.Route]; exists {
				return publishing.ErrSiteRouteInvalid
			}
			seenProducts[activation.ProductID] = struct{}{}
			seenRoutes[activation.Route] = struct{}{}
			product, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, activation.ProductID))
			if err != nil {
				return err
			}
			if product.RecordState != catalog.RecordCurrent || product.Status != catalog.Published || product.Revision != activation.SourceRevision || activation.SiteEpoch != candidateEpoch {
				return publishing.ErrSiteRouteInvalid
			}
			source := publishing.Source{Product: product, SiteEpoch: candidateEpoch}
			source.Category, source.CategoryPath, err = publicCategoryIdentityQuery(ctx, tx, product.CategoryID)
			if err != nil {
				return err
			}
			if product.ManufacturerID != "" {
				if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.ManufacturerID).Scan(&source.ManufacturerSlug); err != nil {
					return err
				}
			}
			if product.BrandID != "" {
				if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.BrandID).Scan(&source.BrandSlug); err != nil {
					return err
				}
			}
			expectedRoute, reason := resolvedProductRoute(source, request.Config)
			if reason != "" || expectedRoute != activation.Route || activation.View.ID != product.ID || activation.View.Revision != product.Revision || activation.View.SiteEpoch != candidateEpoch {
				return publishing.ErrSiteRouteInvalid
			}
			viewJSON, err := json.Marshal(activation.View)
			if err != nil {
				return err
			}
			prepared = append(prepared, preparedActivation{activation: activation, viewJSON: string(viewJSON)})
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		oldRoutes := make(map[string]string)
		rows, err := tx.QueryContext(ctx, `SELECT product_id,route FROM public_activations`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var productID, route string
			if err := rows.Scan(&productID, &route); err != nil {
				rows.Close()
				return err
			}
			oldRoutes[productID] = route
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO website_versions(
			source_working_revision,site_epoch,config_json,created_by,created_at
		) VALUES(?,?,?,?,?)`, workingRevision, candidateEpoch, string(normalizedWorking), request.ActorID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE public_site_state SET active_epoch=?,product_prefix=?,url_pattern=?,updated_at=? WHERE singleton=1 AND active_epoch=?`,
			candidateEpoch, strings.TrimRight(request.Config.ProductPrefix, "/"), request.Config.Pattern, now, currentEpoch); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM public_activations`); err != nil {
			return err
		}
		for _, item := range prepared {
			activation := item.activation
			if oldRoute := oldRoutes[activation.ProductID]; oldRoute != "" && oldRoute != activation.Route {
				if err := saveRouteHistory(ctx, tx, activation.ProductID, oldRoute, activation.Route, "redirect", candidateEpoch, now); err != nil {
					return err
				}
			}
			var generation int64
			err := tx.QueryRowContext(ctx, `SELECT generation FROM public_visibility WHERE product_id=?`, activation.ProductID).Scan(&generation)
			if errors.Is(err, sql.ErrNoRows) {
				generation = 1
			} else if err != nil {
				return err
			} else {
				generation++
			}
			activation.VisibilityGeneration = generation
			if _, err := tx.ExecContext(ctx, `INSERT INTO public_visibility(product_id,generation,visible,updated_at) VALUES(?,?,1,?) ON CONFLICT(product_id) DO UPDATE SET generation=excluded.generation,visible=1,updated_at=excluded.updated_at`, activation.ProductID, generation, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO public_activations(product_id,source_revision,public_revision,site_epoch,route,artifact_id,manifest_hash,visibility_generation,view_json,activated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
				activation.ProductID, activation.SourceRevision, activation.PublicRevision, activation.SiteEpoch, activation.Route,
				activation.ArtifactID, activation.ManifestHash, generation, item.viewJSON, activation.ActivatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if err := upsertPublicSearchProjection(ctx, tx, activation, generation, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='superseded',updated_at=? WHERE entity_type='product' AND entity_id=? AND desired_revision<=? AND status IN ('pending','processing')`, now, activation.ProductID, activation.SourceRevision); err != nil {
				return err
			}
			if err := markPublicationDependenciesDirty(ctx, tx, activation.ProductID, activation.SourceRevision, "site.routes.activated", now); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, request.ActorID, "site.routes_published", "site", "public", map[string]any{
			"from_epoch": currentEpoch, "to_epoch": candidateEpoch, "product_prefix": request.Config.ProductPrefix,
			"url_pattern": request.Config.Pattern, "affected": len(prepared),
		})
	})
}

func (s *Store) ActivatePublication(ctx context.Context, intent publishing.Intent, activation publishing.ActivePublication, coordinator publishing.VisibilityCoordinator) (bool, error) {
	viewJSON, err := json.Marshal(activation.View)
	if err != nil {
		return false, err
	}
	activated := false
	err = s.withVisibilityTransaction(ctx, coordinator, func(tx *sql.Tx) error {
		var revision int64
		var recordState catalog.RecordState
		var status catalog.Status
		if err := tx.QueryRowContext(ctx, `SELECT revision,record_state,status FROM products WHERE id=?`, intent.ProductID).
			Scan(&revision, &recordState, &status); err != nil {
			return err
		}
		var intentState string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM publication_intents WHERE id=? AND entity_id=? AND desired_revision=?`,
			intent.ID, intent.ProductID, intent.DesiredRevision).Scan(&intentState); err != nil {
			return err
		}
		if intentState != "processing" || revision != intent.DesiredRevision || recordState != catalog.RecordCurrent || status != catalog.Published {
			_, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='superseded',updated_at=? WHERE id=? AND status='processing'`,
				time.Now().UTC().Format(time.RFC3339Nano), intent.ID)
			return err
		}
		if activation.ProductID != intent.ProductID || activation.SourceRevision != revision || activation.PublicRevision != revision || activation.Route == "" {
			return errors.New("activation does not match current source")
		}
		var generation int64
		var currentlyVisible int
		err := tx.QueryRowContext(ctx, `SELECT generation,visible FROM public_visibility WHERE product_id=?`, intent.ProductID).
			Scan(&generation, &currentlyVisible)
		if errors.Is(err, sql.ErrNoRows) {
			generation = 1
		} else if err != nil {
			return err
		} else if currentlyVisible == 0 {
			generation++
		}
		activation.VisibilityGeneration = generation
		now := activation.ActivatedAt.UTC().Format(time.RFC3339Nano)
		var oldRoute string
		err = tx.QueryRowContext(ctx, `SELECT route FROM public_activations WHERE product_id=?`, intent.ProductID).Scan(&oldRoute)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if oldRoute != "" && oldRoute != activation.Route {
			if err := saveRouteHistory(ctx, tx, intent.ProductID, oldRoute, activation.Route, "redirect", activation.SiteEpoch, now); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_visibility(product_id,generation,visible,updated_at)
			VALUES(?,?,1,?) ON CONFLICT(product_id) DO UPDATE SET generation=excluded.generation,visible=1,updated_at=excluded.updated_at`,
			intent.ProductID, generation, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_activations(
			product_id,source_revision,public_revision,site_epoch,route,artifact_id,manifest_hash,visibility_generation,view_json,activated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(product_id) DO UPDATE SET
			source_revision=excluded.source_revision,public_revision=excluded.public_revision,site_epoch=excluded.site_epoch,
			route=excluded.route,artifact_id=excluded.artifact_id,manifest_hash=excluded.manifest_hash,
			visibility_generation=excluded.visibility_generation,view_json=excluded.view_json,activated_at=excluded.activated_at`,
			intent.ProductID, revision, revision, activation.SiteEpoch, activation.Route, activation.ArtifactID,
			activation.ManifestHash, generation, string(viewJSON), now); err != nil {
			return err
		}
		if err := upsertPublicSearchProjection(ctx, tx, activation, generation, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='completed',updated_at=? WHERE id=? AND status='processing'`, now, intent.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='superseded',updated_at=?
			WHERE entity_type='product' AND entity_id=? AND id<>? AND status IN ('pending','processing') AND desired_revision<=?`,
			now, intent.ProductID, intent.ID, revision); err != nil {
			return err
		}
		if err := markPublicationDependenciesDirty(ctx, tx, intent.ProductID, revision, "product.activated", now); err != nil {
			return err
		}
		activated = true
		return nil
	})
	return activated, err
}

func (s *Store) CompletePublicationIntent(ctx context.Context, intentID, state string) error {
	if state != "completed" && state != "superseded" {
		return errors.New("invalid publication intent completion state")
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status=?,updated_at=?
			WHERE id=? AND status IN ('pending','processing')`, state, time.Now().UTC().Format(time.RFC3339Nano), intentID)
		return err
	})
}

func (s *Store) FailPublicationIntent(ctx context.Context, intentID, message string) error {
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE publication_intents SET status='failed',error_message=?,updated_at=?
			WHERE id=? AND status='processing'`, message, time.Now().UTC().Format(time.RFC3339Nano), intentID)
		return err
	})
}

func (s *Store) ActivePublications(ctx context.Context) ([]publishing.ActivePublication, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.product_id,a.source_revision,a.public_revision,a.site_epoch,a.route,
		a.artifact_id,a.manifest_hash,a.visibility_generation,a.view_json,a.activated_at
		FROM public_activations a JOIN public_visibility v ON v.product_id=a.product_id
		JOIN products p ON p.id=a.product_id
		WHERE v.visible=1 AND v.generation=a.visibility_generation AND p.record_state='current' AND p.status='published'
		ORDER BY a.route,a.product_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var activations []publishing.ActivePublication
	for rows.Next() {
		var activation publishing.ActivePublication
		var viewJSON, activatedAt string
		if err := rows.Scan(&activation.ProductID, &activation.SourceRevision, &activation.PublicRevision, &activation.SiteEpoch,
			&activation.Route, &activation.ArtifactID, &activation.ManifestHash, &activation.VisibilityGeneration,
			&viewJSON, &activatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(viewJSON), &activation.View); err != nil {
			return nil, err
		}
		activation.ActivatedAt, err = time.Parse(time.RFC3339Nano, activatedAt)
		if err != nil {
			return nil, err
		}
		activations = append(activations, activation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return s.attachActiveTranslations(ctx, activations)
}

func (s *Store) attachActiveTranslations(ctx context.Context, activations []publishing.ActivePublication) ([]publishing.ActivePublication, error) {
	var enabled bool
	var supportedJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT content_multilingual_enabled,supported_locales_json FROM site_settings WHERE singleton=1`).Scan(&enabled, &supportedJSON); err != nil {
		return nil, err
	}
	if !enabled {
		return activations, nil
	}
	for index := range activations {
		rows, err := s.db.QueryContext(ctx, `SELECT locale,name,description,features,specification FROM product_translations WHERE product_id=? ORDER BY locale`, activations[index].ProductID)
		if err != nil {
			return nil, err
		}
		activations[index].View.Localizations = make(map[string]publishing.LocalizedContent)
		for rows.Next() {
			var locale string
			var item publishing.LocalizedContent
			if err := rows.Scan(&locale, &item.Name, &item.Description, &item.Features, &item.Specification); err != nil {
				rows.Close()
				return nil, err
			}
			if localeInJSON(supportedJSON, locale) {
				activations[index].View.Localizations[locale] = item
			}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return activations, nil
}

func (s *Store) SearchPublications(ctx context.Context, foldedQuery, projectionVersion string) ([]publishing.ActivePublication, error) {
	escaped := catalog.EscapeLike(foldedQuery)
	prefix := escaped + "%"
	contains := "%" + escaped + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT a.product_id,a.source_revision,a.public_revision,a.site_epoch,a.route,
		a.artifact_id,a.manifest_hash,a.visibility_generation,a.view_json,a.activated_at
		FROM public_search_projection sp
		JOIN public_activations a ON a.product_id=sp.product_id AND a.source_revision=sp.source_revision AND a.site_epoch=sp.site_epoch
		JOIN public_visibility v ON v.product_id=a.product_id AND v.visible=1 AND v.generation=a.visibility_generation AND v.generation=sp.visibility_generation
		JOIN products p ON p.id=a.product_id AND p.record_state='current' AND p.status='published'
		WHERE sp.projection_version=? AND sp.all_folded LIKE ? ESCAPE '\'
		ORDER BY CASE
			WHEN sp.part_number_folded=? THEN 0
			WHEN sp.part_number_folded LIKE ? ESCAPE '\' THEN 1
			WHEN sp.product_name_folded LIKE ? ESCAPE '\' OR sp.manufacturer_folded LIKE ? ESCAPE '\' OR sp.brand_folded LIKE ? ESCAPE '\' THEN 2
			ELSE 3 END,
			a.product_id
		LIMIT 2000`, projectionVersion, contains, foldedQuery, prefix, prefix, prefix, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activations := make([]publishing.ActivePublication, 0)
	for rows.Next() {
		var activation publishing.ActivePublication
		var viewJSON, activatedAt string
		if err := rows.Scan(&activation.ProductID, &activation.SourceRevision, &activation.PublicRevision, &activation.SiteEpoch,
			&activation.Route, &activation.ArtifactID, &activation.ManifestHash, &activation.VisibilityGeneration, &viewJSON, &activatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(viewJSON), &activation.View); err != nil {
			return nil, err
		}
		activation.ActivatedAt, err = time.Parse(time.RFC3339Nano, activatedAt)
		if err != nil {
			return nil, err
		}
		activations = append(activations, activation)
	}
	return activations, rows.Err()
}

func (s *Store) RebuildSearchPublications(ctx context.Context, activations []publishing.ActivePublication) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM public_search_projection`); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, activation := range activations {
			if err := upsertPublicSearchProjection(ctx, tx, activation, activation.VisibilityGeneration, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ConvergePublicationDependencies(ctx context.Context, productID string, revision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM publication_dirty WHERE entity_id=? AND desired_revision<=?`, productID, revision)
		return err
	})
}

func (s *Store) ActiveRouteHistory(ctx context.Context) ([]publishing.RouteHistory, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT route,product_id,target_route,state,site_epoch FROM public_route_history ORDER BY route`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := make([]publishing.RouteHistory, 0)
	for rows.Next() {
		var item publishing.RouteHistory
		if err := rows.Scan(&item.Route, &item.ProductID, &item.TargetRoute, &item.State, &item.SiteEpoch); err != nil {
			return nil, err
		}
		history = append(history, item)
	}
	return history, rows.Err()
}

func (s *Store) PublicAssets(ctx context.Context, ids []string) ([]publishing.PublicAsset, error) {
	assets := make([]publishing.PublicAsset, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, publishing.ErrArtifactInvalid
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		var asset publishing.PublicAsset
		if err := s.db.QueryRowContext(ctx, `SELECT id,storage_path,mime_type,size_bytes,checksum FROM assets WHERE id=?`, id).
			Scan(&asset.ID, &asset.StoragePath, &asset.MIMEType, &asset.SizeBytes, &asset.Checksum); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, publishing.ErrArtifactInvalid
			}
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (s *Store) QueuePublicationRepair(ctx context.Context, activation publishing.ActivePublication, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	intentID, err := randomID("pub")
	if err != nil {
		return err
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var revision int64
		var recordState catalog.RecordState
		var status catalog.Status
		if err := tx.QueryRowContext(ctx, `SELECT revision,record_state,status FROM products WHERE id=?`, activation.ProductID).
			Scan(&revision, &recordState, &status); err != nil {
			return err
		}
		if revision != activation.SourceRevision || recordState != catalog.RecordCurrent || status != catalog.Published {
			return nil
		}
		var activeArtifact string
		if err := tx.QueryRowContext(ctx, `SELECT artifact_id FROM public_activations WHERE product_id=? AND source_revision=?`, activation.ProductID, activation.SourceRevision).
			Scan(&activeArtifact); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if activeArtifact != activation.ArtifactID {
			return nil
		}
		var existing int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_intents WHERE entity_type='product' AND entity_id=? AND desired_revision=? AND status IN ('pending','processing')`, activation.ProductID, activation.SourceRevision).
			Scan(&existing); err != nil {
			return err
		}
		if existing != 0 {
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO publication_intents(id,entity_type,entity_id,desired_revision,cause,status,created_at,updated_at) VALUES(?,'product',?,?,?,'pending',?,?)`,
			intentID, activation.ProductID, activation.SourceRevision, "repair: "+reason, now, now)
		return err
	})
}

func (s *Store) RevokeProduct(ctx context.Context, request publishing.RevokeRequest, coordinator publishing.VisibilityCoordinator) error {
	return s.withVisibilityTransaction(ctx, coordinator, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, request.ActorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if err := requireActorCapability(ctx, tx, request.ActorID, identity.CapabilityCatalogPublish); err != nil {
			return err
		}
		var recordState catalog.RecordState
		var status catalog.Status
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT record_state,status,revision FROM products WHERE id=?`, request.ProductID).
			Scan(&recordState, &status, &revision); err != nil {
			return err
		}
		if recordState == catalog.RecordArchived {
			return catalog.ErrArchivedProduct
		}
		if revision != request.ExpectedRevision {
			return catalog.ErrRevisionConflict
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		newRevision := revision + 1
		cause := "product.hidden"
		if request.Archive {
			cause = "product.archived"
			if _, err := tx.ExecContext(ctx, `UPDATE products SET record_state='archived',status='hidden',revision=?,updated_by=?,updated_at=?
				WHERE id=? AND revision=? AND record_state='current'`, newRevision, request.ActorID, now, request.ProductID, revision); err != nil {
				return err
			}
		} else {
			if status == catalog.Hidden {
				newRevision = revision
			} else if _, err := tx.ExecContext(ctx, `UPDATE products SET status='hidden',revision=?,updated_by=?,updated_at=?
				WHERE id=? AND revision=? AND record_state='current'`, newRevision, request.ActorID, now, request.ProductID, revision); err != nil {
				return err
			}
		}
		var generation int64
		err := tx.QueryRowContext(ctx, `SELECT generation FROM public_visibility WHERE product_id=?`, request.ProductID).Scan(&generation)
		if errors.Is(err, sql.ErrNoRows) {
			generation = 1
		} else if err != nil {
			return err
		} else {
			generation++
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_visibility(product_id,generation,visible,updated_at)
			VALUES(?,?,0,?) ON CONFLICT(product_id) DO UPDATE SET generation=excluded.generation,visible=0,updated_at=excluded.updated_at`,
			request.ProductID, generation, now); err != nil {
			return err
		}
		var revokedRoute string
		err = tx.QueryRowContext(ctx, `SELECT route FROM public_activations WHERE product_id=?`, request.ProductID).Scan(&revokedRoute)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if request.Archive && revokedRoute != "" {
			if err := saveRouteHistory(ctx, tx, request.ProductID, revokedRoute, "", "gone", 0, now); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM public_activations WHERE product_id=?`, request.ProductID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM public_search_projection WHERE product_id=?`, request.ProductID); err != nil {
			return err
		}
		if newRevision != revision {
			if err := appendAudit(ctx, tx, request.ActorID, cause, "product", request.ProductID, map[string]any{
				"from_revision": revision, "to_revision": newRevision,
			}); err != nil {
				return err
			}
			if err := appendPublicationIntent(ctx, tx, request.ProductID, newRevision, cause, now); err != nil {
				return err
			}
		}
		return markPublicationDependenciesDirty(ctx, tx, request.ProductID, newRevision, cause, now)
	})
}

func upsertPublicSearchProjection(ctx context.Context, tx *sql.Tx, activation publishing.ActivePublication, generation int64, now string) error {
	view := activation.View
	partNumber := catalog.FoldSearch(view.PartNumber)
	productName := catalog.FoldSearch(view.Name)
	manufacturer := catalog.FoldSearch(view.Manufacturer)
	brand := catalog.FoldSearch(view.Brand)
	category := catalog.FoldSearch(view.Category)
	searchValues := []string{view.PartNumber, view.Name, view.Manufacturer, view.Brand, view.Category, view.Lifecycle, publishingApplicationNames(view.Applications)}
	var enabled bool
	var supportedJSON string
	if err := tx.QueryRowContext(ctx, `SELECT content_multilingual_enabled,supported_locales_json FROM site_settings WHERE singleton=1`).Scan(&enabled, &supportedJSON); err != nil {
		return err
	}
	if enabled {
		rows, err := tx.QueryContext(ctx, `SELECT locale,name,description,features,specification FROM product_translations WHERE product_id=?`, activation.ProductID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var locale string
			var values [4]string
			if err := rows.Scan(&locale, &values[0], &values[1], &values[2], &values[3]); err != nil {
				rows.Close()
				return err
			}
			if localeInJSON(supportedJSON, locale) {
				searchValues = append(searchValues, values[:]...)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	all := catalog.FoldSearch(strings.Join(searchValues, " "))
	_, err := tx.ExecContext(ctx, `INSERT INTO public_search_projection(
		product_id,source_revision,site_epoch,visibility_generation,projection_version,
		part_number_folded,product_name_folded,manufacturer_folded,brand_folded,category_folded,all_folded,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(product_id) DO UPDATE SET
		source_revision=excluded.source_revision,site_epoch=excluded.site_epoch,visibility_generation=excluded.visibility_generation,
		projection_version=excluded.projection_version,part_number_folded=excluded.part_number_folded,
		product_name_folded=excluded.product_name_folded,manufacturer_folded=excluded.manufacturer_folded,
		brand_folded=excluded.brand_folded,category_folded=excluded.category_folded,all_folded=excluded.all_folded,updated_at=excluded.updated_at`,
		activation.ProductID, activation.SourceRevision, activation.SiteEpoch, generation, catalog.SearchProjectionVersion,
		partNumber, productName, manufacturer, brand, category, all, now)
	return err
}

func publishingApplicationNames(applications []publishing.Application) string {
	names := make([]string, 0, len(applications))
	for _, application := range applications {
		names = append(names, application.Name)
	}
	return strings.Join(names, " ")
}

func saveRouteHistory(ctx context.Context, tx *sql.Tx, productID, oldRoute, targetRoute, state string, siteEpoch int64, now string) error {
	for _, suffix := range []string{"", ".json", ".md"} {
		target := ""
		if targetRoute != "" {
			target = targetRoute + suffix
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_route_history(route,product_id,target_route,state,site_epoch,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(route) DO UPDATE SET product_id=excluded.product_id,target_route=excluded.target_route,state=excluded.state,site_epoch=excluded.site_epoch,updated_at=excluded.updated_at`,
			oldRoute+suffix, productID, target, state, siteEpoch, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) withVisibilityTransaction(ctx context.Context, coordinator publishing.VisibilityCoordinator, apply func(*sql.Tx) error) error {
	if coordinator == nil {
		return errors.New("visibility coordinator required")
	}
	select {
	case s.writer <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.writer }()
	coordinator.LockVisibility()
	defer coordinator.UnlockVisibility()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := apply(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := coordinator.InstallVisibility(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("visibility state committed but public index installation failed: %w", err)
	}
	return nil
}

func markPublicationDependenciesDirty(ctx context.Context, tx *sql.Tx, productID string, revision int64, reason, now string) error {
	for _, kind := range []string{"search", "category", "manufacturer", "brand", "application", "lifecycle", "document_type", "sitemap", "manifest"} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO publication_dirty(kind,entity_id,desired_revision,reason,updated_at)
			VALUES(?,?,?,?,?) ON CONFLICT(kind,entity_id) DO UPDATE SET
			desired_revision=excluded.desired_revision,reason=excluded.reason,updated_at=excluded.updated_at`,
			kind, productID, revision, reason, now); err != nil {
			return err
		}
	}
	return nil
}
