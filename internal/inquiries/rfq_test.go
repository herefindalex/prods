package inquiries

import "testing"

func TestCanonicalHashStableAndMeaningful(t *testing.T) {
	first := Submission{Name: " Ada ", Email: " ada@example.test ", Items: []Item{{Kind: "requested", Requested: " X-1 ", RawQuery: "X-1"}}}
	second := Submission{Email: "ada@example.test", Name: "Ada", Items: []Item{{RawQuery: "X-1", Requested: "X-1", Kind: "requested"}}}
	firstHash, err := first.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := second.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("equivalent submission models differ: %s != %s", firstHash, secondHash)
	}
	second.Items[0].Requested = "X-2"
	changedHash, err := second.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == firstHash {
		t.Fatal("meaningful edit must change canonical hash")
	}
}
