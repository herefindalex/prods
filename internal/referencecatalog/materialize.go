package referencecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/site"
	storesqlite "prods/internal/storage/sqlite"
)

const (
	ManifestFilename  = "reference-manifest.json"
	InventoryFilename = "taxonomy-inventory.json"
	FixtureOwnerEmail = "fixture-owner@prods.invalid"
	FixturePassword   = "ReferenceDataset1!"
)

var ErrUnsafeTarget = errors.New("reference dataset target must be a new, dedicated directory")

type CreateResult struct {
	DataRoot     string
	DatabasePath string
	ManifestPath string
	OwnerEmail   string
	Password     string
	Products     int
}

type PublishResult struct {
	DataRoot       string
	Published      int
	ManifestPath   string
	PublicRoot     string
	PublicationLag storesqlite.OperationalHealth
}

func Create(ctx context.Context, dataRoot string, seed int64) (CreateResult, error) {
	root, err := newDataRoot(dataRoot)
	if err != nil {
		return CreateResult{}, err
	}
	dataset, manifest, err := Build(seed)
	if err != nil {
		return CreateResult{}, err
	}
	databasePath := filepath.Join(root, "prods.db")
	store, err := storesqlite.Create(databasePath)
	if err != nil {
		return CreateResult{}, fmt.Errorf("create reference database: %w", err)
	}
	defer store.Close()
	passwordHash, err := identity.HashPassword(FixturePassword)
	if err != nil {
		return CreateResult{}, fmt.Errorf("hash fixture owner password: %w", err)
	}
	locales := []string{"en-US", "zh-TW", "zh-CN", "ja-JP", "ko-KR", "de-DE", "fr-FR", "it-IT", "es-ES", "pt-BR"}
	owner, err := store.CompleteInstallation(ctx, storesqlite.Installation{
		OwnerEmail:       FixtureOwnerEmail,
		OwnerDisplayName: "Reference Fixture Owner",
		PasswordHash:     passwordHash,
		DefaultLocale:    "en-US",
		SupportedLocales: locales,
		TimeZone:         "UTC",
	})
	if err != nil {
		return CreateResult{}, fmt.Errorf("complete fixture installation: %w", err)
	}
	state, err := store.WebsiteState(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("load fixture website state: %w", err)
	}
	_, err = store.SaveWebsiteLocalization(ctx, owner.ID, state.WorkingRevision, site.WebsiteLocalization{
		DefaultLocale: "en-US", EnabledLocales: locales, ContentEditingEnabled: true,
	})
	if err != nil {
		return CreateResult{}, fmt.Errorf("enable fixture content locales: %w", err)
	}
	if err := loadTaxonomy(ctx, store, owner.ID, dataset); err != nil {
		return CreateResult{}, err
	}
	if err := configureListingProfiles(ctx, store, owner.ID); err != nil {
		return CreateResult{}, err
	}
	if err := loadProducts(ctx, store, owner.ID, dataset); err != nil {
		return CreateResult{}, err
	}
	if err := writeReferencePDF(filepath.Join(root, "assets", "reference", "reference-datasheet.pdf")); err != nil {
		return CreateResult{}, err
	}
	manifestPath := filepath.Join(root, ManifestFilename)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return CreateResult{}, err
	}
	if err := writeJSON(filepath.Join(root, InventoryFilename), manifest.Taxonomy); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{DataRoot: root, DatabasePath: databasePath, ManifestPath: manifestPath, OwnerEmail: FixtureOwnerEmail, Password: FixturePassword, Products: len(dataset.Products)}, nil
}

