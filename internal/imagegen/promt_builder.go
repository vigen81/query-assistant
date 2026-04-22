package imagegen

import (
	"fmt"
	"regexp"
	"strings"
)

// TemplateVersion identifies a prompt template by language and version.
type TemplateVersion struct {
	Language string
	Version  string
}

// PromptParams holds the runtime values substituted into a template.
// All content fields are optional individually; see HasContent() for minimum requirements.
type PromptParams struct {
	Width               int
	Height              int
	Headline            string
	SecondaryText       string
	CTAText             string
	VisualStyle         string
	CreativeDescription string
}

// HasText returns true if any of the rendered-text fields are set.
// Used to gate text-specific sections of the prompt.
func (p PromptParams) HasText() bool {
	return p.Headline != "" || p.SecondaryText != "" || p.CTAText != ""
}

// HasContent returns true if the minimum content requirement is satisfied:
// at least one of headline, cta_text, or creative_description must be provided.
func (p PromptParams) HasContent() bool {
	return p.Headline != "" || p.CTAText != "" || p.CreativeDescription != ""
}

// PromptBuilder selects the correct versioned template for a language and
// renders it with the supplied parameters.
type PromptBuilder struct {
	templates map[string]string // key: "<language>/<version>"
	defaults  map[string]string // key: language → latest version
}

// NewPromptBuilder creates a PromptBuilder with all built-in templates registered.
func NewPromptBuilder() *PromptBuilder {
	pb := &PromptBuilder{
		templates: make(map[string]string),
		defaults:  make(map[string]string),
	}
	pb.register("en", "v1", bannerEnV1)
	return pb
}

// register stores a template and marks it as the default for its language.
func (pb *PromptBuilder) register(language, version, tpl string) {
	key := language + "/" + version
	pb.templates[key] = tpl
	pb.defaults[language] = version
}

// Build renders the appropriate template for the given language.
// Falls back to English when the requested language has no registered template.
//
// Dynamic blocks injected by Build:
//   - {required_text_block}      — REQUIRED TEXT + TEXT INSTRUCTIONS (empty when no text fields)
//   - {creative_guidance_block}  — ADDITIONAL CREATIVE GUIDANCE (empty when creative_description absent)
//   - {hierarchy_instruction}    — single-line text hierarchy rule (adapts to present fields)
//   - {text_composition_block}   — TEXT-SAFE + READABILITY rules (empty when no text fields)
//   - {strict_text_constraint}   — adapts "no text" rule based on whether text is expected
func (pb *PromptBuilder) Build(language string, p PromptParams) (string, error) {
	version, ok := pb.defaults[language]
	if !ok {
		version = pb.defaults["en"]
		language = "en"
	}

	tpl, ok := pb.templates[language+"/"+version]
	if !ok {
		return "", fmt.Errorf("prompt template not found for %s/%s", language, version)
	}

	rendered := tpl
	rendered = strings.ReplaceAll(rendered, "{width}", fmt.Sprintf("%d", p.Width))
	rendered = strings.ReplaceAll(rendered, "{height}", fmt.Sprintf("%d", p.Height))
	rendered = strings.ReplaceAll(rendered, "{visual_style}", p.VisualStyle)
	rendered = strings.ReplaceAll(rendered, "{required_text_block}", buildRequiredTextBlock(p))
	rendered = strings.ReplaceAll(rendered, "{creative_guidance_block}", buildCreativeGuidanceBlock(p))
	rendered = strings.ReplaceAll(rendered, "{hierarchy_instruction}", buildHierarchyInstruction(p))
	rendered = strings.ReplaceAll(rendered, "{text_composition_block}", buildTextCompositionBlock(p))
	rendered = strings.ReplaceAll(rendered, "{strict_text_constraint}", buildStrictTextConstraint(p))

	// Collapse runs of 3+ blank lines created when optional blocks are empty.
	rendered = normaliseBlankLines(rendered)
	rendered = strings.TrimSpace(rendered)

	return rendered, nil
}

