package harbor

import "testing"

func TestImportListsUnsupportedAndDoesNotMerge(t *testing.T) {
	raw := []byte(`{"schema_version":"harbor.subset/v1","name":"分页","instruction":"修游标","environment":{"image":"sha256:abc"},"verifier":{"hidden":true}}`)
	got, err := Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoMerge || got.Name != "分页" || got.Prompt != "修游标" {
		t.Fatalf("%+v", got)
	}
	if len(got.UnsupportedFields) != 2 {
		t.Fatalf("%v", got.UnsupportedFields)
	}
}
