package adapter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type managedMCP struct {
	server  MCPServer
	config  MCPConfig
	catalog MCPCatalog
}

// MCPRegistry owns configuration, authentication, catalog refresh, status,
// and cleanup for explicitly allowlisted servers.
type MCPRegistry struct {
	mu        sync.RWMutex
	allowlist map[string]struct{}
	servers   map[string]*managedMCP
}

func NewMCPRegistry(allowlist ...string) *MCPRegistry {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		if name = strings.TrimSpace(name); name != "" {
			allowed[name] = struct{}{}
		}
	}
	return &MCPRegistry{allowlist: allowed, servers: make(map[string]*managedMCP)}
}

func (r *MCPRegistry) Register(ctx context.Context, config MCPConfig, server MCPServer) error {
	if server == nil || config.Name == "" {
		return fmt.Errorf("MCP registration requires name and server")
	}
	r.mu.Lock()
	if _, allowed := r.allowlist[config.Name]; !allowed {
		r.mu.Unlock()
		return fmt.Errorf("MCP server %q is not allowlisted", config.Name)
	}
	if _, exists := r.servers[config.Name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("MCP server %q already registered", config.Name)
	}
	r.mu.Unlock()
	if err := server.Validate(config); err != nil {
		return fmt.Errorf("validate MCP server %q: %w", config.Name, err)
	}
	if err := server.Configure(ctx, config); err != nil {
		return fmt.Errorf("configure MCP server %q: %w", config.Name, err)
	}
	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("start MCP server %q: %w", config.Name, err)
	}
	started := true
	defer func() {
		if started {
			_ = server.Stop(context.WithoutCancel(ctx))
		}
	}()
	if config.Auth != AuthNone {
		if err := server.Authenticate(ctx); err != nil {
			return fmt.Errorf("authenticate MCP server %q: %w", config.Name, err)
		}
	}
	catalog, err := server.Discover(ctx)
	if err != nil {
		return fmt.Errorf("discover MCP server %q: %w", config.Name, err)
	}
	if catalog.Version == "" {
		return fmt.Errorf("MCP server %q returned an unversioned catalog", config.Name)
	}
	r.mu.Lock()
	r.servers[config.Name] = &managedMCP{server: server, config: config, catalog: catalog}
	r.mu.Unlock()
	started = false
	return nil
}

func (r *MCPRegistry) Refresh(ctx context.Context, name string, refreshAuth bool) (MCPCatalog, error) {
	item, err := r.get(name)
	if err != nil {
		return MCPCatalog{}, err
	}
	if refreshAuth && item.config.Auth != AuthNone {
		if err := item.server.RefreshAuth(ctx); err != nil {
			return MCPCatalog{}, fmt.Errorf("refresh MCP auth %q: %w", name, err)
		}
	}
	catalog, err := item.server.Discover(ctx)
	if err != nil {
		return MCPCatalog{}, fmt.Errorf("refresh MCP catalog %q: %w", name, err)
	}
	if catalog.Version == "" {
		return MCPCatalog{}, fmt.Errorf("MCP server %q returned an unversioned catalog", name)
	}
	r.mu.Lock()
	if current, ok := r.servers[name]; ok && current.server == item.server {
		current.catalog = catalog
	}
	r.mu.Unlock()
	return catalog, nil
}

func (r *MCPRegistry) Reconnect(ctx context.Context, name string) error {
	item, err := r.get(name)
	if err != nil {
		return err
	}
	_ = item.server.Stop(context.WithoutCancel(ctx))
	if err := item.server.Start(ctx); err != nil {
		return fmt.Errorf("restart MCP server %q: %w", name, err)
	}
	if item.config.Auth != AuthNone {
		if err := item.server.RefreshAuth(ctx); err != nil {
			return fmt.Errorf("refresh MCP auth %q: %w", name, err)
		}
	}
	_, err = r.Refresh(ctx, name, false)
	return err
}

// WatchCatalog consumes versioned updates until ctx is cancelled. Invalid or
// empty versions stop the watch rather than replacing a known-good catalog.
func (r *MCPRegistry) WatchCatalog(ctx context.Context, name string) error {
	item, err := r.get(name)
	if err != nil {
		return err
	}
	updates, err := item.server.WatchCatalog(ctx)
	if err != nil {
		return fmt.Errorf("watch MCP catalog %q: %w", name, err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case catalog, ok := <-updates:
			if !ok {
				return fmt.Errorf("MCP catalog watch %q closed", name)
			}
			if catalog.Version == "" {
				return fmt.Errorf("MCP catalog watch %q returned an unversioned update", name)
			}
			r.mu.Lock()
			if current, exists := r.servers[name]; exists && current.server == item.server {
				current.catalog = catalog
			}
			r.mu.Unlock()
		}
	}
}

func (r *MCPRegistry) Catalog(name string) (MCPCatalog, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.servers[name]
	if !ok {
		return MCPCatalog{}, false
	}
	return item.catalog, true
}

func (r *MCPRegistry) Statuses(ctx context.Context) (map[string]MCPStatus, error) {
	r.mu.RLock()
	names := make([]string, 0, len(r.servers))
	for name := range r.servers {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	out := make(map[string]MCPStatus, len(names))
	for _, name := range names {
		item, err := r.get(name)
		if err != nil {
			return nil, err
		}
		status, err := item.server.Status(ctx)
		if err != nil {
			return nil, fmt.Errorf("status MCP server %q: %w", name, err)
		}
		out[name] = status
	}
	return out, nil
}

func (r *MCPRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	item, ok := r.servers[name]
	if ok {
		delete(r.servers, name)
	}
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("MCP server %q not registered", name)
	}
	if err := item.server.Stop(ctx); err != nil {
		return fmt.Errorf("stop MCP server %q: %w", name, err)
	}
	return nil
}

func (r *MCPRegistry) get(name string) (*managedMCP, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.servers[name]
	if !ok {
		return nil, fmt.Errorf("MCP server %q not registered", name)
	}
	copyItem := *item
	return &copyItem, nil
}
