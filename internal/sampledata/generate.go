package sampledata

import (
	"strings"

	"prods/internal/catalog"
	"prods/internal/distribution"
	"prods/internal/referencecatalog"
)

// Generate creates the deterministic release-bound sample payload. This
// package is used by the release asset generator and is not linked into the
// Prods runtime binary.
func Generate(releaseVersion string) (distribution.SampleData, error) {
	releaseVersion = strings.TrimSpace(releaseVersion)
	dataset, _, err := referencecatalog.Build(referencecatalog.DefaultSeed)
	if err != nil {
		return distribution.SampleData{}, err
	}
	sample := distribution.SampleData{
		SchemaVersion: 1, ReleaseVersion: releaseVersion, DatasetVersion: dataset.Version,
		Seed: dataset.Seed, SourceStatement: referencecatalog.SourceStatement,
		Dictionaries:     make([]distribution.DictionaryEntry, 0, len(dataset.Dictionaries)),
		Categories:       make([]distribution.Category, 0, len(dataset.Categories)),
		Specs:            make([]distribution.SpecDefinition, 0, len(dataset.Specs)),
		SpecSets:         make([]distribution.SpecSet, 0, len(dataset.SpecSets)),
		CategorySpecSets: make([]distribution.CategorySpecSet, 0, len(dataset.CategorySpecSet)),
		Products:         make([]distribution.Product, 0, len(dataset.Products)),
	}
	for _, entry := range dataset.Dictionaries {
		sample.Dictionaries = append(sample.Dictionaries, distribution.DictionaryEntry{
			ID: entry.ID, Kind: string(entry.Kind), Name: entry.Name, Description: entry.Description,
			SourceLocale: entry.SourceLocale, Slug: entry.Slug, Status: string(entry.Status),
		})
	}
	for _, category := range dataset.Categories {
		sample.Categories = append(sample.Categories, distribution.Category{
			ID: category.ID, ParentID: category.ParentID, Name: category.Name, Description: category.Description,
			SourceLocale: category.SourceLocale, Slug: category.Slug, Status: string(category.Status),
		})
		if specSetID := dataset.CategorySpecSet[category.ID]; specSetID != "" {
			sample.CategorySpecSets = append(sample.CategorySpecSets, distribution.CategorySpecSet{
				CategoryID: category.ID, SpecSetID: specSetID,
			})
		}
	}
	for _, spec := range dataset.Specs {
		sample.Specs = append(sample.Specs, distribution.SpecDefinition{
			ID: spec.ID, Name: spec.Name, PreferredUnit: spec.PreferredUnit, Filterable: spec.Filterable,
			SemanticVersion: spec.SemanticVer, Status: string(spec.Status),
		})
	}
	for _, set := range dataset.SpecSets {
		sample.SpecSets = append(sample.SpecSets, distribution.SpecSet{
			ID: set.ID, Name: set.Name, Status: string(set.Status), SpecIDs: append([]string(nil), set.SpecIDs...),
		})
	}
	for _, seed := range dataset.Products {
		product := seed.Product
		status := catalog.Hidden
		recordState := catalog.RecordCurrent
		switch seed.DesiredState {
		case referencecatalog.DesiredPublished:
			status = catalog.Published
		case referencecatalog.DesiredArchived:
			recordState = catalog.RecordArchived
		}
		sample.Products = append(sample.Products, distribution.Product{
			ID: product.ID, PartNumber: product.PartNumber, Name: product.Name,
			SourceLocale: product.SourceLocale, ManufacturerID: product.ManufacturerID, Manufacturer: product.Manufacturer,
			BrandID: product.BrandID, Brand: product.Brand, LifecycleID: product.LifecycleID,
			ApplicationIDs: append([]string(nil), product.ApplicationIDs...), CategoryID: product.CategoryID,
			PackageFormFactor: product.PackageFormFactor, Description: product.Description,
			Features: product.Features, Specification: product.Specification, DocumentURL: product.DocumentURL,
			Status: string(status), RecordState: string(recordState), SpecValues: sampleSpecValues(seed.SpecValues),
		})
	}
	return sample, nil
}

func sampleSpecValues(values []referencecatalog.SpecValueSeed) []distribution.SpecValue {
	result := make([]distribution.SpecValue, 0, len(values))
	for _, value := range values {
		result = append(result, distribution.SpecValue{
			SpecID: value.SpecID, RawValue: value.RawValue, SourceLocale: value.SourceLocale,
		})
	}
	return result
}
