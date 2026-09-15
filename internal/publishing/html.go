package publishing

import (
	"bytes"
	"html/template"
	"net/url"

	"prods/internal/localization"
	"prods/internal/site"
)

var productTemplate = template.Must(template.New("product").Parse(`<!doctype html>
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
<style>{{.ThemeCSS}}</style>
{{if .View.Site.Theme.CustomCSSEnabled}}<link rel="stylesheet" href="/site.css?v={{.View.SiteEpoch}}">{{end}}
</head>
<body>
<header><a href="{{.CatalogURL}}">{{if .View.Site.Organization.PrimaryLogoAsset}}<img src="/assets/{{.View.Site.Organization.PrimaryLogoAsset}}" alt="{{.View.Site.Organization.DisplayName}}">{{else}}{{.View.Site.Organization.DisplayName}}{{end}}</a><nav aria-label="Primary">{{range .Navigation}}<a href="{{.URL}}"{{if .OpenNewWindow}} target="_blank" rel="noopener noreferrer"{{end}}>{{.Label}}</a> {{end}}</nav>{{if .LanguageLinks}}<nav aria-label="{{.Text.Language}}">{{range .LanguageLinks}}<a href="{{.URL}}" hreflang="{{.Locale}}"{{if .Active}} aria-current="page"{{end}}>{{.Locale}}</a> {{end}}</nav>{{end}}</header>
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
<footer>{{if .View.Site.Organization.LegalName}}{{.View.Site.Organization.LegalName}}{{else}}{{.View.Site.Organization.DisplayName}}{{end}}{{if .View.Site.Organization.PrivacyURL}} · <a href="{{.View.Site.Organization.PrivacyURL}}">{{.Text.Privacy}}</a>{{end}}{{if .View.Site.Organization.TermsURL}} · <a href="{{.View.Site.Organization.TermsURL}}">{{.Text.Terms}}</a>{{end}}</footer>
<script type="module" src="/static/public/public-islands.js"></script>
</body>
</html>`))

type languageLink struct {
	Locale string
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
	data := struct {
		View          PublicView
		JSONLD        template.JS
		Description   string
		Navigation    []site.NavigationItem
		ThemeCSS      template.CSS
		Text          localization.Messages
		CatalogURL    string
		RFQURL        string
		LanguageLinks []languageLink
	}{
		View: view, JSONLD: template.JS(jsonLD), Description: description,
		Navigation: configuration.VisibleNavigation(), ThemeCSS: template.CSS(configuration.Stylesheet()),
		Text: localization.ApplyPublicCopy(localization.For(locale), view.PublicCopy), CatalogURL: localizedURL("/search", locale, explicit),
		RFQURL: localizedURL(view.RFQURL, locale, explicit),
	}
	for _, supported := range view.SupportedLocales {
		data.LanguageLinks = append(data.LanguageLinks, languageLink{
			Locale: supported, URL: localizedURL(view.CanonicalURL, supported, true), Active: supported == locale,
		})
	}
	var out bytes.Buffer
	if err := productTemplate.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
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
