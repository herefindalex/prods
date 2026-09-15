package importing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

var (
	ErrInvalidWorkbook = errors.New("invalid xlsx workbook")
	ErrResourceLimit   = errors.New("xlsx import exceeds configured resource limit")
	ErrInvalidMapping  = errors.New("invalid import mapping")
)

type Target string

const (
	TargetPartNumber          Target = "part_number"
	TargetProductName         Target = "product_name"
	TargetManufacturerID      Target = "manufacturer_id"
	TargetBrandID             Target = "brand_id"
	TargetCategoryID          Target = "category_id"
	TargetPackageFormFactor   Target = "package_form_factor"
	TargetDescription         Target = "description"
	TargetFeatures            Target = "features"
	TargetSpecification       Target = "specification"
	TargetLifecycleID         Target = "lifecycle_id"
	TargetApplicationIDs      Target = "application_ids"
	TargetSourceLocale        Target = "source_locale"
	TargetNameSourceLocale    Target = "name_source_locale"
	TargetDescriptionLocale   Target = "description_source_locale"
	TargetFeaturesLocale      Target = "features_source_locale"
	TargetSpecificationLocale Target = "specification_source_locale"
)

type Limits struct {
	MaxBytes   int64
	MaxRows    int
	MaxColumns int
}

func (l Limits) normalized() Limits {
	if l.MaxBytes <= 0 {
		l.MaxBytes = 32 << 20
	}
	if l.MaxRows <= 0 {
		l.MaxRows = 25_000
	}
	if l.MaxColumns <= 0 {
		l.MaxColumns = 256
	}
	return l
}

type Workbook struct {
	Sheet        string
	HeaderRow    int
	Headers      []string
	Rows         []Row
	FullyScanned bool
	CheckedRows  int
}

type Row struct {
	Number int
	Cells  []string
}

func ReadXLSX(input io.Reader, sheet string, headerRow int, limits Limits) (Workbook, error) {
	return ReadXLSXContext(context.Background(), input, sheet, headerRow, limits)
}

func ReadXLSXContext(ctx context.Context, input io.Reader, sheet string, headerRow int, limits Limits) (Workbook, error) {
	limits = limits.normalized()
	if headerRow < 1 {
		headerRow = 1
	}
	limited := &io.LimitedReader{R: input, N: limits.MaxBytes + 1}
	file, err := excelize.OpenReader(limited, excelize.Options{RawCellValue: true})
	if err != nil {
		return Workbook{Sheet: sheet, HeaderRow: headerRow, FullyScanned: false}, fmt.Errorf("%w: %v", ErrInvalidWorkbook, err)
	}
	defer file.Close()
	if limited.N <= 0 {
		return Workbook{Sheet: sheet, HeaderRow: headerRow, FullyScanned: false}, ErrResourceLimit
	}
	if sheet == "" {
		sheets := file.GetSheetList()
		if len(sheets) == 0 {
			return Workbook{HeaderRow: headerRow, FullyScanned: false}, ErrInvalidWorkbook
		}
		sheet = sheets[0]
	}
	rows, err := file.Rows(sheet)
	if err != nil {
		return Workbook{Sheet: sheet, HeaderRow: headerRow, FullyScanned: false}, fmt.Errorf("%w: %v", ErrInvalidWorkbook, err)
	}
	defer rows.Close()
	result := Workbook{Sheet: sheet, HeaderRow: headerRow}
	rowNumber := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			result.CheckedRows = rowNumber
			return result, err
		}
		rowNumber++
		cells, err := rows.Columns()
		if err != nil {
			result.CheckedRows = rowNumber - 1
			return result, fmt.Errorf("%w after row %d: %v", ErrInvalidWorkbook, rowNumber-1, err)
		}
		if len(cells) > limits.MaxColumns {
			result.CheckedRows = rowNumber
			return result, fmt.Errorf("%w: row %d has %d columns (limit %d)", ErrResourceLimit, rowNumber, len(cells), limits.MaxColumns)
		}
		if rowNumber < headerRow {
			continue
		}
		if rowNumber == headerRow {
			result.Headers = append([]string(nil), cells...)
			continue
		}
		if len(result.Rows) >= limits.MaxRows {
			result.CheckedRows = rowNumber
			return result, fmt.Errorf("%w: more than %d data rows", ErrResourceLimit, limits.MaxRows)
		}
		result.Rows = append(result.Rows, Row{Number: rowNumber, Cells: append([]string(nil), cells...)})
		result.CheckedRows = rowNumber
	}
	if err := rows.Error(); err != nil {
		return result, fmt.Errorf("%w after row %d: %v", ErrInvalidWorkbook, result.CheckedRows, err)
	}
	if len(result.Headers) == 0 {
		return result, ErrInvalidWorkbook
	}
	result.FullyScanned = true
	return result, nil
}

