package httpapi

import "testing"

func TestLoopbackNotifyKeepsForeignURL(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"notify_url":"file:///tmp/hook"}`, "file:///tmp/hook"},
		{`{"notify_url":"https://example.com/hook"}`, "https://example.com/hook"},
		{`{"notify_url":"http://127.0.0.1:43117/hook"}`, "http://127.0.0.1:43117/hook"},
		{`{"notify_url":"http://localhost:43117/hook"}`, "http://localhost:43117/hook"},
		{`{}`, "local"},
	}
	for _, tc := range cases {
		if got := loopbackNotify([]byte(tc.body)); got != tc.want {
			t.Fatalf("%s => %q", tc.body, got)
		}
	}
}
