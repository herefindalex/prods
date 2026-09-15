package webapp

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/site"
)

const (
	maxPublicCopyExchangeBytes    = 8 << 20
	maxPublicCopyExchangeUnzip    = 64 << 20
	maxPublicCopyExchangeXMLUnzip = 32 << 20
)

type publicCopyExchangeRow struct {
	Line              int    `json:"line"`
	Key               string `json:"key"`
	Locale            string `json:"locale"`
	Value             string `json:"value"`
	Action            string `json:"action"`
	DefinitionVersion int64  `json:"definition_version"`
	OfficialBundle    string `json:"official_bundle_version"`
}

type publicCopyExchangeIssue struct {
	Line    int    `json:"line"`
	Key     string `json:"key,omitempty"`
	Locale  string `json:"locale,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type publicCopyExchangeChange struct {
	Line   int    `json:"line"`
	Key    string `json:"key"`
	Locale string `json:"locale"`
	Action string `json:"action"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type publicCopyExchangePreview struct {
	WorkingRevision int64                      `json:"working_revision"`
	FullyValidated  bool                       `json:"fully_validated"`
	Rows            []publicCopyExchangeRow    `json:"rows"`
	Issues          []publicCopyExchangeIssue  `json:"issues"`
	Changes         []publicCopyExchangeChange `json:"changes"`
}

func (s *Server) adminExportPublicCopy(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.PathValue("format")))
	if format != "csv" && format != "xlsx" {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeValidationFailed)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	catalogValue, err := s.store.PublicCopyCatalog(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	locales := selectedPublicCopyLocales(r.URL.Query().Get("locales"), state.WorkingLocalization.EnabledLocales)
	if len(locales) == 0 {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	contentType := "text/csv; charset=utf-8"
	filename := "prods-public-copy.csv"
	if format == "xlsx" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		filename = "prods-public-copy.xlsx"
	}
	s.generateExport(w, r, current.UserID, "public_copy", format, filename, contentType, func(path string) (int, error) {
		return writePublicCopyExchange(path, format, scope, locales, state.WorkingLocalization, catalogValue)
	})
}

func selectedPublicCopyLocales(value string, enabled []string) []string {
	allowed := make(map[string]bool, len(enabled))
	for _, locale := range enabled {
		allowed[locale] = true
	}
	if strings.TrimSpace(value) == "" {
		return append([]string(nil), enabled...)
	}
	var result []string
	seen := make(map[string]bool)
	for _, item := range strings.Split(value, ",") {
		locale, ok := localization.NormalizeBuiltinLocale(item)
		if ok && allowed[locale] && !seen[locale] {
			seen[locale] = true
			result = append(result, locale)
		}
	}
	return result
}

func writePublicCopyExchange(path, format, scope string, locales []string, settings site.WebsiteLocalization, catalogValue localization.PublicCopyCatalog) (int, error) {
	definitions := make([]localization.PublicCopyDefinition, 0, len(catalogValue.Definitions))
	for _, definition := range catalogValue.Definitions {
		if scope == "" || strings.HasPrefix(definition.Key, scope+".") {
			definitions = append(definitions, definition)
		}
	}
	headers := []string{"Key", "Locale", "Value", "Action", "Definition Version", "Official Bundle Version"}
	rows := make([][]any, 0, len(definitions)*len(locales)+1)
	rows = append(rows, stringsToAny(headers))
	for _, definition := range definitions {
		for _, locale := range locales {
			value := publicCopyDefaultValue(catalogValue.Defaults, definition.Key, locale)
			if override := settings.PublicCopyOverrides[definition.Key][locale]; override.Value != "" {
				value = override.Value
			}
			rows = append(rows, []any{definition.Key, locale, value, "upsert", definition.DefinitionVersion, catalogValue.OfficialBundle})
		}
	}
	if format == "csv" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		complete := false
		defer func() {
			_ = file.Close()
			if !complete {
				_ = os.Remove(path)
			}
		}()
		if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return 0, err
		}
		writer := csv.NewWriter(file)
		for _, row := range rows {
			values := make([]string, len(row))
			for index := range row {
				values[index] = fmt.Sprint(row[index])
			}
			if err := writer.Write(values); err != nil {
				return 0, err
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return 0, err
		}
		if err := file.Sync(); err != nil {
			return 0, err
		}
		complete = true
		return len(rows) - 1, file.Close()
	}
	workbook := excelize.NewFile()
	defer workbook.Close()
	workbook.SetSheetName("Sheet1", "Public Copy")
	stream, err := workbook.NewStreamWriter("Public Copy")
	if err != nil {
		return 0, err
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := stream.SetRow(cell, row); err != nil {
			return 0, err
		}
	}
	if err := stream.Flush(); err != nil {
		return 0, err
	}
	if err := workbook.SaveAs(path); err != nil {
		return 0, err
	}
	return len(rows) - 1, nil
}

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for index := range values {
		result[index] = values[index]
	}
	return result
}

