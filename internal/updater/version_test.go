package updater

import "testing"

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "v0.2.0", want: "v0.2.0", ok: true},
		{input: "1.24.3", want: "v1.24.3", ok: true},
		{input: " v2.0.1 ", want: "v2.0.1", ok: true},
		{input: "v1.2", ok: false},
		{input: "v1.2.3-rc.1", ok: false},
		{input: "v1.02.3", ok: false},
		{input: "abc123", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeVersion(tt.input)
			if tt.ok && err != nil {
				t.Fatalf("NormalizeVersion() error = %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("NormalizeVersion() expected error, got %q", got)
			}
			if got != tt.want {
				t.Fatalf("NormalizeVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{a: "v1.0.0", b: "v1.0.1", want: -1},
		{a: "v1.2.0", b: "v1.1.9", want: 1},
		{a: "2.0.0", b: "v2.0.0", want: 0},
	}
	for _, tt := range tests {
		got, err := CompareVersions(tt.a, tt.b)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
