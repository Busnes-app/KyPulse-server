// Package egress is the one outbound HTTP client: health polls, KyYard and webhooks all go
// through it. Private and LAN addresses are allowed, because that is where the apps live;
// loopback, link-local and the known cloud metadata addresses, unspecified, multicast and
// reserved ranges are refused at dial time, so a DNS rebind cannot slip past a check made at resolution.
// Redirects are refused and response bodies are capped.
package egress

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrRefusedAddress = errors.New("egress: destination resolves only to loopback, link-local or reserved addresses")
	ErrRedirect       = errors.New("egress: redirects are refused")
	ErrBodyTooLarge   = errors.New("egress: response body over the cap")
	ErrScheme         = errors.New("egress: URL must be https (or http where explicitly allowed), with a host and no credentials")
)

// Options configures a Client. Zero values mean the defaults below.
type Options struct {
	AllowHTTP bool          // plain http:// URLs; off for webhooks and KyYard, on for health targets
	Timeout   time.Duration // whole request; default 5s
	MaxBody   int64         // bytes read from a response; default 64 KiB
}

const (
	DefaultTimeout = 5 * time.Second
	DefaultMaxBody = 64 << 10
)

// Response is what a caller gets: the body is already read and capped.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	TLS        *tls.ConnectionState // the handshake that carried it; nil over plain http
}

type lookupFunc func(ctx context.Context, network, host string) ([]net.IP, error)
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// fallbackDial bounds a connect attempt while another resolved address remains to try. A
// reachable LAN or internet host connects well inside it.
const fallbackDial = 2 * time.Second

// Client is safe for concurrent use.
type Client struct {
	http      *http.Client
	tls       *tls.Config // shared with the transport; Handshake clones it
	allowHTTP bool
	maxBody   int64
	timeout   time.Duration
	lookup    lookupFunc
	dial      dialFunc
}

// New builds a Client with the system resolver.
func New(o Options) *Client {
	return newClient(o, net.DefaultResolver.LookupIP, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)
}

func newClient(o Options, lookup lookupFunc, dial dialFunc) *Client {
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxBody <= 0 {
		o.MaxBody = DefaultMaxBody
	}
	c := &Client{allowHTTP: o.AllowHTTP, maxBody: o.MaxBody, timeout: o.Timeout, lookup: lookup, dial: dial,
		tls: &tls.Config{MinVersion: tls.VersionTLS12}}
	transport := &http.Transport{
		TLSClientConfig: c.tls,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return c.guardedDial(ctx, network, address, allowedIP)
		},
	}
	c.http = &http.Client{Timeout: o.Timeout, Transport: transport, CheckRedirect: refuseRedirect}
	return c
}

// guardedDial resolves address and dials only the addresses allow admits.
func (c *Client) guardedDial(ctx context.Context, network, address string, allow func(net.IP) bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := c.lookup(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var allowed []net.IP
	for _, ip := range ips {
		if allow(ip) {
			allowed = append(allowed, ip)
		}
	}
	// Try each allowed address in turn, as the standard dialer would: one dead address
	// of a multi-homed name must not fail the whole request. The dial context carries no
	// request deadline, so each attempt but the last is capped at fallbackDial (or its
	// share of the client timeout, if smaller): one that never answers cannot use the
	// whole budget before a live address is tried.
	share := c.timeout / time.Duration(max(len(allowed), 1))
	if share > fallbackDial {
		share = fallbackDial
	}
	dialErr := error(ErrRefusedAddress)
	for i, ip := range allowed {
		attempt, cancel := ctx, context.CancelFunc(func() {})
		if i < len(allowed)-1 {
			attempt, cancel = context.WithTimeout(ctx, share)
		}
		conn, err := c.dial(attempt, network, net.JoinHostPort(ip.String(), port))
		cancel()
		if err == nil {
			return conn, nil
		}
		dialErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, dialErr
}
func refuseRedirect(*http.Request, []*http.Request) error { return ErrRedirect }

// ValidateURL is the check a URL gets when an admin saves it: scheme, host, no credentials,
// and a literal IP is judged the same way the dialer will judge a resolved one.
func ValidateURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil {
		return ErrScheme
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return ErrScheme
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !allowedIP(ip) {
		return ErrRefusedAddress
	}
	return nil
}

// Get fetches url and returns the capped body. A body over the cap is ErrBodyTooLarge, not a
// truncated success: the caller must never parse half a document as a health report.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	return c.do(ctx, http.MethodGet, rawURL, "", nil, nil)
}

// GetWith is Get with extra headers, for a bearer-authenticated read.
func (c *Client) GetWith(ctx context.Context, rawURL string, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodGet, rawURL, "", nil, headers)
}

