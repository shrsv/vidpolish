package server

import (
	"fmt"
	"net/http"

	"vidpolish/internal/config"
	"vidpolish/internal/ytauth"
)

type secretField struct {
	Set     bool   `json:"set"`
	Preview string `json:"preview,omitempty"`
}

func redactSecret(v string) secretField {
	if v == "" {
		return secretField{Set: false}
	}
	preview := v
	if len(v) > 8 {
		preview = v[:4] + "…" + v[len(v)-4:]
	}
	return secretField{Set: true, Preview: preview}
}

type configResponse struct {
	YouTube struct {
		ClientID            string      `json:"clientId"`
		ClientSecret        secretField `json:"clientSecret"`
		RefreshToken        secretField `json:"refreshToken"`
		Privacy             string      `json:"privacy"`
		DefaultLanguage     string      `json:"defaultLanguage"`
		DefaultTags         []string    `json:"defaultTags"`
		DescriptionTemplate string      `json:"descriptionTemplate"`
	} `json:"youtube"`
	Thumbnail struct {
		Enabled         bool   `json:"enabled"`
		LogoPath        string `json:"logoPath"`
		BackgroundColor string `json:"backgroundColor"`
		AccentColor     string `json:"accentColor"`
		TextColor       string `json:"textColor"`
	} `json:"thumbnail"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		// No config yet; report an all-empty response rather than an
		// error, so the UI can offer to create one.
		if _, initErr := config.Init(); initErr != nil {
			writeError(w, http.StatusInternalServerError, initErr)
			return
		}
		cfg, err = config.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, toConfigResponse(cfg))
}

func toConfigResponse(cfg *config.Config) *configResponse {
	var out configResponse
	out.YouTube.ClientID = cfg.YouTube.ClientID
	out.YouTube.ClientSecret = redactSecret(cfg.YouTube.ClientSecret)
	out.YouTube.RefreshToken = redactSecret(cfg.YouTube.RefreshToken)
	out.YouTube.Privacy = cfg.YouTube.Privacy
	out.YouTube.DefaultLanguage = cfg.YouTube.DefaultLanguage
	out.YouTube.DefaultTags = cfg.YouTube.DefaultTags
	out.YouTube.DescriptionTemplate = cfg.YouTube.DescriptionTemplate
	out.Thumbnail.Enabled = cfg.Thumbnail.Enabled
	out.Thumbnail.LogoPath = cfg.Thumbnail.LogoPath
	out.Thumbnail.BackgroundColor = cfg.Thumbnail.BackgroundColor
	out.Thumbnail.AccentColor = cfg.Thumbnail.AccentColor
	out.Thumbnail.TextColor = cfg.Thumbnail.TextColor
	return &out
}

// configUpdateRequest uses pointer fields so JSON null/absent means
// "leave unchanged" and an explicit value means "set it" — the UI never
// needs to round-trip a real secret value back to the server.
type configUpdateRequest struct {
	YouTube *struct {
		ClientID            *string   `json:"clientId"`
		ClientSecret        *string   `json:"clientSecret"`
		Privacy             *string   `json:"privacy"`
		DefaultLanguage     *string   `json:"defaultLanguage"`
		DefaultTags         *[]string `json:"defaultTags"`
		DescriptionTemplate *string   `json:"descriptionTemplate"`
	} `json:"youtube"`
	Thumbnail *struct {
		Enabled         *bool   `json:"enabled"`
		LogoPath        *string `json:"logoPath"`
		BackgroundColor *string `json:"backgroundColor"`
		AccentColor     *string `json:"accentColor"`
		TextColor       *string `json:"textColor"`
	} `json:"thumbnail"`
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var req configUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		if _, initErr := config.Init(); initErr != nil {
			writeError(w, http.StatusInternalServerError, initErr)
			return
		}
		cfg, err = config.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	if yt := req.YouTube; yt != nil {
		if yt.ClientID != nil {
			cfg.YouTube.ClientID = *yt.ClientID
		}
		if yt.ClientSecret != nil {
			cfg.YouTube.ClientSecret = *yt.ClientSecret
		}
		if yt.Privacy != nil {
			cfg.YouTube.Privacy = *yt.Privacy
		}
		if yt.DefaultLanguage != nil {
			cfg.YouTube.DefaultLanguage = *yt.DefaultLanguage
		}
		if yt.DefaultTags != nil {
			cfg.YouTube.DefaultTags = *yt.DefaultTags
		}
		if yt.DescriptionTemplate != nil {
			cfg.YouTube.DescriptionTemplate = *yt.DescriptionTemplate
		}
	}
	if th := req.Thumbnail; th != nil {
		if th.Enabled != nil {
			cfg.Thumbnail.Enabled = *th.Enabled
		}
		if th.LogoPath != nil {
			cfg.Thumbnail.LogoPath = *th.LogoPath
		}
		if th.BackgroundColor != nil {
			cfg.Thumbnail.BackgroundColor = *th.BackgroundColor
		}
		if th.AccentColor != nil {
			cfg.Thumbnail.AccentColor = *th.AccentColor
		}
		if th.TextColor != nil {
			cfg.Thumbnail.TextColor = *th.TextColor
		}
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, toConfigResponse(cfg))
}

const youtubeLoginEventKey = "__youtube_login__"

// handleYouTubeLogin starts the OAuth flow and returns the consent URL
// immediately; completion is reported over
// GET /api/youtube/login/events.
func (s *Server) handleYouTubeLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if cfg.YouTube.ClientID == "" || cfg.YouTube.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("set client_id and client_secret first"))
		return
	}

	authURL, wait, err := ytauth.StartLogin(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	go func() {
		refreshToken, err := wait()
		if err != nil {
			s.hub.Publish(youtubeLoginEventKey, "error: "+err.Error())
			return
		}
		cfg, loadErr := config.Load()
		if loadErr != nil {
			s.hub.Publish(youtubeLoginEventKey, "error: "+loadErr.Error())
			return
		}
		cfg.YouTube.RefreshToken = refreshToken
		if saveErr := config.Save(cfg); saveErr != nil {
			s.hub.Publish(youtubeLoginEventKey, "error: "+saveErr.Error())
			return
		}
		s.hub.Publish(youtubeLoginEventKey, "done")
	}()

	writeJSON(w, http.StatusOK, map[string]string{"authUrl": authURL})
}

func (s *Server) handleYouTubeLoginEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, cancel := s.hub.Subscribe(youtubeLoginEventKey)
	defer cancel()
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
