package searchtext

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Fold is the pinned NFC/full-case-fold rule shared by keyword and field
// queries. It intentionally preserves diacritics and literal wildcard text.
func Fold(value string) string { return norm.NFC.String(cases.Fold().String(norm.NFC.String(value))) }
