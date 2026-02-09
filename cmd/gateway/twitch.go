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

// fetchBroadcasterInfo récupère les informations du broadcaster (l'utilisateur connecté)
func (a *App) fetchBroadcasterInfo(ctx context.Context, accessToken, userID string) (*twitchUser, error) {
	params := url.Values{}
	params.Set("id", userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users?"+params.Encode(), nil)
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

// fetchModeratedChannels récupère les chaînes modérées par un utilisateur + sa propre chaîne
func (a *App) fetchModeratedChannels(ctx context.Context, accessToken, userID string) ([]struct {
	BroadcasterID    string
	BroadcasterLogin string
	BroadcasterName  string
}, error) {
	// Récupérer les informations du broadcaster (pour sa propre chaîne)
	broadcaster, err := a.fetchBroadcasterInfo(ctx, accessToken, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch broadcaster info: %w", err)
	}

	// Initialiser la liste avec la propre chaîne du broadcaster
	out := []struct {
		BroadcasterID    string
		BroadcasterLogin string
		BroadcasterName  string
	}{
		{
			BroadcasterID:    broadcaster.ID,
			BroadcasterLogin: broadcaster.Login,
			BroadcasterName:  broadcaster.DisplayName,
		},
	}

	// Récupérer les chaînes où l'utilisateur est modérateur
	params := url.Values{}
	params.Set("user_id", userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/moderation/channels?"+params.Encode(), nil)
	if err != nil {
		return out, nil // Retourner au moins la propre chaîne du broadcaster
	}
	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, nil // Retourner au moins la propre chaîne du broadcaster
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		// Si le scope n'est pas disponible, retourner au moins la propre chaîne
		return out, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, nil // Retourner au moins la propre chaîne du broadcaster
	}

	var tr twitchModeratedChannelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return out, nil // Retourner au moins la propre chaîne du broadcaster
	}

	// Ajouter les chaînes modérées (en évitant les doublons si par hasard l'API retournait la propre chaîne)
	for _, c := range tr.Data {
		// Éviter d'ajouter deux fois la même chaîne
		if c.BroadcasterID == broadcaster.ID {
			continue
		}
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
