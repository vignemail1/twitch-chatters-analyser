package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type App struct {
	addr               string
	twitchClientID     string
	twitchClientSecret string

	// Rate limiter global pour respecter les limites Twitch (800 req/min)
	limiter *rate.Limiter

	// Cache simple pour les réponses (optionnel)
	cacheMu sync.RWMutex
	cache   map[string]cacheEntry
}

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

func main() {
	port := getenv("APP_PORT", "8081")
	twitchClientID := getenv("TWITCH_CLIENT_ID", "")
	twitchClientSecret := getenv("TWITCH_CLIENT_SECRET", "")

	if twitchClientID == "" || twitchClientSecret == "" {
		log.Fatal("TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET are required")
	}

	// Twitch limite à 800 req/min pour les app tokens
	// On prend une marge : 600 req/min = 10 req/sec
	ratePerSec, _ := strconv.Atoi(getenv("RATE_LIMIT_REQUESTS_PER_SECOND", "10"))
	burst := ratePerSec * 2 // Burst de 2 secondes

	app := &App{
		addr:               ":" + port,
		twitchClientID:     twitchClientID,
		twitchClientSecret: twitchClientSecret,
		limiter:            rate.NewLimiter(rate.Limit(ratePerSec), burst),
		cache:              make(map[string]cacheEntry),
	}

	// Nettoyage du cache toutes les 5 minutes
	go app.cleanCachePeriodically(5 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/chatters", app.handleChatters)
	mux.HandleFunc("/users", app.handleUsers)
	mux.HandleFunc("/moderated-channels", app.handleModeratedChannels)

	handler := loggingMiddleware(mux)

	httpServer := &http.Server{
		Addr:              app.addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("twitch-api listening on %s (rate: %d req/s, burst: %d)", app.addr, ratePerSec, burst)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleChatters proxy vers GET https://api.twitch.tv/helix/chat/chatters
func (a *App) handleChatters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	broadcasterID := r.URL.Query().Get("broadcaster_id")
	moderatorID := r.URL.Query().Get("moderator_id")
	accessToken := r.Header.Get("Authorization") // Format: "Bearer {token}"

	if broadcasterID == "" || moderatorID == "" {
		http.Error(w, "missing broadcaster_id or moderator_id", http.StatusBadRequest)
		return
	}

	if accessToken == "" {
		http.Error(w, "missing Authorization header", http.StatusUnauthorized)
		return
	}

	// Attendre le rate limiter
	if err := a.limiter.Wait(r.Context()); err != nil {
		http.Error(w, "rate limit context error", http.StatusTooManyRequests)
		return
	}

	// Construire l'URL Twitch
	params := url.Values{}
	params.Set("broadcaster_id", broadcasterID)
	params.Set("moderator_id", moderatorID)
	if first := r.URL.Query().Get("first"); first != "" {
		params.Set("first", first)
	}
	if after := r.URL.Query().Get("after"); after != "" {
		params.Set("after", after)
	}

	twitchURL := "https://api.twitch.tv/helix/chat/chatters?" + params.Encode()

	// Proxy la requête
	body, statusCode, err := a.proxyTwitchRequest(r.Context(), twitchURL, accessToken)
	if err != nil {
		log.Printf("proxy chatters error: %v", err)
		http.Error(w, "failed to fetch chatters from Twitch", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

// handleUsers proxy vers GET https://api.twitch.tv/helix/users
func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accessToken := r.Header.Get("Authorization")
	if accessToken == "" {
		http.Error(w, "missing Authorization header", http.StatusUnauthorized)
		return
	}

	// Construire la requête avec tous les paramètres id= ou login=
	params := url.Values{}
	for key, values := range r.URL.Query() {
		if key == "id" || key == "login" {
			for _, v := range values {
				params.Add(key, v)
			}
		}
	}

	if len(params) == 0 {
		http.Error(w, "missing id or login parameters", http.StatusBadRequest)
		return
	}

	// Vérifier le cache
	cacheKey := "users:" + params.Encode()
	if cached := a.getCache(cacheKey); cached != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}

	// Attendre le rate limiter
	if err := a.limiter.Wait(r.Context()); err != nil {
		http.Error(w, "rate limit context error", http.StatusTooManyRequests)
		return
	}

	twitchURL := "https://api.twitch.tv/helix/users?" + params.Encode()

	body, statusCode, err := a.proxyTwitchRequest(r.Context(), twitchURL, accessToken)
	if err != nil {
		log.Printf("proxy users error: %v", err)
		http.Error(w, "failed to fetch users from Twitch", http.StatusBadGateway)
		return
	}

	if statusCode == http.StatusOK {
		// Cache les infos utilisateurs pour 5 minutes
		a.setCache(cacheKey, body, 5*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

// handleModeratedChannels proxy vers GET https://api.twitch.tv/helix/moderation/channels
func (a *App) handleModeratedChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	accessToken := r.Header.Get("Authorization")

	if userID == "" {
		http.Error(w, "missing user_id", http.StatusBadRequest)
		return
	}

	if accessToken == "" {
		http.Error(w, "missing Authorization header", http.StatusUnauthorized)
		return
	}

	// Vérifier le cache (1 minute pour les channels modérées)
	cacheKey := "moderated:" + userID
	if cached := a.getCache(cacheKey); cached != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}

	// Attendre le rate limiter
	if err := a.limiter.Wait(r.Context()); err != nil {
		http.Error(w, "rate limit context error", http.StatusTooManyRequests)
		return
	}

	params := url.Values{}
	params.Set("user_id", userID)
	twitchURL := "https://api.twitch.tv/helix/moderation/channels?" + params.Encode()

	body, statusCode, err := a.proxyTwitchRequest(r.Context(), twitchURL, accessToken)
	if err != nil {
		log.Printf("proxy moderated-channels error: %v", err)
		http.Error(w, "failed to fetch moderated channels from Twitch", http.StatusBadGateway)
		return
	}

	if statusCode == http.StatusOK {
		// Cache pour 1 minute
		a.setCache(cacheKey, body, 1*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

func (a *App) proxyTwitchRequest(ctx context.Context, twitchURL, accessToken string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, twitchURL, nil)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("twitch API error: %s - %s", resp.Status, string(body))
	}

	return body, resp.StatusCode, nil
}

// Cache management
func (a *App) getCache(key string) []byte {
	a.cacheMu.RLock()
	defer a.cacheMu.RUnlock()

	entry, ok := a.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.data
}

func (a *App) setCache(key string, data []byte, ttl time.Duration) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	a.cache[key] = cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
}

func (a *App) cleanCachePeriodically(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		a.cacheMu.Lock()
		now := time.Now()
		for key, entry := range a.cache {
			if now.After(entry.expiresAt) {
				delete(a.cache, key)
			}
		}
		a.cacheMu.Unlock()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s from %s in %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
