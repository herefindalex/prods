package inquiries

import (
	"errors"
	"testing"
)

func TestRFQStatusTransitionContract(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusNew, StatusInProgress}:    true,
		{StatusNew, StatusSpam}:          true,
		{StatusInProgress, StatusClosed}: true,
		{StatusInProgress, StatusSpam}:   true,
		{StatusClosed, StatusInProgress}: true,
		{StatusSpam, StatusNew}:          true,
		{StatusSpam, StatusInProgress}:   true,
	}
	statuses := []Status{StatusNew, StatusInProgress, StatusClosed, StatusSpam}
	for _, from := range statuses {
		for _, to := range statuses {
			if got := CanTransition(from, to); got != allowed[[2]Status{from, to}] {
				t.Fatalf("transition %s -> %s = %v", from, to, got)
			}
		}
	}
}

func TestNormalizeRecipientsKeepsKindsSeparateAndRejectsAmbiguity(t *testing.T) {
	got, err := NormalizeRecipients([]Recipient{
		{Kind: RecipientUser, UserID: " user_one "},
		{Kind: RecipientEmail, Email: "Sales@Example.TEST"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].UserID != "user_one" || got[0].Email != "" || got[1].Email != "sales@example.test" {
		t.Fatalf("normalized recipients = %#v", got)
	}
	for _, invalid := range [][]Recipient{
		{{Kind: RecipientUser, UserID: "u", Email: "also@example.test"}},
		{{Kind: RecipientEmail, Email: "Display <mail@example.test>"}},
		{{Kind: RecipientEmail, Email: "not-an-email"}},
		{{Kind: RecipientEmail, Email: "same@example.test"}, {Kind: RecipientEmail, Email: "SAME@example.test"}},
	} {
		if _, err := NormalizeRecipients(invalid); !errors.Is(err, ErrInvalidRecipient) {
			t.Fatalf("invalid recipients %#v error = %v", invalid, err)
		}
	}
}
