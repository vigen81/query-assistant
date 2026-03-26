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
const bannerEnV1 = `Create a high-quality promotional banner image for an online gaming platform.

Banner size: {width}x{height} pixels.

Visual style preset: {visual_style}

Main headline text to render clearly in the image:
"{headline}"

Secondary text to render near or below the headline:
"{secondary_text}"

Call-to-action button text:
"{cta_text}"

Additional creative guidance:
{creative_description}

Design requirements:

• The banner must look like a professional marketing promotion.
• The layout must fit a wide banner format.
• The headline text must be large, clear, and highly readable.
• Secondary text should be smaller but still clearly readable.
• The CTA text should appear inside a button or highlighted element.
• Typography must be clean, modern, and easy to read.
• Maintain clear visual hierarchy: headline → secondary text → CTA.

Visual guidance:

• Use strong contrast between text and background.
• The design should feel modern, premium and visually striking.
• Visual elements may include light effects, glowing accents, and abstract decorative elements.
• Avoid clutter and keep the layout balanced.

Strict constraints:

• Do NOT include logos.
• Do NOT include human faces.
• Do NOT include copyrighted characters.
• Do NOT add any extra text that was not provided above.
• All text must be spelled correctly in English.

Output requirements:

• Produce a single clean banner image.
• Ensure all text is readable and not distorted.
• Avoid warped, broken, or misspelled letters.
• Prioritize text readability over visual effects.`
