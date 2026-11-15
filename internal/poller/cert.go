package poller

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"
)

// Cert is the leaf certificate a poll's own handshake presented. Problem is set when the
// handshake refused it, in the order Go verifies: expiry, then name, then chain.
type Cert struct {
	NotAfter  time.Time `json:"not_after"`
	Issuer    string    `json:"issuer,omitempty"`
	NameMatch bool      `json:"name_match"`
	Problem   string    `json:"problem,omitempty"` // cert_expired|cert_not_yet_valid|cert_name_mismatch|cert_untrusted
}

// maxIssuer caps the issuer text: the watched endpoint chooses it.
const maxIssuer = 200

// CertOf reads the leaf from a verified handshake, or from the unverified chain a failed
// verification carries. host is the name the handshake checked. Nil when there was no
// certificate: plain http, or a failure before the server sent one.
func CertOf(state *tls.ConnectionState, err error, host string) *Cert {
	var leaf *x509.Certificate
	var verr *tls.CertificateVerificationError
	switch {
	case err == nil && state != nil && len(state.PeerCertificates) > 0:
		leaf = state.PeerCertificates[0]
	case errors.As(err, &verr) && len(verr.UnverifiedCertificates) > 0:
		leaf = verr.UnverifiedCertificates[0]
	default:
		return nil
	}
	issuer := leaf.Issuer.CommonName
	if issuer == "" {
		issuer = leaf.Issuer.String()
	}
	if len(issuer) > maxIssuer {
		issuer = issuer[:maxIssuer]
	}
	c := &Cert{NotAfter: leaf.NotAfter.UTC(), Issuer: issuer, NameMatch: leaf.VerifyHostname(host) == nil}
	if verr != nil {
		c.Problem = problem(verr.Err, leaf)
	}
	return c
}

func problem(err error, leaf *x509.Certificate) string {
	var invalid x509.CertificateInvalidError
	var hostErr x509.HostnameError
	switch {
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		if time.Now().Before(leaf.NotBefore) {
			return "cert_not_yet_valid"
		}
		return "cert_expired"
	case errors.As(err, &hostErr):
		return "cert_name_mismatch"
	}
	return "cert_untrusted"
}
