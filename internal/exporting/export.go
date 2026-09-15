package exporting

import (
	"context"
	"encoding/csv"
	"os"
	"strconv"
	"strings"

	"prods/internal/storage/sqlite"

	"github.com/xuri/excelize/v2"
)

type Store interface {
	ForEachProductExport(context.Context, string, func(sqlite.ProductExportRow) error, func(sqlite.RawSpecExportRow) error) error
	ForEachRFQExport(context.Context, string, func(sqlite.RFQExportRow) error) error
}

func ProductsXLSX(ctx context.Context, store Store, actorID, path string) (int, error) {
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "Products"); err != nil {
		return 0, err
	}
	if _, err := workbook.NewSheet("Raw Specs"); err != nil {
		return 0, err
	}
	products, err := workbook.NewStreamWriter("Products")
	if err != nil {
		return 0, err
	}
	specs, err := workbook.NewStreamWriter("Raw Specs")
	if err != nil {
		return 0, err
	}
	productHeaders := []any{"ID", "Part Number", "Manufacturer", "Brand", "Name", "Category ID", "Lifecycle",
		"Package / Form Factor", "Description", "Features", "Specification", "Document URL", "Record State", "Public Status",
		"Revision", "Created At", "Updated At"}
	specHeaders := []any{"Product ID", "Spec ID", "Spec Name", "Raw Value", "Source Locale", "Source Revision", "Active", "Updated At"}
	if err := products.SetRow("A1", productHeaders); err != nil {
		return 0, err
	}
	if err := specs.SetRow("A1", specHeaders); err != nil {
		return 0, err
	}
	productRow, specRow, totalRows := 2, 2, 0
	err = store.ForEachProductExport(ctx, actorID, func(row sqlite.ProductExportRow) error {
		cell, err := excelize.CoordinatesToCellName(1, productRow)
		if err != nil {
			return err
		}
		productRow++
		totalRows++
		return products.SetRow(cell, []any{row.ID, row.PartNumber, row.Manufacturer, row.Brand, row.Name, row.CategoryID,
			row.Lifecycle, row.PackageFormFactor, row.Description, row.Features, row.Specification, row.DocumentURL,
			row.RecordState, row.Status, row.Revision, row.CreatedAt, row.UpdatedAt})
	}, func(row sqlite.RawSpecExportRow) error {
		cell, err := excelize.CoordinatesToCellName(1, specRow)
		if err != nil {
			return err
		}
		specRow++
		totalRows++
		return specs.SetRow(cell, []any{row.ProductID, row.SpecID, row.SpecName, row.RawValue, row.SourceLocale,
			row.SourceRevision, row.Active, row.UpdatedAt})
	})
	if err != nil {
		return 0, err
	}
	if err := products.Flush(); err != nil {
		return 0, err
	}
	if err := specs.Flush(); err != nil {
		return 0, err
	}
	if err := workbook.SaveAs(path); err != nil {
		return 0, err
	}
	return totalRows, nil
}

func RFQsCSV(ctx context.Context, store Store, actorID, path string) (int, error) {
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
	if err := writer.Write([]string{"RFQ ID", "Status", "Revision", "Name", "Email", "Company", "Phone", "Country/Region",
		"General Message", "Recipients", "Privacy State", "Privacy Processed At", "Created At", "Updated At", "Item Index", "Item Kind", "Product ID", "Requested Part",
		"Raw Query", "Quantity", "Notes", "Public Snapshot JSON"}); err != nil {
		return 0, err
	}
	rows := 0
	err = store.ForEachRFQExport(ctx, actorID, func(row sqlite.RFQExportRow) error {
		rows++
		return writer.Write([]string{
			csvCell(row.RFQID), csvCell(row.Status), strconv.FormatInt(row.Revision, 10), csvCell(row.Name), csvCell(row.Email),
			csvCell(row.Company), csvCell(row.Phone), csvCell(row.Country), csvCell(row.GeneralMessage), csvCell(row.Recipients),
			csvCell(row.PrivacyState), csvCell(row.PrivacyAt), csvCell(row.CreatedAt), csvCell(row.UpdatedAt), strconv.Itoa(row.ItemIndex), csvCell(row.ItemKind), csvCell(row.ProductID),
			csvCell(row.Requested), csvCell(row.RawQuery), csvCell(row.Quantity), csvCell(row.Notes), csvCell(row.PublicSnapshot),
		})
	})
	if err != nil {
		return 0, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	complete = true
	return rows, nil
}

func csvCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
