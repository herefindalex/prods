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
		Categories: make([]distribution.Category, 0, len(dataset.Categories)),
		Products:   make([]distribution.Product, 0, len(dataset.Products)),
	}
	for _, category := range dataset.Categories {
		sample.Categories = append(sample.Categories, distribution.Category{
			ID: category.ID, ParentID: category.ParentID, Name: category.Name, Description: category.Description,
			SourceLocale: category.SourceLocale, Slug: category.Slug, Status: string(category.Status),
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
			Manufacturer: product.Manufacturer, CategoryID: product.CategoryID,
			PackageFormFactor: product.PackageFormFactor, Description: product.Description,
			Features: product.Features, Specification: product.Specification, DocumentURL: product.DocumentURL,
			Status: string(status), RecordState: string(recordState),
		})
	}
	return sample, nil
}
