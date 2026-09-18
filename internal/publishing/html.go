package publishing

import (
	"bytes"
	"html/template"
	"net/url"

	"prods/internal/localization"
	"prods/internal/site"
)

var productTemplate = template.Must(template.New("product").Funcs(template.FuncMap{"headerURL": trustedHeaderURL}).Parse(`{{define "product-nav-nodes"}}{{range .}}<li>{{if .Children}}<details class="site-nav-group"><summary>{{.Item.Label}}</summary>{{if .Item.URL}}<a class="site-nav-overview" href="{{.Item.URL}}" aria-label="{{.Item.Label}}" title="{{.Item.Label}}"{{if .Item.OpenNewWindow}} target="_blank" rel="noopener noreferrer"{{end}}><span aria-hidden="true">→</span></a>{{end}}<ul>{{template "product-nav-nodes" .Children}}</ul></details>{{else if .Item.URL}}<a href="{{.Item.URL}}"{{if .Item.OpenNewWindow}} target="_blank" rel="noopener noreferrer"{{end}}>{{.Item.Label}}</a>{{else}}<span>{{.Item.Label}}</span>{{end}}</li>{{end}}{{end}}
{{define "product-footer-items"}}{{range .}}<li>{{if .TrustedURL}}<a href="{{.TrustedURL}}">{{.Label}}</a>{{else if .URL}}<a href="{{.URL}}">{{.Label}}</a>{{else}}<span>{{.Label}}</span>{{end}}</li>{{end}}{{end}}
<!doctype html>
<html lang="{{.View.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{if .View.Name}}{{.View.Name}} — {{end}}{{.View.PartNumber}}</title>
{{if .Description}}<meta name="description" content="{{.Description}}">{{end}}
{{if .View.CanonicalURL}}<link rel="canonical" href="{{.View.CanonicalURL}}"><link rel="alternate" hreflang="x-default" href="{{.View.CanonicalURL}}"><link rel="alternate" type="application/json" href="{{.View.CanonicalURL}}.json"><link rel="alternate" type="text/markdown" href="{{.View.CanonicalURL}}.md">{{end}}
{{if .View.Site.Organization.FaviconAsset}}<link rel="icon" href="/assets/{{.View.Site.Organization.FaviconAsset}}">{{end}}
{{if .View.Site.Organization.SocialImageAsset}}<meta property="og:image" content="/assets/{{.View.Site.Organization.SocialImageAsset}}">{{end}}
<script type="application/ld+json">{{.JSONLD}}</script>
<link rel="stylesheet" href="/static/public/public.css">
<style>{{.ThemeCSS}}</style>
{{if .View.Site.Theme.CustomCSSEnabled}}<link rel="stylesheet" href="/site.css?v={{.View.SiteEpoch}}">{{end}}
</head>
<body data-site-epoch="{{.View.SiteEpoch}}">
<header class="site-header site-header-{{.View.Site.Header.Layout}}{{if .View.Site.Header.Sticky}} is-sticky{{end}}">
{{range .View.Site.Header.Rows}}{{if eq .Type "announcement"}}<div class="site-header-row announcement-row">{{range .Items}}{{if eq .Kind "text"}}<span>{{.Text}}</span>{{else if eq .Kind "link"}}<a href="{{headerURL .URL}}">{{.Label}}</a>{{end}}{{end}}</div>{{end}}{{end}}
{{range .View.Site.Header.Rows}}{{if eq .Type "utility"}}<div class="site-header-row utility-row">{{range .Items}}{{if eq .Kind "text"}}<span>{{.Text}}</span>{{else if eq .Kind "link"}}<a href="{{headerURL .URL}}">{{.Label}}</a>{{else if eq .SystemAction "rfq"}}<a href="{{$.RFQURL}}">{{$.Text.RequestQuote}}</a>{{else if eq .SystemAction "catalog"}}<a href="{{$.CatalogURL}}">{{$.Text.Catalog}}</a>{{end}}{{end}}</div>{{end}}{{end}}
<div class="site-header-row main-row"><a class="site-brand" href="{{.CatalogURL}}"><strong>{{if .View.Site.Organization.PrimaryLogoAsset}}<img src="/assets/{{.View.Site.Organization.PrimaryLogoAsset}}" alt="{{.View.Site.Organization.DisplayName}}">{{else}}{{.View.Site.Organization.DisplayName}}{{end}}</strong></a>
{{range .View.Site.Header.Rows}}{{if eq .Type "main"}}{{range .Items}}{{if eq .Kind "link"}}<a href="{{headerURL .URL}}">{{.Label}}</a>{{else if eq .SystemAction "catalog_search"}}<form class="header-search" action="{{$.SearchURL}}" method="get" role="search"><label><span class="visually-hidden">{{$.Text.SearchLabel}}</span><input name="q" placeholder="{{$.Text.SearchLabel}}"></label><button>{{$.Text.Search}}</button></form>{{else if eq .SystemAction "rfq"}}<a class="header-rfq" href="{{$.RFQURL}}">{{$.Text.RequestQuote}}</a>{{end}}{{end}}{{end}}{{end}}
{{if and .ShowHeaderLocale .LanguageLinks}}<details class="language-menu"><summary>{{range .LanguageLinks}}{{if .Active}}{{.Name}}{{end}}{{end}}</summary><nav aria-label="{{.Text.Language}}">{{range .LanguageLinks}}<a href="{{.URL}}" hreflang="{{.Locale}}"{{if .Active}} aria-current="page"{{end}}>{{.Name}}</a>{{end}}</nav></details>{{end}}</div>
{{if .NavigationTree}}<nav class="primary-navigation" aria-label="Primary"><ul>{{template "product-nav-nodes" .NavigationTree}}</ul></nav>{{end}}</header>
<main>
<article data-product-id="{{.View.ID}}" data-public-revision="{{.View.Revision}}" data-site-epoch="{{.View.SiteEpoch}}">
<h1>{{if .View.Name}}{{.View.Name}}{{else}}{{.View.PartNumber}}{{end}}</h1>
{{if .View.Images}}<section aria-label="{{.Text.ProductImages}}"><ul>{{range .View.Images}}<li><img src="{{.URL}}" alt="{{.AltText}}" loading="lazy"{{if .Primary}} fetchpriority="high"{{end}}></li>{{end}}</ul></section>{{end}}
<dl>
<dt>{{.Text.PartNumber}}</dt><dd>{{.View.PartNumber}}</dd>
{{if .View.Manufacturer}}<dt>{{.Text.Manufacturer}}</dt><dd>{{if .View.ManufacturerURL}}<a href="{{.View.ManufacturerURL}}">{{.View.Manufacturer}}</a>{{else}}{{.View.Manufacturer}}{{end}}</dd>{{end}}
{{if .View.Brand}}<dt>{{.Text.Brand}}</dt><dd>{{if .View.BrandURL}}<a href="{{.View.BrandURL}}">{{.View.Brand}}</a>{{else}}{{.View.Brand}}{{end}}</dd>{{end}}
{{if .View.Category}}<dt>{{.Text.Category}}</dt><dd>{{if .View.CategoryURL}}<a href="{{.View.CategoryURL}}">{{.View.Category}}</a>{{else}}{{.View.Category}}{{end}}</dd>{{end}}
{{if .View.Lifecycle}}<dt>{{.Text.Lifecycle}}</dt><dd>{{.View.Lifecycle}}</dd>{{end}}
{{if .View.Applications}}<dt>{{.Text.Applications}}</dt><dd>{{range $index, $application := .View.Applications}}{{if $index}}, {{end}}<a href="{{$application.URL}}">{{$application.Name}}</a>{{end}}</dd>{{end}}
</dl>
{{if .View.Description}}<section><h2>{{.Text.Description}}</h2><p>{{.View.Description}}</p></section>{{end}}
{{if .View.Features}}<section><h2>{{.Text.Features}}</h2><p>{{.View.Features}}</p></section>{{end}}
{{if .View.Specification}}<section><h2>{{.Text.Specification}}</h2><p>{{.View.Specification}}</p></section>{{end}}
{{if .View.Specifications}}<section><h2>{{.Text.Specifications}}</h2><dl>{{range .View.Specifications}}<dt>{{.Name}}</dt><dd>{{.RawValue}}{{if .PreferredUnit}} {{.PreferredUnit}}{{end}}</dd>{{end}}</dl></section>{{end}}
{{if .View.Documents}}<section><h2>{{.Text.Documents}}</h2><ul>{{range .View.Documents}}<li><a href="{{.URL}}">{{.Label}}</a>{{if .Language}} ({{.Language}}){{end}}</li>{{end}}</ul></section>{{end}}
{{if .RFQURL}}<section aria-labelledby="rfq-heading"><h2 id="rfq-heading">{{.Text.RequestQuote}}</h2><p><a href="{{.RFQURL}}">{{.Text.RequestQuote}}</a></p><div id="rfq-island" data-product-id="{{.View.ID}}" data-part-number="{{.View.PartNumber}}"></div></section>{{end}}
</article>
</main>
<footer class="site-footer site-footer-{{.View.Site.Footer.Layout}}"><div class="footer-grid">
{{if .View.Site.Footer.BrandBlock.ShowBrand}}<section class="footer-brand"><h2>{{.View.Site.Organization.DisplayName}}</h2>{{if and .View.Site.Footer.BrandBlock.ShowDescription .View.Site.Organization.Description}}<p>{{.View.Site.Organization.Description}}</p>{{end}}{{range .FooterBrandContacts}}<p>{{if .TrustedURL}}<a href="{{.TrustedURL}}">{{.Label}}</a>{{else if .URL}}<a href="{{.URL}}">{{.Label}}</a>{{else}}{{.Label}}{{end}}</p>{{end}}</section>{{end}}
{{range .FooterSections}}{{if .Collapsible}}<details class="footer-section footer-section-collapsible" open><summary>{{.Heading}}</summary><ul>{{template "product-footer-items" .Items}}</ul></details>{{else}}<section class="footer-section"><h2>{{.Heading}}</h2><ul>{{template "product-footer-items" .Items}}</ul></section>{{end}}{{end}}</div>
{{if and .View.Site.Footer.ShowSocialLinks .View.Site.Organization.SocialLinks}}<nav class="footer-social" aria-label="Social">{{range .View.Site.Organization.SocialLinks}}<a href="{{.URL}}" target="_blank" rel="noopener noreferrer">{{.Label}}</a>{{end}}</nav>{{end}}
<nav class="footer-legal" aria-label="Legal">{{range .View.Site.Footer.LegalLinks}}<a href="{{.URL}}">{{.Label}}</a>{{end}}{{if .View.Site.Organization.PrivacyURL}}<a href="{{.View.Site.Organization.PrivacyURL}}">{{.Text.Privacy}}</a>{{end}}{{if .View.Site.Organization.TermsURL}}<a href="{{.View.Site.Organization.TermsURL}}">{{.Text.Terms}}</a>{{end}}</nav>
{{if and .ShowFooterLocale .LanguageLinks}}<details class="language-menu footer-language"><summary>{{range .LanguageLinks}}{{if .Active}}{{.Name}}{{end}}{{end}}</summary><nav aria-label="{{.Text.Language}}">{{range .LanguageLinks}}<a href="{{.URL}}" hreflang="{{.Locale}}"{{if .Active}} aria-current="page"{{end}}>{{.Name}}</a>{{end}}</nav></details>{{end}}
<div class="footer-company">{{if .View.Site.Footer.CopyrightText}}<p>{{.View.Site.Footer.CopyrightText}}</p>{{else}}<p>{{if .View.Site.Organization.LegalName}}{{.View.Site.Organization.LegalName}}{{else}}{{.View.Site.Organization.DisplayName}}{{end}}</p>{{end}}{{range .View.Site.Footer.RegistrationLines}}<p>{{.}}</p>{{end}}{{if .View.Site.Footer.Disclaimer}}<p class="footer-disclaimer">{{.View.Site.Footer.Disclaimer}}</p>{{end}}</div></footer>
<script type="module" src="/static/public/public-islands.js"></script>
</body>
</html>`))

