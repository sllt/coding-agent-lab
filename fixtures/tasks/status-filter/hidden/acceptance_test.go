package filter

import "testing"

func TestAcceptanceFilterByStatus(t *testing.T) {
	got := FilterByStatus([]Order{{ID: "1", Status: "open"}, {ID: "2", Status: "paid"}}, "open")
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("%+v", got)
	}
}
