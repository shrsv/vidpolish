// Package ytauth implements the OAuth 2.0 installed-app flow needed to
// upload videos to YouTube. A plain Google API key is not enough for
// videos.insert (it is a write operation on a user's channel), so this
// package drives a one-time browser consent and exchanges the resulting
// code for a refresh token, then mints short-lived access tokens from
// that refresh token on every later run.
package ytauth

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vidpolish/internal/browseropen"
)

// UploadScope is the OAuth scope required to upload videos.
const UploadScope = "https://www.googleapis.com/auth/youtube.upload"

const (
	authEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenEndpoint = "https://oauth2.googleapis.com/token"
)

// Login runs the installed-app OAuth flow: it starts a local redirect
// listener, opens the consent page in the user's browser, waits for the
// authorization code, and exchanges it for a refresh token.
func Login(clientID, clientSecret string) (refreshToken string, err error) {
	_, wait, err := StartLogin(clientID, clientSecret)
	if err != nil {
		return "", err
	}
	return wait()
}

// StartLogin begins the installed-app OAuth flow: it starts a local
// redirect listener and returns the consent URL immediately (having
// already made a best-effort attempt to open it in a browser), without
// waiting for the user to complete it. Call the returned wait function to
// block until the user finishes (or 5 minutes elapse), exchanging the
// resulting code for a refresh token. This split lets a caller (e.g. a
// web UI) show/relay the URL right away instead of blocking on it.
func StartLogin(clientID, clientSecret string) (authURL string, wait func() (string, error), err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("starting local redirect listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			fmt.Fprintln(w, "vidpolish: authorization denied, you can close this tab.")
			errCh <- fmt.Errorf("authorization denied: %s", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			fmt.Fprintln(w, "vidpolish: missing authorization code, you can close this tab.")
			errCh <- fmt.Errorf("no authorization code in callback")
			return
		}
		fmt.Fprintln(w, "vidpolish: authorization complete, you can close this tab.")
		codeCh <- code
	})
	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	authURL = buildAuthURL(clientID, redirectURI)
	fmt.Println("==> opening browser for YouTube authorization:")
	fmt.Println("   ", authURL)
	if err := browseropen.Open(authURL); err != nil {
		fmt.Println("==> could not open a browser automatically; open the URL above manually")
	}

	wait = func() (string, error) {
		defer server.Close()
		defer listener.Close()
		select {
		case code := <-codeCh:
			return exchangeCode(clientID, clientSecret, code, redirectURI)
		case err := <-errCh:
			return "", err
		case <-time.After(5 * time.Minute):
			return "", fmt.Errorf("timed out waiting for authorization")
		}
	}
	return authURL, wait, nil
}

func buildAuthURL(clientID, redirectURI string) string {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("response_type", "code")
	v.Set("scope", UploadScope)
	v.Set("access_type", "offline")
	v.Set("prompt", "consent")
	return authEndpoint + "?" + v.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func exchangeCode(clientID, clientSecret, code, redirectURI string) (string, error) {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("client_secret", clientSecret)
	v.Set("code", code)
	v.Set("redirect_uri", redirectURI)
	v.Set("grant_type", "authorization_code")

	tok, err := postToken(v)
	if err != nil {
		return "", err
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("no refresh token returned; revoke prior access at https://myaccount.google.com/permissions and try again")
	}
	return tok.RefreshToken, nil
}

// AccessToken exchanges a stored refresh token for a short-lived access
// token to use as a Bearer credential on API requests.
func AccessToken(clientID, clientSecret, refreshToken string) (string, error) {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("client_secret", clientSecret)
	v.Set("refresh_token", refreshToken)
	v.Set("grant_type", "refresh_token")

	tok, err := postToken(v)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func postToken(v url.Values) (*tokenResponse, error) {
	resp, err := http.Post(tokenEndpoint, "application/x-www-form-urlencoded", strings.NewReader(v.Encode()))
	if err != nil {
		return nil, fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if tok.Error != "" {
		return nil, fmt.Errorf("token error: %s (%s)", tok.Error, tok.ErrorDesc)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: %s", resp.Status)
	}
	return &tok, nil
}