// Post sends body with the given content type and extra headers.
func (c *Client) Post(ctx context.Context, rawURL, contentType string, body []byte, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodPost, rawURL, contentType, body, headers)
}

func (c *Client) do(ctx context.Context, method, rawURL, contentType string, body []byte, headers map[string]string) (*Response, error) {
	if err := ValidateURL(rawURL, c.allowHTTP); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kypulse")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.maxBody {
		return nil, ErrBodyTooLarge
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data, TLS: resp.TLS}, nil
}

// Handshake dials hostport through the address guard, completes one TLS handshake with
// serverName (the host when empty) and closes without sending application data. allowLocal
// additionally admits loopback and link-local, never the metadata addresses. A failed
// verification returns the *tls.CertificateVerificationError, which carries the chain.
func (c *Client) Handshake(ctx context.Context, hostport, serverName string, allowLocal bool) (*tls.ConnectionState, error) {
	if err := ValidateHostPort(hostport, allowLocal); err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(hostport)
	if serverName == "" {
		serverName = host
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	allow := allowedIP
	if allowLocal {
		allow = localIP
	}
	raw, err := c.guardedDial(ctx, "tcp", hostport, allow)
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	cfg := c.tls.Clone()
	cfg.ServerName = serverName
	conn := tls.Client(raw, cfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	state := conn.ConnectionState()
	return &state, nil
}

// ValidateHostPort is the save-time check for a TLS target: a host and a numeric port, and
// a literal IP judged as the dialer will judge it.
func ValidateHostPort(hostport string, allowLocal bool) error {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || host == "" || len(host) > 253 || strings.ContainsAny(host, "/@?#% ") {
		return ErrScheme
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return ErrScheme
	}
	if ip := net.ParseIP(host); ip != nil {
		if allowLocal && !localIP(ip) || !allowLocal && !allowedIP(ip) {
			return ErrRefusedAddress
		}
	}
	return nil
}

var metadataIPs = mustCIDRs("169.254.169.254/32", "169.254.170.2/32")

// localIP is allowedIP plus loopback and link-local unicast, for a target an administrator
// explicitly marked local. Cloud metadata addresses stay refused.
func localIP(ip net.IP) bool {
	if allowedIP(ip) {
		return true
	}
	if ip == nil || !(ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return false
	}
	for _, n := range metadataIPs {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// Cause names why a request failed, in a fixed vocabulary the UI can show and tests can pin.
func Cause(err error) string {
	var netErr net.Error
	var dnsErr *net.DNSError
	var tlsRecord tls.RecordHeaderError
	var certErr *tls.CertificateVerificationError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrRefusedAddress):
		return "address_refused"
	case errors.Is(err, ErrRedirect):
		return "redirect"
	case errors.Is(err, ErrBodyTooLarge):
		return "body_too_large"
	case errors.Is(err, ErrScheme):
		return "bad_url"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "timeout"
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.As(err, &certErr), errors.As(err, &tlsRecord), strings.Contains(err.Error(), "tls:"):
		return "tls"
	case strings.Contains(err.Error(), "connection refused"):
		return "refused"
	}
	return "network"
}

var reservedRanges = mustCIDRs("0.0.0.0/8", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b::/96", "fec0::/10",
	"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", // RFC 5737 documentation
	"fd00:ec2::254/128", "100.100.100.200/32", // AWS IPv6 and Alibaba metadata, inside ULA and CGNAT
)

// allowedIP admits private (RFC 1918, ULA) and CGNAT addresses and refuses everything that
// can only mean this host, this link, or nowhere.
func allowedIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, n := range reservedRanges {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(fmt.Sprintf("egress: bad CIDR %q: %v", c, err))
		}
		out = append(out, n)
	}
	return out
}
