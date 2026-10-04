package orders

import "testing"

func TestAcceptanceTieBreakAndInvalidCursor(t *testing.T) {
	all := []Order{
		{ID: "b", CreatedAt: 10},
		{ID: "a", CreatedAt: 10},
		{ID: "c", CreatedAt: 9},
	}
	first, cursor, err := Page(all, "", 1)
	if err != nil || len(first) != 1 || first[0].ID != "b" {
		t.Fatalf("first page %+v %q %v", first, cursor, err)
	}
	second, _, err := Page(all, cursor, 1)
	if err != nil || len(second) != 1 || second[0].ID != "a" {
		t.Fatalf("second page %+v cursor %q %v", second, cursor, err)
	}
	if _, _, err := Page(all, "not-a-cursor", 1); err == nil {
		t.Fatal("invalid cursor returned success")
	}
	page, _, err := Page(all, "", 1000)
	if err != nil || len(page) != 3 {
		t.Fatalf("cap %+v %v", page, err)
	}
}
