package mcpserver

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTPOptions are resolved transport settings. Token values never enter config files.
type HTTPOptions struct {
	Address        string
	BearerToken    string
	AllowedHosts   []string
	AllowedOrigins []string
	MaxBodyBytes   int64
	MaxConcurrent  int
	RequestTimeout time.Duration
	ShutdownGrace  time.Duration
}

// HTTPHandler returns a stateless Streamable HTTP handler with bounded admission.
func (s *Server) HTTPHandler(opts HTTPOptions) (http.Handler, error) {
	if err := validateHTTPOptions(opts); err != nil {
		return nil, err
	}
	base := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: opts.MaxBodyBytes, PropagateRequestCancellation: true,
	})
	semaphore := make(chan struct{}, opts.MaxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !validHost(r.Host, opts.Address, opts.AllowedHosts) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		if opts.BearerToken != "" && !validBearer(r.Header.Get("Authorization"), opts.BearerToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="agentic-moe"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(opts.AllowedOrigins, origin) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
		default:
			http.Error(w, "server busy", http.StatusTooManyRequests)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), opts.RequestTimeout)
		defer cancel()
		base.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}

func validHost(requestHost, address string, allowed []string) bool {
	host := requestHost
	if parsed, _, err := net.SplitHostPort(requestHost); err == nil {
		host = parsed
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if len(allowed) > 0 {
		for _, candidate := range allowed {
			if strings.EqualFold(strings.Trim(candidate, "[]"), host) {
				return true
			}
		}
		return false
	}
	bindHost, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if isLoopbackHost(bindHost) {
		return isLoopbackHost(host)
	}
	return strings.EqualFold(strings.Trim(bindHost, "[]"), host)
}

// RunHTTP serves until cancellation and then performs a bounded graceful shutdown.
func (s *Server) RunHTTP(ctx context.Context, opts HTTPOptions) error {
	handler, err := s.HTTPHandler(opts)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr: opts.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: opts.RequestTimeout + 5*time.Second, WriteTimeout: opts.RequestTimeout + 5*time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	listener, err := net.Listen("tcp", opts.Address)
	if err != nil {
		return fmt.Errorf("listen mcp http: %w", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(listener) }()
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve mcp http: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownGrace)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown mcp http: %w", err)
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve mcp http: %w", err)
		}
		return nil
	}
}

func validateHTTPOptions(opts HTTPOptions) error {
	host, _, err := net.SplitHostPort(opts.Address)
	if err != nil {
		return fmt.Errorf("mcp http: address must be host:port: %w", err)
	}
	if opts.MaxBodyBytes <= 0 || opts.MaxConcurrent <= 0 || opts.RequestTimeout <= 0 || opts.ShutdownGrace <= 0 {
		return fmt.Errorf("mcp http: limits and timeouts must be positive")
	}
	if !isLoopbackHost(host) && opts.BearerToken == "" {
		return fmt.Errorf("mcp http: bearer token is required for non-loopback binds")
	}
	if opts.BearerToken != "" && (strings.TrimSpace(opts.BearerToken) == "" || strings.ContainsAny(opts.BearerToken, " \t\r\n")) {
		return fmt.Errorf("mcp http: bearer token must be a non-empty token without whitespace")
	}
	if ip := net.ParseIP(host); (host == "" || ip != nil && ip.IsUnspecified()) && len(opts.AllowedHosts) == 0 {
		return fmt.Errorf("mcp http: wildcard binds require allowed hosts")
	}
	for _, allowed := range opts.AllowedHosts {
		allowed = strings.Trim(strings.TrimSpace(allowed), "[]")
		if allowed == "" || strings.ContainsAny(allowed, "/@") || strings.Contains(allowed, ":") && net.ParseIP(allowed) == nil {
			return fmt.Errorf("mcp http: invalid allowed host %q", allowed)
		}
	}
	for _, origin := range opts.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("mcp http: invalid allowed origin %q", origin)
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validBearer(header, expected string) bool {
	scheme, actual, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || actual == "" || strings.Contains(actual, " ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