type footerLink struct {
	Label      string
	URL        string
	TrustedURL template.URL
}

type footerSection struct {
	Heading     string
	Collapsible bool
	Items       []footerLink
}

type languageLink struct {
	Locale string
	Name   string
	URL    string
	Active bool
}

func HTML(view PublicView) ([]byte, error) {
	return renderHTML(view, view.Language, false)
}

func HTMLForLocale(view PublicView, locale string) ([]byte, error) {
	return renderHTML(view, locale, true)
}

func renderHTML(view PublicView, locale string, explicit bool) ([]byte, error) {
	view.Applications = append([]Application(nil), view.Applications...)
	configuration := view.Site
	if err := configuration.Prepare(); err != nil {
		configuration = site.DefaultConfiguration()
	}
	view.Site = configuration
	if locale == "" {
		locale = view.Language
	}
	view.Language = locale
	jsonLD, err := JSONLD(view)
	if err != nil {
		return nil, err
	}
	if explicit {
		view.ManufacturerURL = localizedURL(view.ManufacturerURL, locale, true)
		view.BrandURL = localizedURL(view.BrandURL, locale, true)
		view.CategoryURL = localizedURL(view.CategoryURL, locale, true)
		for index := range view.Applications {
			view.Applications[index].URL = localizedURL(view.Applications[index].URL, locale, true)
		}
	}
	description := view.Description
	if description == "" {
		description = configuration.SEO.DefaultDescription
	}
	messages := localization.ApplyPublicCopy(localization.For(locale), view.PublicCopy)
	catalogURL := localizedURL("/catalog", locale, explicit)
	rfqURL := localizedURL(view.RFQURL, locale, explicit)
	searchURL := localizedURL("/search", locale, explicit)
	footerBrandContacts, footerSections := buildFooterViews(configuration, messages, catalogURL, searchURL, rfqURL)
	data := struct {
		View                PublicView
		JSONLD              template.JS
		Description         string
		NavigationTree      []*site.NavigationNode
		ThemeCSS            template.CSS
		Text                localization.Messages
		CatalogURL          string
		SearchURL           string
		RFQURL              string
		LanguageLinks       []languageLink
		FooterBrandContacts []footerLink
		FooterSections      []footerSection
		ShowHeaderLocale    bool
		ShowFooterLocale    bool
	}{
		View: view, JSONLD: template.JS(jsonLD), Description: description,
		NavigationTree: configuration.VisibleNavigationTree(), ThemeCSS: template.CSS(configuration.Stylesheet()),
		Text: messages, CatalogURL: catalogURL, SearchURL: searchURL, RFQURL: rfqURL,
		FooterBrandContacts: footerBrandContacts, FooterSections: footerSections,
		ShowHeaderLocale: configuration.ShowHeaderLocale(), ShowFooterLocale: configuration.ShowFooterLocale(),
	}
	for _, supported := range view.SupportedLocales {
		data.LanguageLinks = append(data.LanguageLinks, languageLink{
			Locale: supported,
			Name:   localization.BuiltinLocaleName(supported),
			URL:    localizedURL(view.CanonicalURL, supported, true),
			Active: supported == locale,
		})
	}
	var out bytes.Buffer
	if err := productTemplate.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func buildFooterViews(configuration site.Configuration, messages localization.Messages, catalogURL, searchURL, rfqURL string) ([]footerLink, []footerSection) {
	contacts := make(map[string]site.ContactMethod, len(configuration.Organization.Contacts))
	for _, contact := range configuration.Organization.Contacts {
		contacts[contact.ID] = contact
	}
	brandContacts := make([]footerLink, 0, len(configuration.Footer.BrandBlock.ContactIDs))
	for _, id := range configuration.Footer.BrandBlock.ContactIDs {
		if contact, ok := contacts[id]; ok {
			brandContacts = append(brandContacts, footerLink{Label: contact.Label + ": " + contact.Value, TrustedURL: trustedContactURL(contact.URL)})
		}
	}
	sections := make([]footerSection, 0, len(configuration.Footer.Sections))
	for _, source := range configuration.Footer.Sections {
		section := footerSection{Heading: source.Heading, Collapsible: source.CollapsibleOnMobile}
		for _, item := range source.Items {
			link := footerLink{}
			switch item.Kind {
			case "link":
				link = footerLink{Label: item.Label, URL: item.URL}
			case "text":
				link.Label = item.Text
			case "contact_ref":
				if contact, ok := contacts[item.ContactID]; ok {
					link = footerLink{Label: contact.Label + ": " + contact.Value, TrustedURL: trustedContactURL(contact.URL)}
				}
			case "system_action":
				switch item.SystemAction {
				case "catalog":
					link = footerLink{Label: messages.Catalog, URL: catalogURL}
				case "catalog_search":
					link = footerLink{Label: messages.Search, URL: searchURL}
				case "rfq":
					link = footerLink{Label: messages.RequestQuote, URL: rfqURL}
				case "locale":
					link.Label = messages.Language
				}
			}
			if link.Label != "" {
				section.Items = append(section.Items, link)
			}
		}
		sections = append(sections, section)
	}
	return brandContacts, sections
}

func trustedContactURL(value string) template.URL {
	if value == "" || !site.ValidContactURL(value) {
		return ""
	}
	return template.URL(value) // #nosec G203 -- restricted by site.ValidContactURL.
}

func trustedHeaderURL(value string) template.URL {
	if value == "" || !site.ValidHeaderLinkURL(value) {
		return ""
	}
	return template.URL(value) // #nosec G203 -- restricted by site.ValidHeaderLinkURL.
}

func localizedURL(rawURL, locale string, explicit bool) string {
	if !explicit || rawURL == "" {
		return rawURL
	}
	location, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := location.Query()
	query.Set("lang", locale)
	location.RawQuery = query.Encode()
	return location.String()
}
