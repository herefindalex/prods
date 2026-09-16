package sqlite

import (
	"errors"
	"fmt"
	"testing"

	"prods/internal/catalog"
)

func TestProductPagesReadBeyondLegacyLimitAndPreserveTotal(t *testing.T) {
	store, owner := installedStore(t)
	for index := 0; index < 505; index++ {
		if _, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: fmt.Sprintf("PART-%04d", index), Name: "same name"}); err != nil {
			t.Fatal(err)
		}
	}
	query := ProductPageQuery{Page: 26, PageSize: 20, Sort: "part_number", Order: "asc"}
	page, err := store.PageProducts(t.Context(), query)
	if err != nil || page.Total != 505 || len(page.Data) != 5 || page.Data[0].PartNumber != "PART-0500" {
		t.Fatalf("last page: total=%d data=%v err=%v", page.Total, page.Data, err)
	}
	last := page.Data[4]
	if err := store.ArchiveProduct(t.Context(), owner.ID, last.ID, last.Revision); err != nil {
		t.Fatal(err)
	}
	page, err = store.PageProducts(t.Context(), query)
	if err != nil || page.Total != 504 || len(page.Data) != 4 {
		t.Fatalf("current page=%+v err=%v", page, err)
	}
	query.IncludeArchived = true
	query.Order = "desc"
	query.Page = 1
	page, err = store.PageProducts(t.Context(), query)
	if err != nil || page.Total != 505 || page.Data[0].ID != last.ID {
		t.Fatalf("archived descending page=%+v err=%v", page, err)
	}
	query.Sort = "name"
	first, err := store.PageProducts(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	query.Page = 2
	second, err := store.PageProducts(t.Context(), query)
	if err != nil || first.Data[19].ID >= second.Data[0].ID {
		t.Fatalf("tie-break order is not stable: %v", err)
	}
	query.Page = 100
	page, err = store.PageProducts(t.Context(), query)
	if err != nil || page.Total != 505 || page.Data == nil || len(page.Data) != 0 {
		t.Fatalf("empty page=%+v err=%v", page, err)
	}
}

func TestProductPageRejectsUnsupportedQuery(t *testing.T) {
	store, _ := installedStore(t)
	for _, query := range []ProductPageQuery{
		{Page: 0, PageSize: 20, Sort: "name", Order: "asc"},
		{Page: 1, PageSize: 101, Sort: "name", Order: "asc"},
		{Page: 1_000_001, PageSize: 20, Sort: "name", Order: "asc"},
		{Page: 1, PageSize: 20, Sort: "unknown", Order: "asc"},
		{Page: 1, PageSize: 20, Sort: "name", Order: "unknown"},
	} {
		if _, err := store.PageProducts(t.Context(), query); !errors.Is(err, ErrInvalidProductPage) {
			t.Fatalf("query=%+v err=%v", query, err)
		}
	}
}
