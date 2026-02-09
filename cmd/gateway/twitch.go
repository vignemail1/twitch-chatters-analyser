package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// exchangeCodeForToken échange un code OAuth contre un token d'accès
func (a *App) exchangeCodeForToken(ctx context.Context, code string) (*twitchTokenResponse, error) {
	if a.twitchClientID == "" || a.twitchClientSecret == "" || a.twitchRedirectURL == "" {
		return nil, fmt.Errorf("twitch client not configured")
	}

	data := url.Values{}
	data.Set("client_id", a.twitchClientID)
	data.Set("client_secret", a.twitchClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", a.twitchRedirectURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch token endpoint returned %s", resp.Status)
	}

	var tr twitchTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

// fetchTwitchUser récupère les informations d'un utilisateur Twitch
func (a *App) fetchTwitchUser(ctx context.Context, accessToken string) (*twitchUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch users endpoint returned %s", resp.Status)
	}

	var ur twitchUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		return nil, err
	}
	if len(ur.Data) == 0 {
		return nil, fmt.Errorf("no user data in response")
	}
	d := ur.Data[0]
	return &twitchUser{
		ID:              d.ID,
		Login:           d.Login,
		DisplayName:     d.DisplayName,
		ProfileImageURL: d.ProfileImageURL,
	}, nil
}

// fetchModeratedChannels récupère les chaînes modérées par un utilisateur
func (a *App) fetchModeratedChannels(ctx context.Context, accessToken, userID string) ([]struct {
	BroadcasterID    string
	BroadcasterLogin string
	BroadcasterName  string
}, error) {
	params := url.Values{}
	params.Set("user_id", userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/moderation/channels?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("forbidden: missing scope user:read:moderated_channels or moderator rights")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch moderation/channels returned %s", resp.Status)
	}

	var tr twitchModeratedChannelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}

	out := make([]struct {
		BroadcasterID    string
		BroadcasterLogin string
		BroadcasterName  string
	}, 0, len(tr.Data))

	for _, c := range tr.Data {
		out = append(out, struct {
			BroadcasterID    string
			BroadcasterLogin string
			BroadcasterName  string
		}{
			BroadcasterID:    c.BroadcasterID,
			BroadcasterLogin: c.BroadcasterLogin,
			BroadcasterName:  c.BroadcasterName,
		})
	}

	return out, nil
}

// revokeTwitchToken révoque un token d'accès Twitch
func (a *App) revokeTwitchToken(ctx context.Context, accessToken string) error {
	data := url.Values{}
	data.Set("client_id", a.twitchClientID)
	data.Set("token", accessToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/revoke", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twitch revoke returned %s", resp.Status)
	}
	return nil
}
