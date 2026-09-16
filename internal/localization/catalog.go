package localization

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/text/language"
)

var ErrUnsupportedLocale = errors.New("unsupported locale")

var interfaceLocales = BuiltinLocaleCodes()

func Available() []string {
	return append([]string(nil), interfaceLocales...)
}

func IsAvailable(locale string) bool {
	tag, err := language.Parse(strings.TrimSpace(locale))
	if err != nil {
		return false
	}
	normalized := tag.String()
	for _, available := range interfaceLocales {
		if normalized == available {
			return true
		}
	}
	return false
}

type Messages struct {
	Catalog          string
	RequestPart      string
	SearchLabel      string
	Search           string
	All              string
	Filters          string
	ClearFilters     string
	SelectForRFQ     string
	SelectedProducts string
	ClearSelection   string
	MoreDetails      string
	LessDetails      string
	NoProducts       string
	NoProductsPrefix string
	RequestThisPart  string
	PreviousPage     string
	NextPage         string
	PartNumber       string
	Manufacturer     string
	Brand            string
	Category         string
	Lifecycle        string
	Applications     string
	ProductImages    string
	Description      string
	Features         string
	Specification    string
	Specifications   string
	Documents        string
	RequestQuote     string
	RFQTitle         string
	CatalogProduct   string
	RequestedPart    string
	OriginalSearch   string
	Name             string
	Email            string
	QuantityOptional string
	Notes            string
	SubmitRFQ        string
	RFQReceived      string
	Reference        string
	ReplayNotice     string
	Privacy          string
	Terms            string
	Language         string
}

var english = Messages{
	Catalog: "Catalog", RequestPart: "Request a part", SearchLabel: "Part number or name", Search: "Search", All: "All",
	Filters: "Filters", ClearFilters: "Clear filters", SelectForRFQ: "Select for RFQ", SelectedProducts: "Selected products", ClearSelection: "Clear selection", MoreDetails: "More details", LessDetails: "Less details",
	NoProducts: "No products found", NoProductsPrefix: "No published product matched", RequestThisPart: "Request this part",
	PreviousPage: "Previous page", NextPage: "Next page", PartNumber: "Part number", Manufacturer: "Manufacturer",
	Brand: "Brand", Category: "Category", Lifecycle: "Lifecycle", Applications: "Applications", ProductImages: "Product images",
	Description: "Description", Features: "Features", Specification: "Specification", Specifications: "Specifications",
	Documents: "Documents", RequestQuote: "Request a quote", RFQTitle: "Request for quotation",
	CatalogProduct: "Catalog product", RequestedPart: "Requested part", OriginalSearch: "Original search",
	Name: "Name", Email: "Email", QuantityOptional: "Quantity (optional)", Notes: "Notes", SubmitRFQ: "Submit RFQ",
	RFQReceived: "RFQ received", Reference: "Reference", ReplayNotice: "This is the original successful result; no duplicate RFQ was created.",
	Privacy: "Privacy", Terms: "Terms", Language: "Language",
}

var traditionalChinese = Messages{
	Catalog: "產品型錄", RequestPart: "提出詢價", SearchLabel: "料號或產品名稱", Search: "搜尋", All: "全部",
	Filters: "篩選條件", ClearFilters: "清除篩選", SelectForRFQ: "加入詢價選取", SelectedProducts: "已選產品", ClearSelection: "清除選取", MoreDetails: "更多資料", LessDetails: "收合資料",
	NoProducts: "找不到產品", NoProductsPrefix: "目前沒有已發布產品符合", RequestThisPart: "詢問這個料號",
	PreviousPage: "上一頁", NextPage: "下一頁", PartNumber: "料號", Manufacturer: "製造商",
	Brand: "品牌", Category: "分類", Lifecycle: "生命週期", Applications: "應用領域", ProductImages: "產品圖片",
	Description: "產品描述", Features: "特色", Specification: "規格", Specifications: "規格",
	Documents: "文件", RequestQuote: "提出詢價", RFQTitle: "詢價需求",
	CatalogProduct: "型錄產品", RequestedPart: "欲詢問料號", OriginalSearch: "原始搜尋內容",
	Name: "姓名", Email: "電子郵件", QuantityOptional: "數量（選填）", Notes: "備註", SubmitRFQ: "送出詢價",
	RFQReceived: "已收到詢價", Reference: "參考編號", ReplayNotice: "這是原先成功提交的結果，系統沒有建立重複詢價。",
	Privacy: "隱私權", Terms: "使用條款", Language: "語言",
}

func For(locale string) Messages {
	tag, err := language.Parse(strings.TrimSpace(locale))
	if err == nil {
		if messages, ok := messageCatalogs[tag.String()]; ok {
			return messages
		}
	}
	return english
}

func NormalizeSupported(defaultLocale string, supported []string) (string, []string, error) {
	defaultLocale = strings.TrimSpace(defaultLocale)
	defaultTag, err := language.Parse(defaultLocale)
	if err != nil {
		return "", nil, ErrUnsupportedLocale
	}
	defaultLocale = defaultTag.String()
	if !IsAvailable(defaultLocale) {
		return "", nil, ErrUnsupportedLocale
	}
	result := make([]string, 0, len(supported))
	seen := make(map[string]struct{}, len(supported))
	foundDefault := false
	for _, candidate := range supported {
		tag, err := language.Parse(strings.TrimSpace(candidate))
		if err != nil {
			return "", nil, ErrUnsupportedLocale
		}
		normalized := tag.String()
		if !IsAvailable(normalized) {
			return "", nil, ErrUnsupportedLocale
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
		foundDefault = foundDefault || normalized == defaultLocale
	}
	if len(result) == 0 || !foundDefault {
		return "", nil, ErrUnsupportedLocale
	}
	return defaultLocale, result, nil
}

// Resolve uses an explicit ?lang value only when it is configured for the
// site. Without one it negotiates Accept-Language, then falls back to default.
func Resolve(defaultLocale string, supported []string, explicit, acceptLanguage string) (string, error) {
	defaultLocale, supported, err := NormalizeSupported(defaultLocale, supported)
	if err != nil {
		return "", err
	}
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		tag, err := language.Parse(explicit)
		if err != nil {
			return "", ErrUnsupportedLocale
		}
		normalized := tag.String()
		for _, candidate := range supported {
			if candidate == normalized {
				return candidate, nil
			}
		}
		return "", ErrUnsupportedLocale
	}
	tags := make([]language.Tag, 0, len(supported))
	for _, candidate := range supported {
		tag, _ := language.Parse(candidate)
		tags = append(tags, tag)
	}
	matcher := language.NewMatcher(tags)
	for _, header := range strings.Split(acceptLanguage, ",") {
		value := strings.TrimSpace(strings.SplitN(header, ";", 2)[0])
		if value == "" || value == "*" {
			continue
		}
		tag, err := language.Parse(value)
		if err != nil {
			continue
		}
		_, index, confidence := matcher.Match(tag)
		if confidence != language.No {
			return supported[index], nil
		}
	}
	return defaultLocale, nil
}

func Explicit(r *http.Request) string {
	return r.URL.Query().Get("lang")
}
