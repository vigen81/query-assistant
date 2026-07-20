package imagegen

import (
	"fmt"
	"strings"
)

// Preset is a provider-agnostic image orientation exposed to the frontend.
//
// The frontend selects one of these values; each provider is responsible for
// translating it into a concrete pixel size it supports. This keeps the API
// contract stable when providers or models change.
type Preset string

const (
	PresetPortrait  Preset = "portrait"
	PresetLandscape Preset = "landscape"
	PresetSquare    Preset = "square"
)

// DefaultPreset is used when the request omits an image preset.
const DefaultPreset = PresetSquare

// supportedPresets is the closed set of values accepted from clients.
var supportedPresets = []Preset{PresetPortrait, PresetLandscape, PresetSquare}

// SupportedPresets returns the accepted preset values, for docs and errors.
func SupportedPresets() []string {
	out := make([]string, 0, len(supportedPresets))
	for _, p := range supportedPresets {
		out = append(out, string(p))
	}
	return out
}

// ParsePreset normalises and validates a client-supplied preset string.
//
// An empty value resolves to DefaultPreset. Values are matched
// case-insensitively and trimmed. Any other value is rejected.
func ParsePreset(raw string) (Preset, error) {
	normalised := strings.ToLower(strings.TrimSpace(raw))
	if normalised == "" {
		return DefaultPreset, nil
	}
	for _, p := range supportedPresets {
		if Preset(normalised) == p {
			return p, nil
		}
	}
	return "", fmt.Errorf(
		"output.image_preset %q is not supported: must be one of %s",
		raw, strings.Join(SupportedPresets(), ", "),
	)
}

// Size is a concrete pixel canvas resolved from a Preset by a provider.
type Size struct {
	Width  int
	Height int
}

// String renders the size in WxH form, as used by provider APIs and logs.
func (s Size) String() string {
	return fmt.Sprintf("%dx%d", s.Width, s.Height)
}
