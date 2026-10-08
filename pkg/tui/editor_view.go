package tui

import "github.com/AnudeepChPaul/digest/pkg/tui/textarea"

var renderEditorView = func(editor *textarea.Model) string {
	return editor.View()
}

type editorViewKey struct {
	revision      int
	width, height int
	focused       bool
	row, column   int
}

type editorViewCache struct {
	key  editorViewKey
	view string
	set  bool
}

func (m Model) editorView() string {
	key := editorViewKey{
		revision: m.editorRevision,
		width:    m.editor.Width(),
		height:   m.editor.Height(),
		focused:  m.editor.Focused(),
		row:      m.editor.Line(),
		column:   m.editor.LineInfo().CharOffset,
	}
	if m.editorCache == nil {
		return renderEditorView(m.editor)
	}
	if !m.editorCache.set || m.editorCache.key != key {
		*m.editorCache = editorViewCache{key: key, view: renderEditorView(m.editor), set: true}
	}
	return m.editorCache.view
}

func (m *Model) replaceEditorText(text string) {
	m.editor.SetValue(text)
	m.editorRevision++
}
