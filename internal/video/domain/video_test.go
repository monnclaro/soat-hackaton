package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to Status
		want     bool
	}{
		{StatusPending, StatusProcessing, true},
		{StatusPending, StatusCompleted, true},
		{StatusPending, StatusFailed, true},
		{StatusProcessing, StatusProcessing, true},
		{StatusProcessing, StatusCompleted, true},
		{StatusProcessing, StatusFailed, true},
		{StatusProcessing, StatusPending, false},
		{StatusCompleted, StatusProcessing, false},
		{StatusCompleted, StatusFailed, false},
		{StatusFailed, StatusCompleted, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("%s → %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestApply(t *testing.T) {
	now := time.Now()
	v := Video{Status: StatusPending}

	if !v.Apply(StatusChange{Status: StatusProcessing}, now) || v.Status != StatusProcessing || !v.UpdatedAt.Equal(now) {
		t.Fatalf("processing: %+v", v)
	}
	if !v.Apply(StatusChange{Status: StatusCompleted, ZipKey: "z.zip", FrameCount: 9}, now) {
		t.Fatal("completed should apply")
	}
	if v.ZipKey != "z.zip" || v.FrameCount != 9 || !v.Downloadable() {
		t.Fatalf("completed: %+v", v)
	}
	if v.Apply(StatusChange{Status: StatusFailed, Error: "late"}, now) || v.Status != StatusCompleted {
		t.Fatal("terminal state must not change")
	}

	f := Video{Status: StatusProcessing}
	if !f.Apply(StatusChange{Status: StatusFailed, Error: "corrompido"}, now) || f.ErrorMessage != "corrompido" || f.Downloadable() {
		t.Fatalf("failed: %+v", f)
	}
}

func TestOwnedBy(t *testing.T) {
	v := Video{UserID: "u1"}
	if !v.OwnedBy("u1") || v.OwnedBy("u2") {
		t.Fatal("ownership")
	}
}

func TestValidateFileName(t *testing.T) {
	name, ext, err := ValidateFileName(`C:\videos\Aula 1.MP4`)
	if err != nil || name != "Aula 1.MP4" || ext != ".mp4" {
		t.Fatalf("got %q %q %v", name, ext, err)
	}
	if name, _, err := ValidateFileName("../../etc/passwd.mkv"); err != nil || name != "passwd.mkv" {
		t.Fatalf("path traversal should be stripped: %q %v", name, err)
	}
	for _, bad := range []string{"", "  ", "file.txt", "noext"} {
		if _, _, err := ValidateFileName(bad); err == nil {
			t.Errorf("ValidateFileName(%q) expected error", bad)
		}
	}
	if _, _, err := ValidateFileName(strings.Repeat("a", 300) + ".mp4"); !errors.Is(err, ErrInvalidFileName) {
		t.Fatalf("long name: %v", err)
	}
}

func TestNames(t *testing.T) {
	if got := StorageKey("u1", "v1", ".mp4"); got != "videos/u1/v1.mp4" {
		t.Fatal(got)
	}
	if got := ZipFileName("aula.mp4"); got != "aula_frames.zip" {
		t.Fatal(got)
	}
	if got := ZipFileName(".mp4"); got != "video_frames.zip" {
		t.Fatal(got)
	}
}
