package assistant

import (
	"encoding/json"
	"testing"
	"time"

	"gv-api/internal/assistant/llm"

	"github.com/stretchr/testify/require"
)

func TestTokenizer_SignVerifyRoundtrip(t *testing.T) {
	tok := newTokenizer("secret", time.Minute)
	d := llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1", Explanation: "uno", NeedsSummary: true}

	token, err := tok.sign(d)
	require.NoError(t, err)

	p, err := tok.verify(token)
	require.NoError(t, err)
	require.Equal(t, llm.KindRead, p.Kind)
	require.Equal(t, "SELECT 1", p.SQL)
	require.Equal(t, "uno", p.Explanation)
	require.True(t, p.NeedsSummary)
}

func TestTokenizer_SignVerifyWriteAction(t *testing.T) {
	tok := newTokenizer("secret", time.Minute)
	d := llm.Decision{
		Kind:        llm.KindWrite,
		Explanation: "crea tarea",
		Action:      &llm.ActionCall{Domain: "tasks", Operation: "create_task", Args: json.RawMessage(`{"name":"x"}`)},
	}
	token, err := tok.sign(d)
	require.NoError(t, err)

	p, err := tok.verify(token)
	require.NoError(t, err)
	require.NotNil(t, p.Action)
	require.Equal(t, "tasks", p.Action.Domain)
	require.JSONEq(t, `{"name":"x"}`, string(p.Action.Args))
}

func TestTokenizer_TamperedTokenRejected(t *testing.T) {
	tok := newTokenizer("secret", time.Minute)
	token, err := tok.sign(llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1"})
	require.NoError(t, err)

	// Flip the last character of the signature.
	bad := token[:len(token)-1] + string(flip(token[len(token)-1]))
	_, err = tok.verify(bad)
	require.ErrorIs(t, err, ErrBadToken)
}

func TestTokenizer_WrongSecretRejected(t *testing.T) {
	a := newTokenizer("secret-a", time.Minute)
	b := newTokenizer("secret-b", time.Minute)
	token, err := a.sign(llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1"})
	require.NoError(t, err)
	_, err = b.verify(token)
	require.ErrorIs(t, err, ErrBadToken)
}

func TestTokenizer_ExpiredTokenRejected(t *testing.T) {
	tok := newTokenizer("secret", time.Minute)
	base := time.Now()
	tok.now = func() time.Time { return base }
	token, err := tok.sign(llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1"})
	require.NoError(t, err)

	tok.now = func() time.Time { return base.Add(2 * time.Minute) }
	_, err = tok.verify(token)
	require.ErrorIs(t, err, ErrBadToken)
}

func TestTokenizer_MalformedRejected(t *testing.T) {
	tok := newTokenizer("secret", time.Minute)
	for _, s := range []string{"", "nodot", "a.", ".b", "not-base64!.sig"} {
		_, err := tok.verify(s)
		require.ErrorIs(t, err, ErrBadToken, "input %q", s)
	}
}

func flip(b byte) byte {
	if b == 'A' {
		return 'B'
	}
	return 'A'
}
