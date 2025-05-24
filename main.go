package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/cabels/saturday/hue"
	"github.com/cabels/saturday/openai"
	"github.com/cabels/saturday/spotify"
)

const tokenFile = "token.json"

type Config struct {
	BridgeIP            string `json:"bridge_ip"`
	Username            string `json:"username"`
	DefaultLight        string `json:"default_light"`
	OpenAIKey           string `json:"openai_api_key"`
	SpotifyClientID     string `json:"spotify_client_id"`
	SpotifyClientSecret string `json:"spotify_client_secret"`
}

func loadCfg() (Config, error) {
	cfgPath := os.Getenv("CONFIG")
	if cfgPath == "" {
		cfgPath = "config.json"
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.BridgeIP == "" || cfg.Username == "" || cfg.DefaultLight == "" || cfg.OpenAIKey == "" || cfg.SpotifyClientID == "" || cfg.SpotifyClientSecret == "" {
		return Config{}, fmt.Errorf("config file missing required fields: bridge_ip, username, default_light, openai_api_key, spotify_client_id, spotify_client_secret")
	}
	return cfg, nil
}

func main() {
	cfg, err := loadCfg()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	var prompt string
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	oaic := openai.New(cfg.OpenAIKey)
	result, err := oaic.Post(prompt)
	if err != nil {
		slog.Error("Failed to get light state from OpenAI", "error", err)
		os.Exit(1)
	}
	slog.Info("Received light state from OpenAI", "bri", result.Bri, "on", result.On, "xy", result.XY)

	huec := hue.New(cfg.BridgeIP, cfg.Username, cfg.DefaultLight)
	if err := huec.SetState(result.On, result.Bri, result.XY); err != nil {
		slog.Error("Failed to set light state on Hue bridge", "error", err)
		os.Exit(1)
	}
	slog.Info("Sent light state to Hue bridge", "bri", result.Bri, "on", result.On, "xy", result.XY)

	spoc := spotify.New(cfg.SpotifyClientID, cfg.SpotifyClientSecret, "http://127.0.0.1:8888/callback", []string{"user-read-private", "user-read-email", "user-modify-playback-state", "user-read-playback-state"}, "some-random-state")
	token, err := spotify.LoadToken(tokenFile)
	if err != nil {
		slog.Warn("Warning: failed to load token, will need to re-authenticate", "error", err)
		token, err = spoc.AuthCode()
		if err != nil {
			slog.Error("Auth failed", "error", err)
			os.Exit(1)
		}
		err = spotify.SaveToken(tokenFile, token)
		if err != nil {
			slog.Warn("Warning: failed to save token", "error", err)
		}
	} else {
		token, err = spoc.RefreshAccessToken(token.RefreshToken)
		if err != nil {
			log.Println("Token refresh failed, need to re-authenticate:", err)
			token, err = spoc.AuthCode()
			if err != nil {
				slog.Error("Auth failed", "error", err)
				os.Exit(1)
			}
		}
		err = spotify.SaveToken(tokenFile, token)
		if err != nil {
			slog.Warn("Warning: failed to save token", "error", err)
		}
	}
	if result.SongTitle != "" || result.Artist != "" {
		var query string
		if result.SongTitle != "" && result.Artist != "" {
			query = result.SongTitle + " " + result.Artist
		} else if result.SongTitle != "" {
			query = result.SongTitle
		} else {
			query = result.Artist
		}

		uri, err := spotify.SearchTrack(token.AccessToken, query)
		if err != nil {
			slog.Error("Failed to search for track", "query", query, "error", err)
			os.Exit(1)
		}
		if err := spotify.PlayTrack(token.AccessToken, uri); err != nil {
			slog.Error("Failed to play track on Spotify", "uri", uri, "error", err)
			os.Exit(1)
		} else {
			slog.Info("Playing track on Spotify", "uri", uri)
		}
	}

	os.Exit(0)
}
