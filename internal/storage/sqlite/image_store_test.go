package sqlite

import (
	"strings"
	"testing"

	"prods/internal/catalog"
)

func TestClonePreservesProductImageReferencesWithIndependentRows(t *testing.T) {
	ctx := t.Context()
	store, owner := installedStore(t)
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "IMAGE-SOURCE"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.CreateAsset(ctx, owner.ID, catalog.Asset{
		ID: "ast_image_source", OwnerType: "product", OwnerID: product.ID, OriginalFilename: "source.png",
		StoragePath: "product/source/content.png", MIMEType: "image/png", SizeBytes: 1, Checksum: strings.Repeat("00", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	image, err := store.AddProductImage(ctx, owner.ID, product.Revision, catalog.ProductImage{
		ProductID: product.ID, AssetID: asset.ID, AltText: "Front", SortOrder: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	clone, err := store.CloneProduct(ctx, owner.ID, product.ID, product.Revision+1, catalog.Product{PartNumber: "IMAGE-CLONE"})
	if err != nil {
		t.Fatal(err)
	}
	images, err := store.ProductImages(ctx, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ID == image.ID || images[0].AssetID != asset.ID || images[0].AltText != image.AltText || !images[0].Primary {
		t.Fatalf("cloned images = %+v", images)
	}
}
