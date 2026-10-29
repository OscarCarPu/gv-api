package printers

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultConnectBase    = "https://connect.prusa3d.com"
	defaultConnectAccount = "https://account.prusa3d.com"
	connectClientID       = "MRHTlZhZqkNrrQ6FUPtjyusAz8nc59ErHXP8XkS4"
	connectScope          = "basic_info user_operations email_lists openid connect"
	connectTimeout        = 15 * time.Second
	connectBodyMax        = 4 << 20
	tokenMargin           = time.Minute
)

var (
	csrfField = regexp.MustCompile(`name="csrfmiddlewaretoken"[^>]*value="([^"]*)"`)
	nextField = regexp.MustCompile(`name="next"[^>]*value="([^"]*)"`)
)

type connectToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

var errVerification = errors.New("Prusa wants a code sent by email, run `make prusa-login` in gv-api")

type connectClient struct {
	user      string
	password  string
	printerID string
	base      string
	account   string
	tokenFile string

	mu      sync.Mutex
	access  string
	expires time.Time
	refresh string
}

func newConnectClient(p Printer) *connectClient {
	c := &connectClient{
		user:      p.ConnectUser,
		password:  p.ConnectPassword,
		printerID: p.ConnectID,
		base:      strings.TrimRight(p.ConnectBase, "/"),
		account:   strings.TrimRight(p.ConnectAccount, "/"),
		tokenFile: p.ConnectTokenFile,
	}
	if c.base == "" {
		c.base = defaultConnectBase
	}
	if c.account == "" {
		c.account = defaultConnectAccount
	}
	return c
}

func (c *connectClient) configured() bool {
	return c.user != "" && c.password != ""
}

func (c *connectClient) redirectURI() string {
	return defaultConnectBase + "/login/auth-callback"
}

func (c *connectClient) tokenFor(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" && time.Now().Before(c.expires) {
		return c.access, nil
	}
	tried := map[string]bool{"": true}
	for _, refresh := range []string{c.refresh, c.readRefresh()} {
		if tried[refresh] {
			continue
		}
		tried[refresh] = true
		tok, err := c.exchange(ctx, url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {refresh},
			"client_id":     {connectClientID},
		})
		if err == nil {
			c.store(tok)
			return c.access, nil
		}
	}
	c.refresh = ""
	tok, err := c.login(ctx, nil)
	if err != nil {
		return "", err
	}
	c.store(tok)
	return c.access, nil
}

func (c *connectClient) readRefresh() string {
	if c.tokenFile == "" {
		return ""
	}
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return ""
	}
	var saved struct {
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal(data, &saved) != nil {
		return ""
	}
	return saved.RefreshToken
}

func (c *connectClient) writeRefresh() {
	if c.tokenFile == "" || c.refresh == "" {
		return
	}
	data, err := json.Marshal(map[string]string{"refresh_token": c.refresh})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.tokenFile), 0o700); err != nil {
		slog.Warn("printers: could not save the Prusa Connect token", "error", err)
		return
	}
	if err := os.WriteFile(c.tokenFile, data, 0o600); err != nil {
		slog.Warn("printers: could not save the Prusa Connect token", "error", err)
	}
}

func (c *connectClient) forget() {
	c.mu.Lock()
	c.access = ""
	c.mu.Unlock()
}

func (c *connectClient) store(t connectToken) {
	c.access = t.AccessToken
	c.expires = time.Now().Add(time.Duration(t.ExpiresIn)*time.Second - tokenMargin)
	if t.RefreshToken != "" {
		c.refresh = t.RefreshToken
		c.writeRefresh()
	}
}

func (c *connectClient) exchange(ctx context.Context, form url.Values) (connectToken, error) {
	var tok connectToken
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.account+"/o/token/", strings.NewReader(form.Encode()))
	if err != nil {
		return tok, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return tok, err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, connectBodyMax))
	if res.StatusCode != http.StatusOK {
		return tok, fmt.Errorf("token endpoint returned %d", res.StatusCode)
	}
	if err := json.Unmarshal(data, &tok); err != nil || tok.AccessToken == "" {
		return tok, errors.New("token endpoint returned no access token")
	}
	return tok, nil
}