type ColumnMapping struct {
	SourceIndex int    `json:"source_index"`
	SourceName  string `json:"source_name"`
	Target      Target `json:"target"`
}

type MappingIssue struct {
	Target        Target   `json:"target"`
	SourceIndexes []int    `json:"source_indexes"`
	SourceNames   []string `json:"source_names"`
	Message       string   `json:"message"`
}

func ValidateFinalMapping(headers []string, mappings []ColumnMapping) []MappingIssue {
	byTarget := make(map[Target][]ColumnMapping)
	for _, mapping := range mappings {
		if mapping.Target == "" {
			continue
		}
		if mapping.SourceIndex < 0 || mapping.SourceIndex >= len(headers) || !validTarget(mapping.Target) {
			byTarget[mapping.Target] = append(byTarget[mapping.Target], mapping)
			continue
		}
		mapping.SourceName = headers[mapping.SourceIndex]
		byTarget[mapping.Target] = append(byTarget[mapping.Target], mapping)
	}
	var issues []MappingIssue
	for target, sources := range byTarget {
		invalid := target == "" || !validTarget(target)
		if len(sources) < 2 && !invalid {
			continue
		}
		issue := MappingIssue{Target: target}
		for _, source := range sources {
			issue.SourceIndexes = append(issue.SourceIndexes, source.SourceIndex)
			name := source.SourceName
			if source.SourceIndex >= 0 && source.SourceIndex < len(headers) {
				name = headers[source.SourceIndex]
			}
			issue.SourceNames = append(issue.SourceNames, name)
		}
		if invalid {
			issue.Message = "mapping contains an unknown target or source column"
		} else {
			issue.Message = fmt.Sprintf("target %q is mapped by multiple source columns: %s", target, strings.Join(issue.SourceNames, ", "))
		}
		issues = append(issues, issue)
	}
	if _, exists := byTarget[TargetPartNumber]; !exists {
		issues = append(issues, MappingIssue{Target: TargetPartNumber, Message: "Part Number mapping is required"})
	}
	return issues
}

func validTarget(target Target) bool {
	switch target {
	case TargetPartNumber, TargetProductName, TargetManufacturerID, TargetBrandID, TargetCategoryID,
		TargetPackageFormFactor, TargetDescription, TargetFeatures, TargetSpecification, TargetLifecycleID, TargetApplicationIDs,
		TargetSourceLocale, TargetNameSourceLocale, TargetDescriptionLocale, TargetFeaturesLocale, TargetSpecificationLocale:
		return true
	default:
		return false
	}
}

// ReferenceIDs parses a multi-value spreadsheet cell. Comma, semicolon, and
// line breaks are accepted so a workbook does not have to encode JSON.
func ReferenceIDs(value string) []string {
	fields := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n' || character == '\r'
	})
	result := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}

func MappedRow(row Row, mappings []ColumnMapping) map[Target]string {
	result := make(map[Target]string, len(mappings))
	for _, mapping := range mappings {
		if mapping.Target == "" || mapping.SourceIndex < 0 || mapping.SourceIndex >= len(row.Cells) {
			continue
		}
		result[mapping.Target] = row.Cells[mapping.SourceIndex]
	}
	return result
}
