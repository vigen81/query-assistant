package imagegen

import (
	"fmt"
	"strings"
)

// TemplateVersion identifies a prompt template by language and version.
type TemplateVersion struct {
	Language string
	Version  string
}

// PromptParams holds the runtime values that are substituted into a template.
type PromptParams struct {
	Width               int
	Height              int
	Headline            string
	SecondaryText       string
	CTAText             string
	VisualStyle         string
	CreativeDescription string
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
	// Future languages:
	// pb.register("hy", "v1", bannerHyV1)
	// pb.register("ru", "v1", bannerRuV1)
	return pb
}

// register stores a template and marks it as the default for its language.
func (pb *PromptBuilder) register(language, version, tpl string) {
	key := language + "/" + version
	pb.templates[key] = tpl
	pb.defaults[language] = version // last registered wins
}

// Build renders the appropriate template for the given language.
// If the language has no template it falls back to English.
func (pb *PromptBuilder) Build(language string, p PromptParams) (string, error) {
	version, ok := pb.defaults[language]
	if !ok {
		// graceful fallback to English
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
	rendered = strings.ReplaceAll(rendered, "{headline}", p.Headline)
	rendered = strings.ReplaceAll(rendered, "{secondary_text}", p.SecondaryText)
	rendered = strings.ReplaceAll(rendered, "{cta_text}", p.CTAText)
	rendered = strings.ReplaceAll(rendered, "{visual_style}", p.VisualStyle)
	rendered = strings.ReplaceAll(rendered, "{creative_description}", p.CreativeDescription)

	return rendered, nil
}

// ---------------------------------------------------------------------------
// Versioned templates
// ---------------------------------------------------------------------------

// bannerEnV1 is the English prompt template v1.
// Source: banner_en_v1_prompt_template.txt
// Variables: {width} {height} {headline} {secondary_text} {cta_text}
//
//	{visual_style} {creative_description}
const bannerEnV1 = `Generate a premium promotional banner image for an online gaming platform.

CANVAS
- Size: {width}x{height} pixels
- Use the requested dimensions and aspect ratio for a single clean banner composition

STYLE
- Visual style preset: {visual_style}

REQUIRED TEXT
{headline_block}
{secondary_text_block}
{cta_text_block}

TEXT INSTRUCTIONS
- Use exactly the provided text elements only
- Do not paraphrase, translate, expand, shorten, duplicate, split incorrectly, or invent any text
- Do not add extra words, fake text, placeholder text, random symbols, or unreadable microtext
- All text must be clearly readable and correctly spelled in English

ADDITIONAL CREATIVE GUIDANCE
{creative_description_block}

PRIMARY OBJECTIVE
- Create a professional, modern, premium marketing banner (not an artistic poster)
- Prioritize text readability and clean promotional layout over artistic complexity

LAYOUT RULES
- Maintain clear visual hierarchy between the provided text elements
- The headline, when provided, must be the most prominent text element
- Secondary text, when provided, must appear near or below the headline and remain clearly readable
- CTA text, when provided, must appear inside a clear, simple, readable button
- Keep composition balanced, structured, and banner-like
- Reserve a clean central or upper-central text-safe area

TEXT-SAFE COMPOSITION RULES
- Keep all decorative elements away from text areas
- Do not place coins, glow, particles, lighting effects, or abstract elements over or directly behind text
- Keep background behind text simple, dark enough, and high-contrast

READABILITY RULES
- Use strong contrast between text and background
- Keep text large, clear, and visually separated
- Do not distort, warp, melt, bend, crop, blur, overlap, fragment, or misspell letters
- Do not crop or cut off any part of the required text
- Do not duplicate text blocks or render partial text

VISUAL GUIDANCE
- Premium, modern, visually striking promotional banner
- May include glow accents, light effects, and controlled decorative elements
- Keep visual richness controlled and supportive of readability

STRICT CONSTRAINTS
- Do not include logos
- Do not include human faces
- Do not include copyrighted characters
- Do not include any text other than the provided text elements

FINAL PRIORITY
1. Exact provided text only
2. Text readability
3. Clear marketing layout
4. Professional banner appearance
5. Visual richness

OUTPUT
- Produce one clean promotional banner image
- Ensure all required text is readable and usable
- Prioritize readability and correctness over visual effects`