func pkcePair() (verifier, challenge string, err error) {
	raw := make([]byte, 64)
	if _, err = rand.Read(raw); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

func (c *connectClient) login(ctx context.Context, askCode func() (string, error)) (connectToken, error) {
	verifier, challenge, err := pkcePair()
	if err != nil {
		return connectToken{}, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return connectToken{}, err
	}
	hc := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	authorize := c.account + "/o/authorize/?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {connectClientID},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"redirect_uri":          {c.redirectURI()},
		"scope":                 {connectScope},
	}.Encode()

	res, body, err := c.step(ctx, hc, http.MethodGet, authorize, nil)
	if err != nil {
		return connectToken{}, err
	}
	loginURL, err := res.Location()
	if err != nil {
		return connectToken{}, errors.New("Prusa did not ask for a login")
	}
	if _, body, err = c.step(ctx, hc, http.MethodGet, loginURL.String(), nil); err != nil {
		return connectToken{}, err
	}
	csrf := csrfField.FindStringSubmatch(body)
	if csrf == nil {
		return connectToken{}, errors.New("Prusa login form has changed")
	}
	form := url.Values{
		"csrfmiddlewaretoken": {html.UnescapeString(csrf[1])},
		"email":               {c.user},
		"password":            {c.password},
	}
	if next := nextField.FindStringSubmatch(body); next != nil {
		form.Set("next", html.UnescapeString(next[1]))
	}
	res, _, err = c.step(ctx, hc, http.MethodPost, loginURL.String(), form)
	if err != nil {
		return connectToken{}, err
	}
	if res.StatusCode != http.StatusFound {
		return connectToken{}, errors.New("Prusa rejected the account email or password")
	}
	if verify, err := res.Location(); err == nil && strings.HasPrefix(verify.Path, "/login/verify") {
		if askCode == nil {
			return connectToken{}, errVerification
		}
		if err := c.verify(ctx, hc, verify.String(), askCode); err != nil {
			return connectToken{}, err
		}
	}

	res, _, err = c.step(ctx, hc, http.MethodGet, authorize, nil)
	if err != nil {
		return connectToken{}, err
	}
	back, err := res.Location()
	if err != nil || back.Query().Get("code") == "" {
		return connectToken{}, errors.New("Prusa did not return an authorization code")
	}
	return c.exchange(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {back.Query().Get("code")},
		"client_id":     {connectClientID},
		"redirect_uri":  {c.redirectURI()},
		"code_verifier": {verifier},
	})
}

func (c *connectClient) verify(ctx context.Context, hc *http.Client, target string, askCode func() (string, error)) error {
	_, body, err := c.step(ctx, hc, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	csrf := csrfField.FindStringSubmatch(body)
	if csrf == nil {
		return errors.New("Prusa verification form has changed")
	}
	code, err := askCode()
	if err != nil {
		return err
	}
	form := url.Values{
		"csrfmiddlewaretoken": {html.UnescapeString(csrf[1])},
		"otp_token":           {code},
	}
	if next := nextField.FindStringSubmatch(body); next != nil {
		form.Set("next", html.UnescapeString(next[1]))
	}
	res, _, err := c.step(ctx, hc, http.MethodPost, target, form)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusFound {
		return errors.New("Prusa did not accept that code")
	}
	return nil
}

func LoginConnect(ctx context.Context, p Printer, in io.Reader, out io.Writer) error {
	c := newConnectClient(p)
	if !c.configured() {
		return errors.New("PRUSA_CONNECT_USER and PRUSA_CONNECT_PASSWORD are not set")
	}
	reader := bufio.NewReader(in)
	tok, err := c.login(ctx, func() (string, error) {
		_, _ = fmt.Fprint(out, "Prusa sent a code to your email. Code: ")
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return "", errors.New("no code entered")
		}
		return strings.TrimSpace(line), nil
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.store(tok)
	c.mu.Unlock()
	return nil
}

func (c *connectClient) step(ctx context.Context, hc *http.Client, method, target string, form url.Values) (*http.Response, string, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, "", err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", target)
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, connectBodyMax))
	return res, string(data), nil
}

func (c *connectClient) get(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	for attempt := 0; ; attempt++ {
		token, err := c.tokenFor(ctx)
		if err != nil {
			return nil, &statusError{http.StatusBadGateway, "Could not log in to Prusa Connect: " + err.Error()}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, &statusError{http.StatusBadGateway, "Prusa Connect is unreachable"}
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, connectBodyMax))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusUnauthorized && attempt == 0:
			c.forget()
			continue
		case res.StatusCode != http.StatusOK:
			return nil, &statusError{http.StatusBadGateway, fmt.Sprintf("Prusa Connect returned %d", res.StatusCode)}
		case readErr != nil:
			return nil, &statusError{http.StatusBadGateway, "Prusa Connect response was cut short"}
		}
		return data, nil
	}
}

func (c *connectClient) printerUUID(ctx context.Context) (string, error) {
	c.mu.Lock()
	id := c.printerID
	c.mu.Unlock()
	if id != "" {
		return id, nil
	}
	data, err := c.get(ctx, "/app/printers")
	if err != nil {
		return "", err
	}
	var list struct {
		Printers []struct {
			UUID string `json:"uuid"`
		} `json:"printers"`
	}
	if err := json.Unmarshal(data, &list); err != nil || len(list.Printers) == 0 {
		return "", &statusError{http.StatusBadGateway, "No printer found on the Prusa Connect account"}
	}
	c.mu.Lock()
	c.printerID = list.Printers[0].UUID
	c.mu.Unlock()
	return list.Printers[0].UUID, nil
}
