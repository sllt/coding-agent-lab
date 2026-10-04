package filter

import "testing"

func TestRegressionListKeepsEveryRow(t *testing.T) {
	got := List([]Order{{ID: "1", Status: "open"}, {ID: "2", Status: "paid"}})
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
}