// ---------------------------------------------------------------------------
// Block builders
// ---------------------------------------------------------------------------

// buildRequiredTextBlock returns the REQUIRED TEXT section including the listed
// fields and the TEXT INSTRUCTIONS block. Returns empty string when no text
// fields are provided — the section is entirely omitted from the prompt.
func buildRequiredTextBlock(p PromptParams) string {
	if !p.HasText() {
		return ""
	}

	var lines []string
	if p.Headline != "" {
		lines = append(lines, fmt.Sprintf(`- Headline: %q`, p.Headline))
	}
	if p.SecondaryText != "" {
		lines = append(lines, fmt.Sprintf(`- Secondary text: %q`, p.SecondaryText))
	}
	if p.CTAText != "" {
		lines = append(lines, fmt.Sprintf(`- CTA button text: %q`, p.CTAText))
	}

	return fmt.Sprintf(
		"REQUIRED TEXT\n%s\n\nTEXT INSTRUCTIONS\n"+
			"- Use exactly the provided text elements only\n"+
			"- Do not paraphrase, translate, expand, shorten, duplicate, split incorrectly, or invent any text\n"+
			"- Do not add extra words, fake text, placeholder text, random symbols, or unreadable microtext\n"+
			"- All text must be clearly readable and correctly spelled in English",
		strings.Join(lines, "\n"),
	)
}

// buildCreativeGuidanceBlock returns the ADDITIONAL CREATIVE GUIDANCE section.
// Returns empty string when creative_description is absent.
func buildCreativeGuidanceBlock(p PromptParams) string {
	if p.CreativeDescription == "" {
		return ""
	}
	return fmt.Sprintf("ADDITIONAL CREATIVE GUIDANCE\n%s", p.CreativeDescription)
}

// buildTextCompositionBlock returns the TEXT-SAFE COMPOSITION RULES and
// READABILITY RULES sections. Returns empty string when no text is present —
// these rules are meaningless on a purely visual banner.
func buildTextCompositionBlock(p PromptParams) string {
	if !p.HasText() {
		return ""
	}
	return `TEXT-SAFE COMPOSITION RULES
- Keep all decorative elements away from text areas
- Do not place coins, glow, particles, lighting effects, abstract elements, or visual noise over or directly behind text
- Keep the background behind text simple, dark enough, and high-contrast
- Maintain safe margins around all text and CTA elements

READABILITY RULES
- Use strong contrast between text and background
- Keep text large, clear, and visually separated
- Do not distort, warp, melt, bend, crop, blur, overlap, fragment, or misspell letters
- Do not crop, cut off, or partially hide any part of the required text
- Do not duplicate text blocks or render partial text
- Do not simulate text using shapes, blocks, or placeholder-like patterns`
}

// buildStrictTextConstraint returns the appropriate strict-constraints line
// for text handling, based on whether rendered text is expected.
func buildStrictTextConstraint(p PromptParams) string {
	if p.HasText() {
		return "- Do not include any text other than the provided text elements"
	}
	return "- Do not include any text, labels, or symbols of any kind"
}

