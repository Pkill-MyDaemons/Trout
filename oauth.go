package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func oauthLog(format string, args ...any) {
	f, err := os.OpenFile(dataPath("oauth.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	// gmail.modify: send + read + label changes. calendar: read/write events. email: account address.
	gmailScope = "https://www.googleapis.com/auth/gmail.modify https://www.googleapis.com/auth/calendar email"
)

type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// oauthDoneMsg is sent back to the TUI when the OAuth flow finishes.
type oauthDoneMsg struct {
	email string // auto-detected Gmail address on success
	err   error
}

func oauthTokenPath() string { return dataPath("oauth_token.json") }

func loadOAuthToken() (*OAuthToken, error) {
	data, err := os.ReadFile(oauthTokenPath())
	if err != nil {
		return nil, err
	}
	var t OAuthToken
	return &t, json.Unmarshal(data, &t)
}

func saveOAuthToken(t *OAuthToken) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(oauthTokenPath(), data, 0600)
}

func isOAuthAuthenticated() bool {
	t, err := loadOAuthToken()
	return err == nil && t.RefreshToken != ""
}

func openBrowser(u string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{u}
	case "linux":
		cmd, args = "xdg-open", []string{u}
	default:
		cmd, args = "cmd", []string{"/c", "start", u}
	}
	exec.Command(cmd, args...).Start() //nolint:errcheck
}

// startOAuthFlowCmd returns a tea.Cmd that runs the browser OAuth flow
// in the background and sends an oauthDoneMsg when finished.
func startOAuthFlowCmd(clientID, clientSecret string) tea.Cmd {
	return func() tea.Msg {
		email, err := runOAuthFlow(clientID, clientSecret)
		return oauthDoneMsg{email: email, err: err}
	}
}

func runOAuthFlow(clientID, clientSecret string) (string, error) {
	oauthLog("starting OAuth flow")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		oauthLog("ERROR: listen: %v", err)
		return "", fmt.Errorf("start local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	state := newID()
	oauthLog("listening on port %d, state=%s", port, state)

	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {gmailScope},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	authURL := googleAuthURL + "?" + params.Encode()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		oauthLog("callback hit: %s", r.URL.RawQuery)
		gotState := r.URL.Query().Get("state")
		if gotState != state {
			oauthLog("ERROR: state mismatch got=%s want=%s", gotState, state)
			errCh <- fmt.Errorf("state mismatch — possible CSRF")
			http.Error(w, "Bad state", 400)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			oauthLog("ERROR: google returned error: %s", e)
			errCh <- fmt.Errorf("google: %s", e)
			fmt.Fprintf(w, "<html><body><h2>Authentication failed: %s. You can close this tab.</h2></body></html>", e)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			oauthLog("ERROR: no code in callback")
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "No code", 400)
			return
		}
		oauthLog("got code (len=%d), sending success page", len(code))
		fmt.Fprint(w, "<html><body><h2>Authenticated! You can close this tab.</h2></body></html>")
		codeCh <- code
	})

	go srv.Serve(listener) //nolint:errcheck
	defer srv.Close()

	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
		oauthLog("received code from channel")
	case err = <-errCh:
		oauthLog("received error from channel: %v", err)
		return "", err
	case <-time.After(5 * time.Minute):
		oauthLog("ERROR: timed out waiting for callback")
		return "", fmt.Errorf("OAuth timed out after 5 minutes")
	}

	oauthLog("exchanging code for token")
	token, err := exchangeCodeForToken(clientID, clientSecret, code, redirectURI)
	if err != nil {
		oauthLog("ERROR: token exchange: %v", err)
		return "", fmt.Errorf("token exchange: %w", err)
	}
	oauthLog("token exchange succeeded, expiry=%s", token.Expiry.Format(time.RFC3339))

	if err := saveOAuthToken(token); err != nil {
		oauthLog("ERROR: save token: %v", err)
		return "", fmt.Errorf("save token: %w", err)
	}
	oauthLog("token saved to %s", oauthTokenPath())

	email, err := fetchUserEmail(token.AccessToken)
	if err != nil {
		oauthLog("WARN: could not fetch user email: %v", err)
	} else {
		oauthLog("user email: %s", email)
	}
	return email, nil
}

func exchangeCodeForToken(clientID, clientSecret, code, redirectURI string) (*OAuthToken, error) {
	return postTokenRequest(url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	})
}

func refreshAccessTokenWith(clientID, clientSecret, refreshToken string) (*OAuthToken, error) {
	t, err := postTokenRequest(url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return nil, err
	}
	if t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, nil
}

func postTokenRequest(vals url.Values) (*OAuthToken, error) {
	req, err := http.NewRequest("POST", googleTokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s: %s", result.Error, result.ErrorDesc)
	}
	return &OAuthToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

func fetchUserEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v1/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Email, nil
}

// getValidAccessToken returns a fresh access token, refreshing if expired.
func getValidAccessToken(cfg *Config) (string, error) {
	t, err := loadOAuthToken()
	if err != nil {
		return "", fmt.Errorf("not authenticated — open config and press Authenticate Gmail")
	}
	if time.Now().After(t.Expiry.Add(-60 * time.Second)) {
		t, err = refreshAccessTokenWith(cfg.GmailClientID, cfg.GmailClientSecret, t.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("token refresh: %w", err)
		}
		_ = saveOAuthToken(t)
	}
	return t.AccessToken, nil
}
