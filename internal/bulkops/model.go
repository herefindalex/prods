package bulkops

import "errors"

var (
	ErrInvalidRequest = errors.New("invalid Product Bulk request")
	ErrRunConflict    = errors.New("Product Bulk run changed or is not executable")
)

type Action string

const (
	ActionPublish         Action = "publish"
	ActionHide            Action = "hide"
	ActionArchive         Action = "archive"
	ActionChangeCategory  Action = "change_category"
	ActionChangeLifecycle Action = "change_lifecycle"
)

func (action Action) Valid() bool {
	switch action {
	case ActionPublish, ActionHide, ActionArchive, ActionChangeCategory, ActionChangeLifecycle:
		return true
	default:
		return false
	}
}

type Request struct {
	ProductIDs []string `json:"product_ids"`
	Action     Action   `json:"action"`
	TargetID   string   `json:"target_id,omitempty"`
}

type PlanItem struct {
	SelectionIndex   int    `json:"selection_index"`
	ProductID        string `json:"product_id"`
	PartNumber       string `json:"part_number"`
	ExpectedRevision int64  `json:"expected_revision"`
	RecordState      string `json:"record_state"`
	PublishingState  string `json:"publishing_state"`
	Eligible         bool   `json:"eligible"`
	Message          string `json:"message,omitempty"`
}

type Preview struct {
	RunID          string     `json:"run_id"`
	Action         Action     `json:"action"`
	TargetID       string     `json:"target_id,omitempty"`
	SelectionCount int        `json:"selection_count"`
	EligibleCount  int        `json:"eligible_count"`
	Warning        string     `json:"warning,omitempty"`
	Items          []PlanItem `json:"items"`
	CreatedAt      string     `json:"created_at"`
}

type ItemStatus string

const (
	ItemPrepared  ItemStatus = "prepared"
	ItemSucceeded ItemStatus = "succeeded"
	ItemNoChange  ItemStatus = "no_change"
	ItemConflict  ItemStatus = "conflict"
	ItemInvalid   ItemStatus = "invalid"
	ItemFailed    ItemStatus = "failed"
)

type ItemResult struct {
	SelectionIndex   int        `json:"selection_index"`
	ProductID        string     `json:"product_id"`
	PartNumber       string     `json:"part_number"`
	Status           ItemStatus `json:"status"`
	ExpectedRevision int64      `json:"expected_revision"`
	ResultRevision   int64      `json:"result_revision,omitempty"`
	Message          string     `json:"message,omitempty"`
	PublicState      string     `json:"public_state,omitempty"`
}

type Receipt struct {
	RunID     string       `json:"run_id"`
	Succeeded int          `json:"succeeded"`
	NoChange  int          `json:"no_change"`
	Conflicts int          `json:"conflicts"`
	Invalid   int          `json:"invalid"`
	Failed    int          `json:"failed"`
	Replay    bool         `json:"replay"`
	Results   []ItemResult `json:"results"`
}

type Run struct {
	Preview Preview  `json:"preview"`
	Status  string   `json:"status"`
	Receipt *Receipt `json:"receipt,omitempty"`
}
