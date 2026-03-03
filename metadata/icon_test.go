package metadata

import "testing"

func TestFindSelfhstIconName(t *testing.T) {
	tests := []struct {
		name    string
		project string
		icons   []string
		want    string
		ok      bool
	}{
		{name: "direct match", project: "openbao", icons: []string{"openbao"}, want: "openbao", ok: true},
		{name: "case insensitive", project: "openbao", icons: []string{"OpenBao"}, want: "OpenBao", ok: true},
		{name: "light variant only", project: "openbao", icons: []string{"openbao-light"}, want: "openbao-light", ok: true},
		{name: "dark variant case mix", project: "OpenBao", icons: []string{"openbao-dark"}, want: "openbao-dark", ok: true},
		{name: "plain preferred over variant", project: "openbao", icons: []string{"openbao-light", "openbao", "openbao-dark"}, want: "openbao", ok: true},
		{name: "no match", project: "OpenBao", icons: []string{"other"}, want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findSelfhstIconName(tt.project, tt.icons)
			if ok != tt.ok {
				t.Fatalf("unexpected ok status: got %v want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("unexpected icon: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestTrimIconVariant(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "openbao-light", want: "openbao"},
		{input: "openbao-dark", want: "openbao"},
		{input: "openbao", want: "openbao"},
		{input: "actual-budget-light", want: "actual-budget"},
		{input: "light", want: "light"}, // too short to be a suffix match
	}

	for _, tt := range tests {
		if got := trimIconVariant(tt.input); got != tt.want {
			t.Fatalf("trimIconVariant(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
