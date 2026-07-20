package imagegen

import "testing"

func TestParsePreset(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Preset
		wantErr bool
	}{
		{"square", "square", PresetSquare, false},
		{"portrait", "portrait", PresetPortrait, false},
		{"landscape", "landscape", PresetLandscape, false},
		{"empty defaults to square", "", DefaultPreset, false},
		{"whitespace defaults to square", "   ", DefaultPreset, false},
		{"case insensitive", "PorTrait", PresetPortrait, false},
		{"trimmed", " landscape ", PresetLandscape, false},
		{"unsupported value", "banner", "", true},
		{"legacy dimensions string", "1024x1024", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePreset(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePreset(%q) = %q, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePreset(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePreset(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestOpenAIAdapterResolveSize(t *testing.T) {
	a := &OpenAIAdapter{}

	tests := []struct {
		preset Preset
		want   Size
	}{
		{PresetSquare, Size{1024, 1024}},
		{PresetPortrait, Size{1024, 1536}},
		{PresetLandscape, Size{1536, 1024}},
	}

	for _, tt := range tests {
		got, err := a.ResolveSize(tt.preset)
		if err != nil {
			t.Fatalf("ResolveSize(%q) unexpected error: %v", tt.preset, err)
		}
		if got != tt.want {
			t.Fatalf("ResolveSize(%q) = %s, want %s", tt.preset, got, tt.want)
		}
	}

	if _, err := a.ResolveSize(Preset("banner")); err == nil {
		t.Fatal("ResolveSize with unsupported preset: want error, got nil")
	}
}

// Every supported preset must be resolvable by the provider — this guards
// against adding a preset without extending the provider mapping.
func TestAllSupportedPresetsResolve(t *testing.T) {
	a := &OpenAIAdapter{}
	for _, p := range supportedPresets {
		if _, err := a.ResolveSize(p); err != nil {
			t.Errorf("preset %q is exposed to clients but not mapped by %s: %v", p, a.Name(), err)
		}
	}
}