func Publish(ctx context.Context, dataRoot, baseURL string) (PublishResult, error) {
	root, err := existingDataRoot(dataRoot)
	if err != nil {
		return PublishResult{}, err
	}
	manifestPath := filepath.Join(root, ManifestFilename)
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return PublishResult{}, err
	}
	if manifest.DatasetVersion != Version {
		return PublishResult{}, fmt.Errorf("unsupported reference dataset version %q", manifest.DatasetVersion)
	}
	store, err := storesqlite.OpenReady(filepath.Join(root, "prods.db"))
	if err != nil {
		return PublishResult{}, fmt.Errorf("open reference database: %w", err)
	}
	defer store.Close()
	owner, err := store.ActiveUserByEmail(ctx, FixtureOwnerEmail)
	if err != nil {
		return PublishResult{}, fmt.Errorf("load fixture owner: %w", err)
	}
	for _, productID := range manifest.PublishedProductIDs {
		product, err := store.Product(ctx, productID)
		if err != nil {
			return PublishResult{}, fmt.Errorf("load product %s for publication: %w", productID, err)
		}
		if product.RecordState != catalog.RecordCurrent {
			return PublishResult{}, fmt.Errorf("published fixture product %s is not current", productID)
		}
		if product.Status == catalog.Published {
			continue
		}
		product.Status = catalog.Published
		if _, err := store.UpdateProduct(ctx, owner.ID, product.Revision, product); err != nil {
			return PublishResult{}, fmt.Errorf("request publication for %s: %w", productID, err)
		}
	}
	if baseURL == "" {
		baseURL = "https://catalog.example.test"
	}
	engine, err := publishing.NewEngine(ctx, store, publishing.Config{
		Root: filepath.Join(root, "generated", "public"), AssetRoot: filepath.Join(root, "assets"), BaseURL: baseURL,
	})
	if err != nil {
		return PublishResult{}, fmt.Errorf("start fixture publisher: %w", err)
	}
	defer engine.Close()
	website, err := store.WebsiteState(ctx)
	if err != nil {
		return PublishResult{}, fmt.Errorf("load reference website working state: %w", err)
	}
	if !reflect.DeepEqual(website.Working, website.Active) || !reflect.DeepEqual(website.WorkingLocalization, website.ActiveLocalization) {
		epoch, routes, routeErr := store.PublicSiteRouteConfig(ctx)
		if routeErr != nil {
			return PublishResult{}, fmt.Errorf("load reference route configuration: %w", routeErr)
		}
		if _, routeErr = engine.PublishSiteRoutes(ctx, publishing.SiteRouteRequest{
			ActorID: owner.ID, ExpectedEpoch: epoch, ExpectedWorkingRevision: website.WorkingRevision, Config: routes,
		}); routeErr != nil {
			return PublishResult{}, fmt.Errorf("publish reference website configuration: %w", routeErr)
		}
	}
	if _, err := engine.ProcessBatch(ctx, 0); err != nil {
		return PublishResult{}, fmt.Errorf("process reference publication batch: %w", err)
	}
	health, err := waitForPublication(ctx, store, engine, int64(len(manifest.PublishedProductIDs)))
	if err != nil {
		return PublishResult{}, err
	}
	manifest.PublicationApplied = true
	if err := writeJSON(manifestPath, manifest); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{DataRoot: root, Published: len(manifest.PublishedProductIDs), ManifestPath: manifestPath, PublicRoot: filepath.Join(root, "generated", "public"), PublicationLag: health}, nil
}

func configureListingProfiles(ctx context.Context, store *storesqlite.Store, actorID string) error {
	state, err := store.WebsiteState(ctx)
	if err != nil {
		return fmt.Errorf("load listing profile working state: %w", err)
	}
	state.Working.CategoryListingProfiles = map[string]site.CategoryListingProfile{
		categoryID("semi-ldo"): {
			VisibleColumns: []string{"part_number", "name", "manufacturer", "spec:spc_input_voltage", "spec:spc_output_voltage", "spec:spc_output_current", "package", "documents", "rfq"},
			DefaultSort:    "part_number", DefaultSortDirection: "asc",
			MobileKeySpecs: []string{"spec:spc_input_voltage", "spec:spc_output_voltage", "spec:spc_output_current"},
		},
		categoryID("semi-buck"): {
			VisibleColumns: []string{"part_number", "manufacturer", "spec:spc_output_current", "spec:spc_input_voltage", "spec:spc_switching_frequency", "package", "documents", "rfq"},
			DefaultSort:    "manufacturer", DefaultSortDirection: "asc",
			MobileKeySpecs: []string{"spec:spc_output_current", "spec:spc_input_voltage"},
		},
		categoryID("connector-board"): {
			VisibleColumns: []string{"part_number", "name", "manufacturer", "spec:spc_positions", "spec:spc_pitch", "spec:spc_rated_contact_current", "documents", "rfq"},
			DefaultSort:    "name", DefaultSortDirection: "asc",
			MobileKeySpecs: []string{"spec:spc_positions", "spec:spc_pitch"},
		},
	}
	if _, err := store.SaveWebsiteWorking(ctx, actorID, state.WorkingRevision, state.Working); err != nil {
		return fmt.Errorf("save reference listing profiles: %w", err)
	}
	return nil
}

