package orders

import "testing"

func TestRegressionEmptyAndSingle(t *testing.T) {
	page, next, err := Page(nil, "", 0)
	if err != nil || len(page) != 0 || next != "" {
		t.Fatalf("empty %+v %q %v", page, next, err)
	}
	page, _, err = Page([]Order{{ID: "a", CreatedAt: 10}}, "", 20)
	if err != nil || len(page) != 1 || page[0].ID != "a" {
		t.Fatalf("single %+v %v", page, err)
	}
}
