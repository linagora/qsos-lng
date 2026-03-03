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
		{name: "direct match", project: "openbao", icons: []string{"OpenBao"}, want: "OpenBao", ok: true},
		{name: "light variant", project: "OpenBao", icons: []string{"OpenBao-Light"}, want: "OpenBao-Light", ok: true},
		{name: "dark variant case mix", project: "OpenBao", icons: []string{"openbao-Dark"}, want: "openbao-Dark", ok: true},
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
		{input: "OpenBao-Light", want: "OpenBao"},
		{input: "openbao-Dark", want: "openbao"},
		{input: "OpenBao", want: "OpenBao"},
	}

	for _, tt := range tests {
		if got := trimIconVariant(tt.input); got != tt.want {
			t.Fatalf("trimIconVariant(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
