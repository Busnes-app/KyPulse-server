package poller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// testChain makes a root and a leaf for names expiring at notAfter. No network.
func testChain(t *testing.T, notAfter time.Time, names ...string) (*x509.Certificate, tls.Certificate) {
	t.Helper()
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root"},
		NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, _ = x509.ParseCertificate(rootDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: notAfter,
		DNSNames: names, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	return root, tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}
}

// handshake runs a real TLS handshake against a loopback listener owned by the test.
func handshake(t *testing.T, leaf tls.Certificate, roots *x509.CertPool, name string) (*tls.ConnectionState, error) {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{leaf}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			_ = c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()
	a, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	c := tls.Client(a, &tls.Config{RootCAs: roots, ServerName: name, MinVersion: tls.VersionTLS12})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	s := c.ConnectionState()
	return &s, nil
}

func TestCertOfReadsTheHandshake(t *testing.T) {
	soon := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	root, leaf := testChain(t, soon, "app.lan")
	trusted := x509.NewCertPool()
	trusted.AddCert(root)

	for _, tc := range []struct {
		name, host string
		roots      *x509.CertPool
		notAfter   time.Time
		problem    string
		match      bool
	}{
		{"valid", "app.lan", trusted, soon, "", true},
		{"name mismatch", "other.lan", trusted, soon, "cert_name_mismatch", false},
		{"untrusted chain", "app.lan", x509.NewCertPool(), soon, "cert_untrusted", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, err := handshake(t, leaf, tc.roots, tc.host)
			c := CertOf(state, err, tc.host)
			if c == nil {
				t.Fatalf("no cert: %v", err)
			}
			if c.Problem != tc.problem || c.NameMatch != tc.match || !c.NotAfter.Equal(tc.notAfter) || c.Issuer != "Test Root" {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestCertOfNamesAnExpiredLeaf(t *testing.T) {
	root, leaf := testChain(t, time.Now().Add(-time.Hour), "app.lan")
	pool := x509.NewCertPool()
	pool.AddCert(root)
	state, err := handshake(t, leaf, pool, "app.lan")
	if c := CertOf(state, err, "app.lan"); c == nil || c.Problem != "cert_expired" {
		t.Fatalf("got %+v (%v)", c, err)
	}
}

func TestCertOfWithoutACertificateIsNil(t *testing.T) {
	if c := CertOf(nil, &net.OpError{Op: "dial", Err: errString("connection refused")}, "app.lan"); c != nil {
		t.Fatalf("got %+v", c)
	}
	if c := CertOf(nil, nil, "app.lan"); c != nil {
		t.Fatalf("plain http got %+v", c)
	}
}

func TestCertOfCapsTheIssuer(t *testing.T) {
	_, leaf := testChain(t, time.Now().Add(time.Hour), "app.lan")
	parsed, _ := x509.ParseCertificate(leaf.Certificate[0])
	parsed.Issuer.CommonName = strings.Repeat("x", 5000)
	c := CertOf(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{parsed}}, nil, "app.lan")
	if len(c.Issuer) != maxIssuer {
		t.Fatalf("issuer %d bytes", len(c.Issuer))
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestTLSTargetHealthIsReachability(t *testing.T) {
	_, leaf := testChain(t, time.Now().Add(30*24*time.Hour), "edge.lan")
	parsed, _ := x509.ParseCertificate(leaf.Certificate[0])
	g := &fakeGetter{state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{parsed}}}
	obs := make(chan Observation, 3)
	p := &Poller{Get: g, Workers: 3,
		Due: func(context.Context, time.Time) ([]Target, error) {
			return []Target{{ID: "a", URL: "tls://edge.lan:443"}, {ID: "b", URL: "tls://bad-cert.lan:443", SNI: "edge.lan"}, {ID: "c", URL: "tls://gone.lan:443"}}, nil
		},
		Observe: func(_ context.Context, o Observation) { obs <- o },
	}
	p.Tick(context.Background(), time.Now())
	got := map[string]Result{}
	for range 3 {
		o := <-obs
		got[o.Target.ID] = o.Result
	}
	if r := got["a"]; r.State != OK || r.Cert == nil || r.Cert.Problem != "" || !r.Cert.NameMatch {
		t.Fatalf("a: %+v", r)
	}
	if r := got["b"]; r.State != OK || r.Cert == nil || r.Cert.Problem != "cert_untrusted" || !r.Cert.NameMatch {
		t.Fatalf("b: %+v %+v", r, r.Cert)
	}
	if r := got["c"]; r.State != Down || r.Cert != nil || r.Cause != "refused" {
		t.Fatalf("c: %+v", r)
	}
}
