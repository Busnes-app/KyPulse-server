package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/crypto"
)

// PoWChallenge represents a client-side SHA256 puzzle matching the KySecurity standard.
type PoWChallenge struct {
	Algorithm string `json:"algorithm"`
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
	MaxNumber int    `json:"maxnumber"`
	ExpiresAt int64  `json:"expires_at"`
	Signature string `json:"signature"`
}

// PoWSolution represents the client's submitted answer.
type PoWSolution struct {
	Algorithm string `json:"algorithm"`
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
	Number    int    `json:"number"`
	MaxNumber int    `json:"maxnumber"`
	ExpiresAt int64  `json:"expires_at"`
	Signature string `json:"signature"`
}

// GeneratePoWChallenge creates a puzzle of given difficulty (e.g. 5000 to 50000 max iterations).
func GeneratePoWChallenge(difficulty int, signingSecret string) (*PoWChallenge, error) {
	if difficulty <= 0 {
		difficulty = 20000
	}

	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return nil, err
	}
	salt := hex.EncodeToString(saltBytes)

	// Pick a random winning number in range [1, difficulty]
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(difficulty)))
	if err != nil {
		return nil, err
	}
	targetNumber := int(nBig.Int64()) + 1

	targetStr := fmt.Sprintf("%s%d", salt, targetNumber)
	h := sha256.Sum256([]byte(targetStr))
	challenge := hex.EncodeToString(h[:])

	challengeExpires := time.Now().Add(5 * time.Minute).Unix()
	signaturePayload := fmt.Sprintf("%s:%s:%d:%d", salt, challenge, difficulty, challengeExpires)
	return &PoWChallenge{
		Algorithm: "SHA-256",
		Salt:      salt,
		Challenge: challenge,
		MaxNumber: difficulty,
		ExpiresAt: challengeExpires,
		Signature: crypto.ComputeHMACSHA256([]byte(signaturePayload), signingSecret),
	}, nil
}

// powUsedCap bounds the replay set. Each entry costs the caller a solved puzzle, and evicting
// one re-opens a single replay only after another fresh solve, so eviction never beats solving.
const powUsedCap = 10000

// PoWGuard verifies solutions and accepts each signed challenge once, until it expires.
type PoWGuard struct {
	mu   sync.Mutex
	used map[string]int64 // salt -> challenge expiry (unix seconds)
}

func NewPoWGuard() *PoWGuard { return &PoWGuard{used: make(map[string]int64)} }

// Verify decodes and validates the client's base64-encoded JSON solution and consumes it.
func (g *PoWGuard) Verify(solutionBase64, signingSecret string) bool {
	sol, ok := checkPoWSolution(solutionBase64, signingSecret)
	if !ok {
		return false
	}
	now := time.Now().Unix()
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, seen := g.used[sol.Salt]; seen {
		return false
	}
	if len(g.used) >= powUsedCap {
		for salt, exp := range g.used {
			if exp < now {
				delete(g.used, salt)
			}
		}
		for salt := range g.used {
			if len(g.used) < powUsedCap {
				break
			}
			delete(g.used, salt) // random: Go randomises map iteration
		}
	}
	g.used[sol.Salt] = sol.ExpiresAt
	return true
}

func checkPoWSolution(solutionBase64, signingSecret string) (PoWSolution, bool) {
	data, err := base64.StdEncoding.DecodeString(solutionBase64)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(solutionBase64)
		if err != nil {
			return PoWSolution{}, false
		}
	}

	var sol PoWSolution
	if err := json.Unmarshal(data, &sol); err != nil {
		return PoWSolution{}, false
	}

	if sol.Algorithm != "SHA-256" || sol.Salt == "" || sol.Challenge == "" || sol.Signature == "" ||
		sol.Number < 1 || sol.MaxNumber < 1 || sol.Number > sol.MaxNumber || sol.ExpiresAt < time.Now().Unix() {
		return PoWSolution{}, false
	}
	signaturePayload := sol.Salt + ":" + sol.Challenge + ":" + strconv.Itoa(sol.MaxNumber) + ":" + strconv.FormatInt(sol.ExpiresAt, 10)
	if !crypto.VerifyHMACSHA256([]byte(signaturePayload), signingSecret, sol.Signature) {
		return PoWSolution{}, false
	}

	targetStr := fmt.Sprintf("%s%d", sol.Salt, sol.Number)
	h := sha256.Sum256([]byte(targetStr))
	return sol, hex.EncodeToString(h[:]) == sol.Challenge
}
