package importing

import (
	"errors"

	"prods/internal/catalog"
)

var (
	ErrInvalidImport         = errors.New("import preview is not valid for commit")
	ErrImportConflict        = errors.New("import preview no longer matches current data")
	ErrImportReceiptConflict = errors.New("import operation id was already used for a different plan")
)

type IdentityMode string

const (
	IdentityPartNumber IdentityMode = "part_number"
	IdentityComposite  IdentityMode = "manufacturer_part_number"
)

type RowIssue struct {
	Sheet   string `json:"sheet"`
	Row     int    `json:"row"`
	Column  string `json:"column,omitempty"`
	Value   string `json:"value,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Action string

const (
	ActionCreate   Action = "create"
	ActionUpdate   Action = "update"
	ActionNoChange Action = "no_change"
)

type PlanItem struct {
	Sheet            string          `json:"sheet"`
	Row              int             `json:"row"`
	Action           Action          `json:"action"`
	Product          catalog.Product `json:"product"`
	ExistingID       string          `json:"existing_id,omitempty"`
	ExpectedRevision int64           `json:"expected_revision,omitempty"`
}

type Preview struct {
	OperationID    string          `json:"operation_id"`
	IdentityMode   IdentityMode    `json:"identity_mode"`
	Mappings       []ColumnMapping `json:"mappings"`
	FullyScanned   bool            `json:"fully_scanned"`
	FullyValidated bool            `json:"fully_validated"`
	CheckedRows    int             `json:"checked_rows"`
	TotalRows      int             `json:"total_rows"`
	CreateCount    int             `json:"create_count"`
	UpdateCount    int             `json:"update_count"`
	NoChangeCount  int             `json:"no_change_count"`
	Items          []PlanItem      `json:"items"`
	Issues         []RowIssue      `json:"issues"`
}

type Receipt struct {
	OperationID string `json:"operation_id"`
	Created     int    `json:"created"`
	Updated     int    `json:"updated"`
	NoChange    int    `json:"no_change"`
	Replay      bool   `json:"replay"`
}

type TemplateSnapshot struct {
	Sheet                string          `json:"sheet,omitempty"`
	HeaderRow            int             `json:"header_row"`
	IdentityMode         IdentityMode    `json:"identity_mode"`
	SourceLocaleOverride string          `json:"source_locale_override,omitempty"`
	Mappings             []ColumnMapping `json:"mappings"`
}

type Template struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Version  int64            `json:"version"`
	Snapshot TemplateSnapshot `json:"snapshot"`
}

type JobStatus string

const (
	JobQueued       JobStatus = "queued"
	JobParsing      JobStatus = "parsing"
	JobPreviewReady JobStatus = "preview_ready"
	JobCommitting   JobStatus = "committing"
	JobCommitted    JobStatus = "committed"
	JobFailed       JobStatus = "failed"
	JobCancelled    JobStatus = "cancelled"
	JobInterrupted  JobStatus = "interrupted"
)

type Job struct {
	ID               string           `json:"id"`
	ActorID          string           `json:"actor_id"`
	Status           JobStatus        `json:"status"`
	Phase            string           `json:"phase"`
	OriginalFilename string           `json:"original_filename"`
	Checksum         string           `json:"checksum"`
	TemplateSnapshot TemplateSnapshot `json:"template_snapshot"`
	CheckedRows      int              `json:"checked_rows"`
	TotalRows        int              `json:"total_rows"`
	CreateCount      int              `json:"create_count"`
	UpdateCount      int              `json:"update_count"`
	NoChangeCount    int              `json:"no_change_count"`
	FailedCount      int              `json:"failed_count"`
	PreviewPath      string           `json:"-"`
	ReportPath       string           `json:"-"`
	ErrorMessage     string           `json:"error_message,omitempty"`
	CancelRequested  bool             `json:"cancel_requested"`
	CreatedAt        string           `json:"created_at"`
	UpdatedAt        string           `json:"updated_at"`
}
