package importing

import (
	"bytes"
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"
)

func workbookBytes(t *testing.T) []byte {
	t.Helper()
	file := excelize.NewFile()
	sheet := file.GetSheetName(0)
	for cell, value := range map[string]string{
		"A1": "P/N", "B1": "Name", "C1": "Maker",
		"A2": "ABC123", "B2": "Regulator", "C2": "mfr_example",
		"A3": "XYZ789", "B3": "Connector", "C3": "mfr_other",
	} {
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	var buffer bytes.Buffer
	if err := file.Write(&buffer); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestReadXLSXStreamsRowsAndReportsLimitsHonestly(t *testing.T) {
	encoded := workbookBytes(t)
	workbook, err := ReadXLSX(bytes.NewReader(encoded), "", 1, Limits{MaxBytes: int64(len(encoded) + 1), MaxRows: 10, MaxColumns: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !workbook.FullyScanned || workbook.CheckedRows != 3 || len(workbook.Rows) != 2 || workbook.Rows[1].Cells[0] != "XYZ789" {
		t.Fatalf("workbook = %+v", workbook)
	}
	limited, err := ReadXLSX(bytes.NewReader(encoded), "", 1, Limits{MaxBytes: int64(len(encoded) + 1), MaxRows: 1, MaxColumns: 10})
	if !errors.Is(err, ErrResourceLimit) || limited.FullyScanned || limited.CheckedRows != 3 {
		t.Fatalf("limited workbook = %+v, err=%v", limited, err)
	}
	corrupt, err := ReadXLSX(bytes.NewReader([]byte("not an xlsx")), "", 1, Limits{})
	if !errors.Is(err, ErrInvalidWorkbook) || corrupt.FullyScanned {
		t.Fatalf("corrupt workbook = %+v, err=%v", corrupt, err)
	}
}

func TestFinalMappingReportsEveryConflictingSourceAndRequiresPartNumber(t *testing.T) {
	headers := []string{"P/N", "Model No.", "Name"}
	issues := ValidateFinalMapping(headers, []ColumnMapping{
		{SourceIndex: 0, Target: TargetPartNumber},
		{SourceIndex: 1, Target: TargetPartNumber},
		{SourceIndex: 2, Target: TargetProductName},
	})
	if len(issues) != 1 || len(issues[0].SourceIndexes) != 2 || issues[0].SourceNames[0] != "P/N" || issues[0].SourceNames[1] != "Model No." {
		t.Fatalf("mapping issues = %+v", issues)
	}
	issues = ValidateFinalMapping(headers, []ColumnMapping{{SourceIndex: 2, Target: TargetProductName}})
	if len(issues) != 1 || issues[0].Target != TargetPartNumber {
		t.Fatalf("missing Part Number issues = %+v", issues)
	}
}
