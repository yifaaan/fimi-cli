package mcp

import (
	"context"
	"fmt"
	"log"
	"sync"

	"fimi-cli/internal/config"
)

// Manager holds multiple MCP clients and their discovered tools.
type Manager struct {
	clients map[string]*Client
	tools   []Tool
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager and connects to all configured servers.
// Servers that fail to connect are logged and skipped.
func NewManager(ctx context.Context, cfg config.MCPConfig) *Manager {
	m := &Manager{
		clients: make(map[string]*Client),
	}

	if !cfg.Enabled || len(cfg.Servers) == 0 {
		return m
	}

	for name, server := range cfg.Servers {
		client, err := NewClient(ctx, name, server.Command, server.Args, server.Env)
		if err != nil {
			log.Printf("[MCP] failed to connect to server %q: %v", name, err)
			continue
		}

		m.clients[name] = client
		m.tools = append(m.tools, client.Tools()...)
		log.Printf("[MCP] connected to server %q, discovered %d tools", name, len(client.Tools()))
	}

	return m
}

// Close closes all MCP client connections.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close MCP client %q: %w", name, err))
		}
	}
	m.clients = make(map[string]*Client)
	m.tools = nil

	if len(errs) > 0 {
		return fmt.Errorf("close MCP clients: %v", errs)
	}
	return nil
}

// Tools returns all discovered MCP tools across all servers.
func (m *Manager) Tools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools
}

// Client returns the MCP client for a given server name, or nil if not found.
func (m *Manager) Client(name string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[name]
}

// ClientForTool returns the MCP client that provides the given tool name.
// Returns nil if no client provides the tool.
func (m *Manager) ClientForTool(toolName string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		for _, tool := range client.Tools() {
			if tool.Name == toolName {
				return client
			}
		}
	}
	return nil
}
