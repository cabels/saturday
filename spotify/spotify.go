package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	State        string
}

type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

func New(clientID, clientSecret, redirectURI string, scopes []string, state string) *Client {
	return &Client{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		Scopes:       scopes,
		State:        state,
	}
}

func (c *Client) AuthCode() (*Token, error) {
	codeCh := make(chan string)
	server := &http.Server{Addr: getHostPort(c.RedirectURI)}

	http.HandleFunc(getPath(c.RedirectURI), func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != c.State {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		fmt.Fprintf(w, "Authorization successful! You can close this tab.")
		codeCh <- code
		go func() {
			time.Sleep(1 * time.Second)
			server.Shutdown(context.Background())
		}()
	})

	go func() {
		ln, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return
		}
		server.Serve(ln)
	}()

	authURL := buildAuthURL(c.ClientID, c.RedirectURI, c.Scopes, c.State)
	openBrowser(authURL)

	slog.Info("Please authorize the application in your browser", "url", authURL)
	code := <-codeCh

	return exchangeCodeForToken(c.ClientID, c.ClientSecret, c.RedirectURI, code)
}

func buildAuthURL(clientID, redirectURI string, scopes []string, state string) string {
	scope := strings.Join(scopes, " ")
	return fmt.Sprintf(
		"https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&state=%s&scope=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		state,
		url.QueryEscape(scope),
	)
}

func exchangeCodeForToken(clientID, clientSecret, redirectURI, code string) (*Token, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var token Token
	err = json.NewDecoder(resp.Body).Decode(&token)
	return &token, err
}

func openBrowser(url string) {
	_ = exec.Command("open", url).Start()
}

func getHostPort(redirectURI string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "127.0.0.1:8888"
	}
	return u.Host
}

func getPath(redirectURI string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "/callback"
	}
	return u.Path
}

func SaveToken(filename string, token *Token) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

func LoadToken(filename string) (*Token, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var token Token
	err = json.NewDecoder(f).Decode(&token)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (c *Client) RefreshAccessToken(refreshToken string) (*Token, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.ClientID, c.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var token Token
	err = json.NewDecoder(resp.Body).Decode(&token)
	if err != nil {
		return nil, err
	}

	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	return &token, nil
}

func Play(accessToken string) error {
	req, _ := http.NewRequest("PUT", "https://api.spotify.com/v1/me/player/play", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("play failed: status %d", resp.StatusCode)
	}
	return nil
}

func Pause(accessToken string) error {
	req, _ := http.NewRequest("PUT", "https://api.spotify.com/v1/me/player/pause", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("pause failed: status %d", resp.StatusCode)
	}
	return nil
}

func PlayTrack(accessToken, trackURI string) error {
	body := map[string]interface{}{
		"uris": []string{trackURI},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, _ := http.NewRequest("PUT", "https://api.spotify.com/v1/me/player/play", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("play track failed: status %d", resp.StatusCode)
	}
	return nil
}

type SearchResult struct {
	Tracks struct {
		Items []struct {
			Name    string `json:"name"`
			URI     string `json:"uri"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"items"`
	} `json:"tracks"`
}

func SearchTrack(accessToken, query string) (string, error) {
	q := url.QueryEscape(query)
	url := fmt.Sprintf("https://api.spotify.com/v1/search?type=track&limit=1&q=%s", q)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search failed: status %d", resp.StatusCode)
	}

	var result SearchResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if len(result.Tracks.Items) == 0 {
		return "", fmt.Errorf("no tracks found for '%s'", query)
	}

	return result.Tracks.Items[0].URI, nil
}
