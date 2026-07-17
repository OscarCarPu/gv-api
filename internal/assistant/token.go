package assistant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gv-api/internal/assistant/llm"
)

// tokenPayload is the server-authored, signed content of a suggestion. The
// client treats the encoded token as opaque and never reshapes it, so the
// signature is verified over exactly the bytes the server emitted.
type tokenPayload struct {
	Kind         string          `json:"k"`
	SQL          string          `json:"q,omitempty"`
	Action       *llm.ActionCall `json:"a,omitempty"`
	Explanation  string          `json:"e"`
	NeedsSummary bool            `json:"s,omitempty"`
	IssuedAt     int64           `json:"iat"`
}

// tokenizer signs and verifies suggestion tokens with an HMAC secret.
type tokenizer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func newTokenizer(secret string, ttl time.Duration) *tokenizer {
	return &tokenizer{secret: []byte(secret), ttl: ttl, now: time.Now}
}

// sign encodes and signs a decision into an opaque token string.
func (t *tokenizer) sign(d llm.Decision) (string, error) {
	p := tokenPayload{
		Kind:         d.Kind,
		SQL:          d.SQL,
		Action:       d.Action,
		Explanation:  d.Explanation,
		NeedsSummary: d.NeedsSummary,
		IssuedAt:     t.now().Unix(),
	}
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(body)
	sig := t.mac(b64)
	return b64 + "." + sig, nil
}

// verify checks the signature and expiry and returns the decoded payload.
func (t *tokenizer) verify(token string) (tokenPayload, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return tokenPayload{}, ErrBadToken
	}
	expected := t.mac(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return tokenPayload{}, ErrBadToken
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenPayload{}, ErrBadToken
	}
	var p tokenPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return tokenPayload{}, ErrBadToken
	}
	if t.ttl > 0 {
		age := t.now().Sub(time.Unix(p.IssuedAt, 0))
		if age < 0 || age > t.ttl {
			return tokenPayload{}, fmt.Errorf("%w: expired", ErrBadToken)
		}
	}
	return p, nil
}

func (t *tokenizer) mac(msg string) string {
	m := hmac.New(sha256.New, t.secret)
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
