package events

import "testing"

func TestStatusRoutingKey(t *testing.T) {
	cases := map[string]string{
		StatusProcessing: "video.status.processing",
		StatusCompleted:  "video.status.completed",
		StatusFailed:     "video.status.failed",
		"X":              "video.status.unknown",
	}
	for in, want := range cases {
		if got := StatusRoutingKey(in); got != want {
			t.Fatalf("StatusRoutingKey(%q) = %q, want %q", in, got, want)
		}
	}
}
