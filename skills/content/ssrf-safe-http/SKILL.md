---
name: ssrf-safe-http
description: Build an outbound HTTP fetcher that resists SSRF — http(s) only, no userinfo, no IP literals in loopback/private/link-local ranges, DNS-resolved addresses re-checked at dial time. Use for any tool that fetches a URL the model or the user supplied.
category: security
priority: 5
---

# SSRF-safe HTTP fetch

A naive `http.Get(url)` is an SSRF primitive: an attacker (or a
confused model) can point it at `http://169.254.169.254/`,
`http://localhost:6379/`, `file:///etc/passwd`, or
`http://internal-admin.svc.cluster.local/`. The fix is three layers
of validation.

## Layer 1: URL shape

Reject the request before opening a socket:

- Scheme **must** be `http` or `https`. No `file`, `ftp`, `gopher`,
  `data`, `javascript`, `chrome-extension`, …
- No userinfo (`user:pass@`) — closes credential-smuggling.
- Hard length cap (e.g. 2048 chars).
- Host present.
- If host is a literal IP, refuse anything that fails
  `isUnsafeIP`: loopback, unspecified, multicast, link-local,
  RFC1918, IPv6 ULA. The cloud-metadata IP `169.254.169.254` is
  caught by link-local; mention it in the rejection message so
  reviewers see the threat called out.

## Layer 2: DNS at dial time

Hostnames that look fine can resolve to private space — either
because the operator legitimately runs an internal HTTP service on a
public-looking name, or because an attacker controls the DNS record.
Plug a custom `DialContext` that:

1. Resolves the hostname.
2. Walks every returned address and refuses on the first unsafe IP.
3. Dials the first remaining safe IP explicitly (don't go back
   through the resolver — that re-opens the TOCTOU window).

## Layer 3: bounded response

Even from a safe origin, an attacker can serve gigabytes:

- `client.Timeout` (e.g. 30s).
- `Transport.ResponseHeaderTimeout` so a slow-headers attack can't
  hold the goroutine.
- Read the body via `io.LimitReader(body, maxBytes+1)` — never trust
  `Content-Length`.
- Apply a *hard* upper bound on the caller's `max_length` arg so a
  malicious tool-arg can't blow the context budget.

## Sketch

```go
u, err := validateExternalURL(raw)              // layer 1
if err != nil { return errResult(err) }

client := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        DialContext: func(ctx context.Context, net, addr string) (net.Conn, error) {
            host, port, _ := netpkg.SplitHostPort(addr)
            ips, err := netpkg.DefaultResolver.LookupIPAddr(ctx, host)
            if err != nil { return nil, err }
            for _, a := range ips {
                if isUnsafeIP(a.IP) {
                    return nil, fmt.Errorf("refusing %s (resolves to %s)", host, a.IP)
                }
            }
            return dialer.DialContext(ctx, net, netpkg.JoinHostPort(ips[0].IP.String(), port))
        },
        DisableKeepAlives:     true,
        ResponseHeaderTimeout: 20 * time.Second,
    },
}

resp, err := client.Do(req)                     // layers 2 + 3
defer resp.Body.Close()
body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
```

## Anti-patterns

- Validating the URL but using `http.DefaultClient` — Default has no
  dial guard.
- Following redirects without re-validating the next URL.
- Using `net.ResolveIPAddr` once at validation time and trusting it
  for the dial — the resolver can flip between the two calls (TOCTOU).
- Allowing `127.0.0.1` "just for local dev" — promote a config flag,
  don't silently relax in production.