func publicCopyDefaultValue(defaults []localization.PublicCopyDefault, key, locale string) string {
	for _, item := range defaults {
		if item.Key == key && item.Locale == locale {
			return item.Value
		}
	}
	return ""
}

func (s *Server) adminPreviewPublicCopyImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true); !ok {
		return
	}
	rows, err := readPublicCopyExchangeUpload(w, r)
	if err != nil {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	preview, err := s.validatePublicCopyExchange(r, rows)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) adminCommitPublicCopyImport(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	reservation, err := s.admitResource(r.Context(), "Public Copy translation import commit", s.config.DatabasePath, 2<<20, 2)
	if err != nil {
		s.writeResourceError(w, r, err)
		return
	}
	defer reservation.Release()
	var request struct {
		ExpectedWorkingRevision int64                   `json:"expected_working_revision"`
		Rows                    []publicCopyExchangeRow `json:"rows"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	preview, err := s.validatePublicCopyExchange(r, request.Rows)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	if request.ExpectedWorkingRevision != preview.WorkingRevision {
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
		return
	}
	if !preview.FullyValidated {
		writeJSON(w, http.StatusUnprocessableEntity, preview)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	settings := cloneWebsiteLocalization(state.WorkingLocalization)
	for _, row := range preview.Rows {
		if row.Action == "reset" {
			delete(settings.PublicCopyOverrides[row.Key], row.Locale)
			if len(settings.PublicCopyOverrides[row.Key]) == 0 {
				delete(settings.PublicCopyOverrides, row.Key)
			}
			continue
		}
		if settings.PublicCopyOverrides[row.Key] == nil {
			settings.PublicCopyOverrides[row.Key] = make(map[string]localization.PublicCopyOverride)
		}
		settings.PublicCopyOverrides[row.Key][row.Locale] = localization.PublicCopyOverride{
			Key: row.Key, Locale: row.Locale, Value: row.Value, DefinitionVersion: row.DefinitionVersion,
		}
	}
	updated, err := s.store.SaveWebsiteLocalization(r.Context(), current.UserID, request.ExpectedWorkingRevision, settings)
	if err != nil {
		s.writePublicCopyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) validatePublicCopyExchange(r *http.Request, rows []publicCopyExchangeRow) (publicCopyExchangePreview, error) {
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		return publicCopyExchangePreview{}, err
	}
	catalogValue, err := s.store.PublicCopyCatalog(r.Context())
	if err != nil {
		return publicCopyExchangePreview{}, err
	}
	preview := publicCopyExchangePreview{WorkingRevision: state.WorkingRevision, Rows: rows}
	definitions := make(map[string]localization.PublicCopyDefinition, len(catalogValue.Definitions))
	for _, definition := range catalogValue.Definitions {
		definitions[definition.Key] = definition
	}
	enabled := make(map[string]bool, len(state.WorkingLocalization.EnabledLocales))
	for _, locale := range state.WorkingLocalization.EnabledLocales {
		enabled[locale] = true
	}
	duplicates := make(map[string][]int)
	for index := range preview.Rows {
		row := &preview.Rows[index]
		row.Key = strings.TrimSpace(row.Key)
		row.Locale = strings.TrimSpace(row.Locale)
		row.Action = strings.ToLower(strings.TrimSpace(row.Action))
		row.OfficialBundle = strings.TrimSpace(row.OfficialBundle)
		duplicates[row.Key+"\x00"+row.Locale] = append(duplicates[row.Key+"\x00"+row.Locale], row.Line)
	}
	for _, row := range preview.Rows {
		definition, exists := definitions[row.Key]
		if !exists {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "unknown_key", "unknown or protected Public Copy key"))
			continue
		}
		if normalized, ok := localization.NormalizeBuiltinLocale(row.Locale); !ok || normalized != row.Locale {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "unknown_locale", "locale is not a canonical built-in locale"))
			continue
		}
		if !enabled[row.Locale] {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "disabled_locale", "locale is not enabled in the Website working copy"))
		}
		if row.DefinitionVersion != definition.DefinitionVersion || row.OfficialBundle != catalogValue.OfficialBundle {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "resource_version_mismatch", "definition or official bundle version changed; export a fresh file"))
		}
		if len(duplicates[row.Key+"\x00"+row.Locale]) > 1 {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "duplicate_entry", fmt.Sprintf("duplicate key and locale at lines %v", duplicates[row.Key+"\x00"+row.Locale])))
		}
		if row.Action != "upsert" && row.Action != "reset" {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "invalid_action", "Action must be upsert or reset"))
			continue
		}
		if row.Action == "reset" {
			if strings.TrimSpace(row.Value) != "" {
				preview.Issues = append(preview.Issues, exchangeIssue(row, "reset_value_present", "reset rows must leave Value blank"))
			}
		} else if err := localization.ValidatePublicCopyValue(definition, row.Value); err != nil {
			preview.Issues = append(preview.Issues, exchangeIssue(row, "invalid_value", "value violates markup, placeholder, plural, or select contract"))
		}
		before := publicCopyDefaultValue(catalogValue.Defaults, row.Key, row.Locale)
		if override := state.WorkingLocalization.PublicCopyOverrides[row.Key][row.Locale]; override.Value != "" {
			before = override.Value
		}
		after := row.Value
		if row.Action == "reset" {
			after = publicCopyDefaultValue(catalogValue.Defaults, row.Key, row.Locale)
		}
		preview.Changes = append(preview.Changes, publicCopyExchangeChange{
			Line: row.Line, Key: row.Key, Locale: row.Locale, Action: row.Action, Before: before, After: after,
		})
	}
	preview.FullyValidated = len(rows) > 0 && len(preview.Issues) == 0
	return preview, nil
}

func exchangeIssue(row publicCopyExchangeRow, code, message string) publicCopyExchangeIssue {
	return publicCopyExchangeIssue{Line: row.Line, Key: row.Key, Locale: row.Locale, Code: code, Message: message}
}

func readPublicCopyExchangeUpload(w http.ResponseWriter, r *http.Request) ([]publicCopyExchangeRow, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPublicCopyExchangeBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	var filename string
	var data []byte
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" && data == nil {
			filename = filepath.Base(part.FileName())
			data, err = io.ReadAll(io.LimitReader(part, maxPublicCopyExchangeBytes+1))
			part.Close()
			if err != nil || len(data) > maxPublicCopyExchangeBytes {
				return nil, errors.New("Public Copy exchange file exceeds limit")
			}
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 64<<10))
			part.Close()
		}
	}
	if len(data) == 0 {
		return nil, errors.New("Public Copy exchange file is required")
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension == ".csv" {
		return parsePublicCopyCSV(data)
	}
	if extension == ".xlsx" {
		return parsePublicCopyXLSX(data)
	}
	return nil, errors.New("only CSV or XLSX Public Copy exchange files are supported")
}

func parsePublicCopyCSV(data []byte) ([]publicCopyExchangeRow, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return parsePublicCopyRecords(records)
}

func parsePublicCopyXLSX(data []byte) ([]publicCopyExchangeRow, error) {
	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		RawCellValue:      true,
		UnzipSizeLimit:    maxPublicCopyExchangeUnzip,
		UnzipXMLSizeLimit: maxPublicCopyExchangeXMLUnzip,
	})
	if err != nil {
		return nil, err
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("XLSX has no worksheet")
	}
	iterator, err := workbook.Rows(sheets[0])
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	var records [][]string
	for iterator.Next() {
		row, err := iterator.Columns()
		if err != nil {
			return nil, err
		}
		records = append(records, row)
		if len(records) > 10001 {
			return nil, errors.New("Public Copy exchange exceeds 10000 rows")
		}
	}
	if err := iterator.Error(); err != nil {
		return nil, err
	}
	return parsePublicCopyRecords(records)
}

func parsePublicCopyRecords(records [][]string) ([]publicCopyExchangeRow, error) {
	if len(records) < 2 {
		return nil, errors.New("Public Copy exchange requires a header and data rows")
	}
	headers := make(map[string]int)
	for index, value := range records[0] {
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", "_"))
		headers[normalized] = index
	}
	required := []string{"key", "locale", "value", "action", "definition_version", "official_bundle_version"}
	for _, name := range required {
		if _, exists := headers[name]; !exists {
			return nil, fmt.Errorf("required column %s is missing", name)
		}
	}
	cell := func(record []string, name string) string {
		index := headers[name]
		if index >= len(record) {
			return ""
		}
		return record[index]
	}
	rows := make([]publicCopyExchangeRow, 0, len(records)-1)
	for index, record := range records[1:] {
		if len(record) == 0 || strings.TrimSpace(strings.Join(record, "")) == "" {
			continue
		}
		version, _ := strconv.ParseInt(strings.TrimSpace(cell(record, "definition_version")), 10, 64)
		rows = append(rows, publicCopyExchangeRow{
			Line: index + 2, Key: cell(record, "key"), Locale: cell(record, "locale"), Value: cell(record, "value"),
			Action: cell(record, "action"), DefinitionVersion: version, OfficialBundle: cell(record, "official_bundle_version"),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Line < rows[j].Line })
	return rows, nil
}
