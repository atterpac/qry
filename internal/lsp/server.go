package lsp

import (
	"context"
	"fmt"
	"net"
	"os"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/atterpac/qry/internal/autocomplete"
	"github.com/atterpac/qry/internal/engine"
)

// LSPServer manages the lifecycle of an LSP server over a unix socket.
type LSPServer struct {
	handler  *SQLServer
	listener net.Listener
	sockPath string
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewLSPServer creates a new LSP server backed by the given autocomplete components.
func NewLSPServer(cache *autocomplete.SchemaCache, eng *autocomplete.SuggestionEngine, provider engine.Provider) *LSPServer {
	return &LSPServer{
		handler: NewSQLServer(cache, eng, provider),
		done:    make(chan struct{}),
	}
}

// Start begins listening on a unix socket and serving LSP requests.
// Returns the socket path for clients to connect to.
func (s *LSPServer) Start() (string, error) {
	tmpFile, err := os.CreateTemp("", "qry-lsp-*.sock")
	if err != nil {
		return "", fmt.Errorf("create temp socket: %w", err)
	}
	s.sockPath = tmpFile.Name()
	tmpFile.Close()
	os.Remove(s.sockPath)

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return "", fmt.Errorf("listen unix: %w", err)
	}
	s.listener = ln

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	handler := protocol.ServerHandler(s.handler, jsonrpc2.MethodNotFoundHandler)
	streamServer := jsonrpc2.HandlerServer(protocol.Handlers(handler))

	go func() {
		defer close(s.done)
		jsonrpc2.Serve(ctx, ln, streamServer, 0)
	}()

	return s.sockPath, nil
}

// Shutdown stops the LSP server and cleans up the socket file.
func (s *LSPServer) Shutdown() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	<-s.done
	if s.sockPath != "" {
		os.Remove(s.sockPath)
	}
}
