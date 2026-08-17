package release

import (
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		prev, next string
		want       Kind
		wantErr    error
	}{
		{"1.0.4", "1.0.5", Patch, nil},
		{"1.0.5", "1.1.0", Minor, nil},
		{"1.1.0", "2.0.0", Major, nil},
		{"1.0.5", "2.1.3", Major, nil},
		{"1.0.5", "1.0.5", 0, ErrNoVersionBump},
		{"1.0.5", "1.0.4", 0, ErrVersionWentBackwards},
		{"1.1.0", "1.0.9", 0, ErrVersionWentBackwards},
		{"2.0.0", "1.9.9", 0, ErrVersionWentBackwards},
	}
	for _, tc := range tests {
		t.Run(tc.prev+"->"+tc.next, func(t *testing.T) {
			got, err := Classify(tc.prev, tc.next)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Classify(%q, %q) error = %v, want %v", tc.prev, tc.next, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Classify(%q, %q) = %v", tc.prev, tc.next, err)
			}
			if got != tc.want {
				t.Errorf("Classify(%q, %q) = %v, want %v", tc.prev, tc.next, got, tc.want)
			}
		})
	}
}

func TestClassify_NoPrevious(t *testing.T) {
	got, err := Classify("", "1.0.0")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != Major {
		t.Errorf("first release = %v, want %v", got, Major)
	}
}

func TestClassify_RejectsMalformed(t *testing.T) {
	for _, next := range []string{"1.0", "one.two.three", "", "1.0.x"} {
		if _, err := Classify("1.0.0", next); err == nil {
			t.Errorf("Classify(1.0.0, %q) = nil error, want one", next)
		}
	}
}

func TestNextPatch(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1.0.5", "1.0.6"},
		{"1.9.9", "1.9.10"},
		{"2.0.0", "2.0.1"},
	}
	for _, tc := range tests {
		got, err := NextPatch(tc.in)
		if err != nil {
			t.Fatalf("NextPatch(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("NextPatch(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRelease_Tag(t *testing.T) {
	if got := (Release{Name: "1.0.5"}).Tag(); got != "v1.0.5" {
		t.Errorf("Tag = %q, want v1.0.5", got)
	}
}
