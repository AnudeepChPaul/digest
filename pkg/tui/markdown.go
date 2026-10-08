package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

var (
	markdownRenderersMu sync.Mutex
	markdownRenderers   = map[int]*glamour.TermRenderer{}
)

const maxMarkdownRenderers = 4

func markdownStyle() glamouransi.StyleConfig {
	style := styles.DarkStyleConfig
	style.Document.StylePrimitive.BackgroundColor = nil
	style.Paragraph.StylePrimitive.BackgroundColor = nil
	style.Heading.StylePrimitive.BackgroundColor = nil
	style.H1.StylePrimitive.BackgroundColor = nil
	style.H2.StylePrimitive.BackgroundColor = nil
	style.H3.StylePrimitive.BackgroundColor = nil
	style.H4.StylePrimitive.BackgroundColor = nil
	style.H5.StylePrimitive.BackgroundColor = nil
	style.H6.StylePrimitive.BackgroundColor = nil
	style.BlockQuote.StylePrimitive.BackgroundColor = nil
	style.Code.StylePrimitive.BackgroundColor = nil
	style.CodeBlock.StylePrimitive.BackgroundColor = nil
	if style.CodeBlock.Chroma != nil {
		chroma := *style.CodeBlock.Chroma
		chroma.Background.BackgroundColor = nil
		style.CodeBlock.Chroma = &chroma
	}
	return style
}

func markdownRendererFor(width int) *glamour.TermRenderer {
	markdownRenderersMu.Lock()
	defer markdownRenderersMu.Unlock()
	if renderer, cached := markdownRenderers[width]; cached {
		return renderer
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(markdownStyle()), glamour.WithWordWrap(width))
	if err != nil {
		return nil
	}
	if len(markdownRenderers) >= maxMarkdownRenderers {
		clear(markdownRenderers)
	}
	markdownRenderers[width] = renderer
	return renderer
}

var markdownRenderMu sync.Mutex

var renderMarkdown = func(body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return mutedStyle.Render("(No note body text)")
	}
	renderer := markdownRendererFor(width)
	if renderer == nil {
		return body
	}
	markdownRenderMu.Lock()
	out, err := renderer.Render(body)
	markdownRenderMu.Unlock()
	if err != nil {
		return body
	}
	return strings.TrimSpace(out)
}
