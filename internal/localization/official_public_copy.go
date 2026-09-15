package localization

const OfficialPublicCopyReviewStatus = "machine-generated; not reviewed by professional native, legal, or marketing reviewers"

type officialCopyField struct {
	key         string
	description string
	value       func(Messages) string
}

var officialCopyFields = []officialCopyField{
	{"catalog.title", "Public catalog heading", func(m Messages) string { return m.Catalog }},
	{"catalog.request_part", "Public catalog request-part action", func(m Messages) string { return m.RequestPart }},
	{"search.label", "Public search field label", func(m Messages) string { return m.SearchLabel }},
	{"search.submit", "Public search submit action", func(m Messages) string { return m.Search }},
	{"search.all", "Public search all-filter option", func(m Messages) string { return m.All }},
	{"search.no_products", "Public no-results heading", func(m Messages) string { return m.NoProducts }},
	{"search.no_products_prefix", "Public no-results explanation prefix", func(m Messages) string { return m.NoProductsPrefix }},
	{"search.request_this_part", "Public no-results RFQ action", func(m Messages) string { return m.RequestThisPart }},
	{"pagination.previous", "Public previous-page action", func(m Messages) string { return m.PreviousPage }},
	{"pagination.next", "Public next-page action", func(m Messages) string { return m.NextPage }},
	{"product.part_number", "Public Product part-number label", func(m Messages) string { return m.PartNumber }},
	{"product.manufacturer", "Public Product manufacturer label", func(m Messages) string { return m.Manufacturer }},
	{"product.brand", "Public Product brand label", func(m Messages) string { return m.Brand }},
	{"product.category", "Public Product category label", func(m Messages) string { return m.Category }},
	{"product.lifecycle", "Public Product lifecycle label", func(m Messages) string { return m.Lifecycle }},
	{"product.applications", "Public Product applications label", func(m Messages) string { return m.Applications }},
	{"product.images", "Public Product images heading", func(m Messages) string { return m.ProductImages }},
	{"product.description", "Public Product description heading", func(m Messages) string { return m.Description }},
	{"product.features", "Public Product features heading", func(m Messages) string { return m.Features }},
	{"product.specification", "Public Product specification heading", func(m Messages) string { return m.Specification }},
	{"product.specifications", "Public Product specifications heading", func(m Messages) string { return m.Specifications }},
	{"product.documents", "Public Product documents heading", func(m Messages) string { return m.Documents }},
	{"product.request_quote", "Public Product RFQ action", func(m Messages) string { return m.RequestQuote }},
	{"rfq.title", "Public RFQ page heading", func(m Messages) string { return m.RFQTitle }},
	{"rfq.catalog_product", "Public RFQ catalog-product label", func(m Messages) string { return m.CatalogProduct }},
	{"rfq.requested_part", "Public RFQ requested-part label", func(m Messages) string { return m.RequestedPart }},
	{"rfq.original_search", "Public RFQ original-search label", func(m Messages) string { return m.OriginalSearch }},
	{"rfq.name", "Public RFQ customer-name label", func(m Messages) string { return m.Name }},
	{"rfq.email", "Public RFQ customer-email label", func(m Messages) string { return m.Email }},
	{"rfq.quantity_optional", "Public RFQ optional-quantity label", func(m Messages) string { return m.QuantityOptional }},
	{"rfq.notes", "Public RFQ notes label", func(m Messages) string { return m.Notes }},
	{"rfq.submit", "Public RFQ submit action", func(m Messages) string { return m.SubmitRFQ }},
	{"rfq.received", "Public RFQ success heading", func(m Messages) string { return m.RFQReceived }},
	{"rfq.reference", "Public RFQ reference label", func(m Messages) string { return m.Reference }},
	{"rfq.replay_notice", "Public RFQ idempotent-replay notice", func(m Messages) string { return m.ReplayNotice }},
	{"footer.privacy", "Public privacy link label", func(m Messages) string { return m.Privacy }},
	{"footer.terms", "Public terms link label", func(m Messages) string { return m.Terms }},
	{"locale.language", "Public language selector label", func(m Messages) string { return m.Language }},
}

func OfficialPublicCopyCatalog() PublicCopyCatalog {
	catalog := PublicCopyCatalog{
		OfficialBundle: OfficialBundleVersion,
		ReviewStatus:   OfficialPublicCopyReviewStatus,
		Definitions:    make([]PublicCopyDefinition, 0, len(officialCopyFields)),
		Defaults:       make([]PublicCopyDefault, 0, len(officialCopyFields)*len(messageCatalogs)),
	}
	for _, field := range officialCopyFields {
		catalog.Definitions = append(catalog.Definitions, PublicCopyDefinition{
			Key: field.key, DefinitionVersion: 1, Description: field.description,
			ValueKind: PublicCopyPlain, OfficialBundle: OfficialBundleVersion,
		})
		for _, locale := range BuiltinLocaleCodes() {
			catalog.Defaults = append(catalog.Defaults, PublicCopyDefault{
				Key: field.key, Locale: locale, Value: field.value(messageCatalogs[locale]),
				DefinitionVersion: 1, OfficialBundle: OfficialBundleVersion,
			})
		}
	}
	return catalog
}
