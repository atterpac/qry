package lsp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.lsp.dev/protocol"

	"github.com/atterpac/qry/internal/autocomplete"
	"github.com/atterpac/qry/internal/engine"
)

// SQLServer handles LSP requests using the qry autocomplete engine.
type SQLServer struct {
	NoopServer
	cache    *autocomplete.SchemaCache
	engine   *autocomplete.SuggestionEngine
	provider engine.Provider
	docs     map[protocol.DocumentURI]string
	mu       sync.RWMutex
}

// NewSQLServer creates a new SQL LSP handler.
func NewSQLServer(cache *autocomplete.SchemaCache, eng *autocomplete.SuggestionEngine, provider engine.Provider) *SQLServer {
	return &SQLServer{
		cache:    cache,
		engine:   eng,
		provider: provider,
		docs:     make(map[protocol.DocumentURI]string),
	}
}

func (s *SQLServer) Initialize(_ context.Context, _ *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindFull,
			},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", " "},
			},
			HoverProvider: true,
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "qry",
			Version: "0.1.0",
		},
	}, nil
}

func (s *SQLServer) Initialized(_ context.Context, _ *protocol.InitializedParams) error {
	return nil
}

func (s *SQLServer) DidOpen(_ context.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[params.TextDocument.URI] = params.TextDocument.Text
	return nil
}

func (s *SQLServer) DidChange(_ context.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(params.ContentChanges) > 0 {
		// Full sync mode: last content change has the full text.
		s.docs[params.TextDocument.URI] = params.ContentChanges[len(params.ContentChanges)-1].Text
	}
	return nil
}

func (s *SQLServer) DidClose(_ context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, params.TextDocument.URI)
	return nil
}

func (s *SQLServer) Completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	s.mu.RLock()
	text, ok := s.docs[params.TextDocument.URI]
	s.mu.RUnlock()
	if !ok {
		return &protocol.CompletionList{}, nil
	}

	byteOffset := positionToByteOffset(text, params.Position)
	suggestions := s.engine.Suggest(ctx, s.provider, text, byteOffset)

	items := make([]protocol.CompletionItem, 0, len(suggestions))
	for _, sg := range suggestions {
		insertText := sg.InsertText
		if insertText == "" {
			insertText = sg.Text
		}
		items = append(items, protocol.CompletionItem{
			Label:      sg.Text,
			InsertText: insertText,
			Detail:     sg.Description,
			Kind:       categoryToKind(sg.Category),
		})
	}

	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

func (s *SQLServer) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.mu.RLock()
	text, ok := s.docs[params.TextDocument.URI]
	s.mu.RUnlock()
	if !ok {
		return nil, nil
	}

	word := wordAtPosition(text, params.Position)
	if word == "" {
		return nil, nil
	}

	// Check if word is a known table and show its columns.
	schema := s.engine.Schema
	tableName := word

	// Handle schema.table notation.
	if parts := strings.SplitN(word, ".", 2); len(parts) == 2 {
		schema = parts[0]
		tableName = parts[1]
	}

	cols := s.cache.Columns(ctx, s.provider, schema, tableName)
	if len(cols) == 0 {
		return nil, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**%s.%s**\n\n", schema, tableName)
	b.WriteString("| Column | Type | Info |\n")
	b.WriteString("|--------|------|------|\n")
	for _, c := range cols {
		info := ""
		if c.IsPrimaryKey {
			info += "PK "
		}
		if !c.Nullable {
			info += "NOT NULL "
		}
		fmt.Fprintf(&b, "| %s | %s | %s|\n", c.Name, c.DataType, info)
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: b.String(),
		},
	}, nil
}

// positionToByteOffset converts an LSP Position (line, character) to a byte offset in the text.
func positionToByteOffset(text string, pos protocol.Position) int {
	line := int(pos.Line)
	char := int(pos.Character)

	offset := 0
	currentLine := 0
	for i, r := range text {
		if currentLine == line {
			if char <= 0 {
				return i
			}
			char--
			offset = i + len(string(r))
			if r == '\n' {
				return i
			}
			continue
		}
		if r == '\n' {
			currentLine++
		}
	}
	if currentLine == line {
		return offset
	}
	return len(text)
}

// wordAtPosition extracts the word (identifier) at the given position.
func wordAtPosition(text string, pos protocol.Position) string {
	lines := strings.Split(text, "\n")
	line := int(pos.Line)
	if line >= len(lines) {
		return ""
	}
	lineText := lines[line]
	col := int(pos.Character)
	if col > len(lineText) {
		col = len(lineText)
	}

	// Expand left.
	start := col
	for start > 0 && isIdentChar(lineText[start-1]) {
		start--
	}
	// Expand right.
	end := col
	for end < len(lineText) && isIdentChar(lineText[end]) {
		end++
	}
	if start == end {
		return ""
	}
	return lineText[start:end]
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '.'
}

func categoryToKind(category string) protocol.CompletionItemKind {
	switch category {
	case "Table":
		return protocol.CompletionItemKindClass
	case "Column":
		return protocol.CompletionItemKindField
	case "Schema":
		return protocol.CompletionItemKindModule
	case "CTE":
		return protocol.CompletionItemKindVariable
	case "Keyword":
		return protocol.CompletionItemKindKeyword
	case "Function":
		return protocol.CompletionItemKindFunction
	default:
		return protocol.CompletionItemKindText
	}
}
