package auth

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func solvedPoW(t *testing.T, secret string) string {
	t.Helper()
	c, err := GeneratePoWChallenge(1, secret) // difficulty 1: the answer is always 1
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(PoWSolution{Algorithm: c.Algorithm, Salt: c.Salt, Challenge: c.Challenge, Number: 1, MaxNumber: c.MaxNumber, ExpiresAt: c.ExpiresAt, Signature: c.Signature})
	return base64.StdEncoding.EncodeToString(raw)
}

func TestPoWGuardStaysBounded(t *testing.T) {
	g := NewPoWGuard()
	past, future := time.Now().Add(-time.Minute).Unix(), time.Now().Add(time.Minute).Unix()
	for i := range powUsedCap {
		exp := future
		if i%2 == 0 {
			exp = past
		}
		g.used["filler"+strconv.Itoa(i)] = exp
	}
	if !g.Verify(solvedPoW(t, "s"), "s") {
		t.Fatal("a full guard refused a fresh solution")
	}
	if n := len(g.used); n > powUsedCap/2+1 {
		t.Fatalf("expired salts not pruned: %d entries", n)
	}

	for i := range powUsedCap {
		g.used["live"+strconv.Itoa(i)] = future
	}
	if !g.Verify(solvedPoW(t, "s"), "s") || len(g.used) > powUsedCap {
		t.Fatalf("guard exceeded its cap: %d", len(g.used))
	}
}