// buildHierarchyInstruction returns a single-sentence layout hierarchy rule
// that adapts to exactly which content fields are present.
//
// Combination matrix:
//
//	Headline + Secondary + CTA  → standard three-element hierarchy
//	Headline + CTA              → headline primary, CTA as button below
//	Headline + Secondary        → headline primary, secondary below
//	Headline only               → sole element, maximally prominent
//	CTA + Secondary             → CTA is primary focal element
//	CTA only                    → CTA is sole focal text element
//	Secondary only              → secondary is only element, clearly displayed
//	None (visual-only)          → striking visual composition, no text required
func buildHierarchyInstruction(p PromptParams) string {
	hasHeadline := p.Headline != ""
	hasCTA := p.CTAText != ""
	hasSecondary := p.SecondaryText != ""

	switch {
	case hasHeadline && hasCTA && hasSecondary:
		return "Headline is the primary element; secondary text appears near or below the headline; CTA appears as a clear button below both"
	case hasHeadline && hasCTA:
		return "Headline is the primary element; CTA appears as a clear, readable button below the headline"
	case hasHeadline && hasSecondary:
		return "Headline is the primary element; secondary text appears near or below the headline and remains clearly readable"
	case hasHeadline:
		return "The headline is the sole text element and must be maximally prominent across the composition"
	case hasCTA && hasSecondary:
		return "The CTA button is the primary focal element; secondary text appears above or near the CTA to provide context"
	case hasCTA:
		return "The CTA button is the sole and primary focal text element; it must be prominent, readable, and clearly button-shaped"
	case hasSecondary:
		return "Secondary text is the only text element; display it clearly, prominently, and in a readable size"
	default:
		return "This is a visual-only banner with no required text; the composition must be striking and self-explanatory"
	}
}

// ---------------------------------------------------------------------------
// Whitespace normalisation
// ---------------------------------------------------------------------------

// multipleBlankLines matches three or more consecutive newlines.
var multipleBlankLines = regexp.MustCompile(`\n{3,}`)

// normaliseBlankLines collapses runs of 3+ newlines to exactly 2,
// preventing ragged spacing when optional blocks are absent.
func normaliseBlankLines(s string) string {
	return multipleBlankLines.ReplaceAllString(s, "\n\n")
}

// ---------------------------------------------------------------------------
// Versioned templates
// ---------------------------------------------------------------------------

// bannerEnV1 is the English prompt template v1.
//
// Dynamic placeholders (all resolved by Build before the prompt is sent):
//
//	{width}                    output canvas width in pixels
//	{height}                   output canvas height in pixels
//	{visual_style}             visual style preset string (always present)
//	{required_text_block}      REQUIRED TEXT + TEXT INSTRUCTIONS (omitted when no text fields)
//	{creative_guidance_block}  ADDITIONAL CREATIVE GUIDANCE (omitted when creative_description absent)
//	{hierarchy_instruction}    single-line layout hierarchy rule (adapts to present fields)
//	{text_composition_block}   TEXT-SAFE + READABILITY rules (omitted when no text fields)
//	{strict_text_constraint}   adapts "no extra text" rule based on whether text is expected
const bannerEnV1 = `Generate a premium promotional banner image for an online gaming platform.

CANVAS
- Size: {width}x{height} pixels
- Use the requested dimensions and aspect ratio for one clean, banner-ready composition

STYLE
- Visual style preset: {visual_style}

{required_text_block}

{creative_guidance_block}

PRIMARY OBJECTIVE
- Create a professional, modern, premium marketing banner for an online gaming platform (not an artistic poster)
- Prioritize visual quality, clean promotional structure, and usability over artistic complexity

LAYOUT RULES
- {hierarchy_instruction}
- Keep composition balanced, structured, and banner-like
- Reserve a clean, high-contrast area appropriate to the composition
- Do not place key content too close to image edges

{text_composition_block}

VISUAL GUIDANCE
- Premium, modern, visually striking promotional banner
- May include glow accents, light effects, gradients, and controlled decorative elements
- Keep visual richness controlled and supportive of the overall design
- Avoid visual overload; decoration must support, not compete with, the composition

NEGATIVE CONSTRAINTS
- Do not create cluttered, overly dense, chaotic, or poster-like compositions
- Do not create multiple competing focal points
- Do not duplicate CTA buttons
- Do not use split layouts, collage layouts, or multi-scene compositions

STRICT CONSTRAINTS
- Do not include logos
- Do not include human faces
- Do not include copyrighted characters
{strict_text_constraint}

FINAL PRIORITY
1. {hierarchy_instruction}
2. Clear marketing layout
3. Professional banner appearance
4. Visual richness

OUTPUT
- Produce one premium promotional banner image variant
- Produce a single coherent marketing banner composition
- Prioritize usability and correctness over visual effects`