func Verify(ctx context.Context, dataRoot string) (Manifest, error) {
	root, err := existingDataRoot(dataRoot)
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := readManifest(filepath.Join(root, ManifestFilename))
	if err != nil {
		return Manifest{}, err
	}
	store, err := storesqlite.OpenReady(filepath.Join(root, "prods.db"))
	if err != nil {
		return Manifest{}, err
	}
	defer store.Close()
	for _, productID := range manifest.PublishedProductIDs {
		product, err := store.Product(ctx, productID)
		if err != nil || product.RecordState != catalog.RecordCurrent {
			return Manifest{}, fmt.Errorf("published expectation %s is missing or non-current", productID)
		}
		if manifest.PublicationApplied && product.Status != catalog.Published {
			return Manifest{}, fmt.Errorf("published expectation %s remains hidden", productID)
		}
	}
	for _, productID := range manifest.HiddenProductIDs {
		product, err := store.Product(ctx, productID)
		if err != nil || product.RecordState != catalog.RecordCurrent || product.Status != catalog.Hidden {
			return Manifest{}, fmt.Errorf("hidden expectation %s does not match database", productID)
		}
	}
	for _, productID := range manifest.ArchivedProductIDs {
		product, err := store.Product(ctx, productID)
		if err != nil || product.RecordState != catalog.RecordArchived {
			return Manifest{}, fmt.Errorf("archived expectation %s does not match database", productID)
		}
	}
	if manifest.PublicationApplied {
		health, err := store.OperationalHealth(ctx)
		if err != nil {
			return Manifest{}, err
		}
		if health.ActivePublicProducts != int64(len(manifest.PublishedProductIDs)) || health.PublicationPending != 0 || health.PublicationProcessing != 0 || health.PublicationFailed != 0 || health.PublicationDirty != 0 {
			return Manifest{}, fmt.Errorf("publication state not converged: %+v", health)
		}
		for _, expectation := range manifest.Queries {
			results, err := store.SearchPublications(ctx, catalog.FoldSearch(expectation.Query), catalog.SearchProjectionVersion)
			if err != nil {
				return Manifest{}, fmt.Errorf("verify query %s: %w", expectation.Name, err)
			}
			actual := make([]string, len(results))
			for index := range results {
				actual[index] = results[index].ProductID
			}
			if !slices.Equal(actual, expectation.ProductIDs) {
				return Manifest{}, fmt.Errorf("query %s results differ: got %d want %d", expectation.Name, len(actual), len(expectation.ProductIDs))
			}
		}
	}
	return manifest, nil
}

func loadTaxonomy(ctx context.Context, store *storesqlite.Store, actorID string, dataset Dataset) error {
	for _, entry := range dataset.Dictionaries {
		if _, err := store.CreateDictionaryEntry(ctx, actorID, entry); err != nil {
			return fmt.Errorf("create dictionary %s: %w", entry.ID, err)
		}
	}
	categoryRevisions := make(map[string]int64, len(dataset.Categories))
	for _, category := range dataset.Categories {
		created, err := store.CreateCategory(ctx, actorID, category)
		if err != nil {
			return fmt.Errorf("create category %s: %w", category.ID, err)
		}
		categoryRevisions[created.ID] = created.Revision
	}
	for _, spec := range dataset.Specs {
		if _, err := store.CreateSpecDefinition(ctx, actorID, spec); err != nil {
			return fmt.Errorf("create spec %s: %w", spec.ID, err)
		}
	}
	for _, set := range dataset.SpecSets {
		if _, err := store.CreateSpecSet(ctx, actorID, set); err != nil {
			return fmt.Errorf("create spec set %s: %w", set.ID, err)
		}
	}
	for _, category := range dataset.Categories {
		setID := dataset.CategorySpecSet[category.ID]
		if err := store.SetCategorySpecSet(ctx, actorID, category.ID, setID, categoryRevisions[category.ID]); err != nil {
			return fmt.Errorf("assign spec set %s to %s: %w", setID, category.ID, err)
		}
	}
	return nil
}

