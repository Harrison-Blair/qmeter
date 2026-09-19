package display

// ProviderOrder is the presentation order, kept in step with usage.Registry.
func ProviderOrder() []string {
	return []string{"claude", "codex", "opencode-go", "cursor"}
}

var providerGlyphs = map[string]string{
	"claude":      "◆",
	"codex":       "●",
	"opencode-go": "○",
	"cursor":      "▲",
}

// ProviderGlyph identifies a provider on gauges and reset timelines.
func ProviderGlyph(id string) string {
	if glyph, ok := providerGlyphs[id]; ok {
		return glyph
	}
	return "•"
}
