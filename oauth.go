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

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	// gmail.send to send; email (OpenID) to read the account address after auth.
	gmailScope = "https://www.googleapis.com/auth/gmail.send email"
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	state := newID()

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
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch — possible CSRF")
			http.Error(w, "Bad state", 400)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("google: %s", e)
			fmt.Fprintf(w, "<html><body><h2>Authentication failed: %s. You can close this tab.</h2></body></html>", e)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "No code", 400)
			return
		}
		fmt.Fprint(w, "<html><body><h2>Authenticated! You can close this tab.</h2></body></html>")
		codeCh <- code
	})

	go srv.Serve(listener) //nolint:errcheck
	defer srv.Close()

	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return "", err
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("OAuth timed out after 5 minutes")
	}

	token, err := exchangeCodeForToken(clientID, clientSecret, code, redirectURI)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	if err := saveOAuthToken(token); err != nil {
		return "", fmt.Errorf("save token: %w", err)
	}

	email, _ := fetchUserEmail(token.AccessToken)
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