func loadProducts(ctx context.Context, store *storesqlite.Store, actorID string, dataset Dataset) error {
	documentTypes, err := store.ListDictionaryEntries(ctx, catalog.DictionaryDocumentType)
	if err != nil || len(documentTypes) == 0 {
		return fmt.Errorf("load default document types: count=%d err=%w", len(documentTypes), err)
	}
	documentTypeID := documentTypes[0].ID
	for index, seed := range dataset.Products {
		product := seed.Product
		product.Status = catalog.Hidden
		created, err := store.CreateProduct(ctx, actorID, product)
		if err != nil {
			return fmt.Errorf("create product %s (%d/%d): %w", product.ID, index+1, len(dataset.Products), err)
		}
		revision := created.Revision
		for _, value := range seed.SpecValues {
			if _, err := store.SaveSpecValue(ctx, actorID, created.ID, value.SpecID, value.RawValue, value.SourceLocale, revision); err != nil {
				return fmt.Errorf("save %s for %s: %w", value.SpecID, created.ID, err)
			}
			revision++
		}
		for _, translation := range seed.Translations {
			if _, err := store.SaveProductTranslation(ctx, actorID, created.ID, revision, translation); err != nil {
				return fmt.Errorf("save %s translation for %s: %w", translation.Locale, created.ID, err)
			}
			revision++
		}
		for _, document := range seed.Documents {
			document.DocumentTypeID = documentTypeID
			if _, err := store.AddProductDocument(ctx, actorID, revision, document); err != nil {
				return fmt.Errorf("add document %s to %s: %w", document.ID, created.ID, err)
			}
			revision++
		}
		if seed.DesiredState == DesiredArchived {
			if err := store.ArchiveProduct(ctx, actorID, created.ID, revision); err != nil {
				return fmt.Errorf("archive product %s: %w", created.ID, err)
			}
		}
	}
	return nil
}

func waitForPublication(ctx context.Context, store *storesqlite.Store, engine *publishing.Engine, expected int64) (storesqlite.OperationalHealth, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		health, err := store.OperationalHealth(ctx)
		if err != nil {
			return health, err
		}
		if health.PublicationFailed > 0 {
			return health, fmt.Errorf("reference publication failed: %+v", health)
		}
		if health.ActivePublicProducts == expected && health.PublicationPending == 0 && health.PublicationProcessing == 0 && health.PublicationDirty == 0 && health.SearchProjectionDrift == 0 {
			return health, nil
		}
		engine.Wake()
		select {
		case <-ctx.Done():
			return health, fmt.Errorf("wait for reference publication: %w (state=%+v)", ctx.Err(), health)
		case <-ticker.C:
		}
	}
}

func newDataRoot(value string) (string, error) {
	if value == "" {
		return "", ErrUnsafeTarget
	}
	root, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(root); err == nil || !errors.Is(err, os.ErrNotExist) {
		return "", ErrUnsafeTarget
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func existingDataRoot(value string) (string, error) {
	if value == "" {
		return "", ErrUnsafeTarget
	}
	root, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", ErrUnsafeTarget
	}
	return root, nil
}

func readManifest(path string) (Manifest, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func writeReferencePDF(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Deliberately tiny, locally generated PDF test material. It contains no
	// third-party catalog content and is not represented as an engineering
	// datasheet.
	pdf := []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 300 180]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n4 0 obj<</Length 82>>stream\nBT /F1 12 Tf 32 120 Td (Prods synthetic reference datasheet - not for selection) Tj ET\nendstream endobj\n5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n")
	return os.WriteFile(path, pdf, 0o600)
}
