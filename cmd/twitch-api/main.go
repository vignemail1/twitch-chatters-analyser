package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type App struct {
	addr               string
	twitchClientID     string
	twitchClientSecret string
	limiter            *rate.Limiter
	cache              *Cache
}

type Cache struct {
	mu    sync.RWMutex
	items map[string]*CacheItem
}

type CacheItem struct {
	Value      []byte
	Expiration time.Time
}

func NewCache() *Cache {
	return &Cache{
		items: make(map[string]*CacheItem),
	}
}

func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	if time.Now().After(item.Expiration) {
		return nil, false
	}

	return item.Value, true
}

func (c *Cache) Set(key string, value []byte, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &CacheItem{
		Value:      value,
		Expiration: time.Now().Add(duration),
	}
}

// Nettoyage périodique du cache
func (c *Cache) StartCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			c.mu.Lock()
			now := time.Now()
			for key, item := range c.items {
				if now.After(item.Expiration) {
					delete(c.items, key)
				}
			}
			c.mu.Unlock()
		}
	}()
}

func main() {
	port := getenv("APP_PORT", "8081")
	twitchClientID := getenv("TWITCH_CLIENT_ID", "")
	twitchClientSecret := getenv("TWITCH_CLIENT_SECRET", "")

	if twitchClientID == "" || twitchClientSecret == "" {
		log.Fatal("TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET must be set")
	}

	// Rate limiting: 800 req/min = ~13 req/s avec burst de 20
	rateLimit := getenvInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 600)
	limiter := rate.NewLimiter(rate.Limit(float64(rateLimit)/60.0), 20)

	cache := NewCache()
	cache.StartCleanup(5 * time.Minute)

	app := &App{
		addr:               ":" + port,
		twitchClientID:     twitchClientID,
		twitchClientSecret: twitchClientSecret,
		limiter:            limiter,
		cache:              cache,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/chatters", app.handleChatters)
	mux.HandleFunc("/users", app.handleUsers)
	mux.HandleFunc("/moderated-channels", app.handleModeratedChannels)
	mux.HandleFunc("/healthz", app.handleHealth)

	handler := loggingMiddleware(mux)

	httpServer := &http.Server{
		Addr:              app.addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("twitch-api listening on %s (rate limit: %d req/min)", app.addr, rateLimit)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// GET /chatters?broadcaster_id={id}&moderator_id={id}&access_token={token}
func (a *App) handleChatters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	broadcasterID := r.URL.Query().Get("broadcaster_id")
	moderatorID := r.URL.Query().Get("moderator_id")
	accessToken := r.URL.Query().Get("access_token")

	if broadcasterID == "" || moderatorID == "" || accessToken == "" {
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	// Pas de cache pour les chatters (données temps réel)
	if err := a.limiter.Wait(r.Context()); err != nil {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	params := url.Values{}
	params.Set("broadcaster_id", broadcasterID)
	params.Set("moderator_id", moderatorID)
	params.Set("first", "1000")

	twitchURL := "https://api.twitch.tv/helix/chat/chatters?" + params.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, twitchURL, nil)
	if err != nil {
		log.Printf("create request error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("twitch api error: %v", err)
		http.Error(w, "twitch api error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("read response error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// GET /users?id={id1}&id={id2}&access_token={token}
func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accessToken := r.URL.Query().Get("access_token")
	if accessToken == "" {
		http.Error(w, "missing access_token", http.StatusBadRequest)
		return
	}

	// Récupérer tous les IDs
	ids := r.URL.Query()["id"]
	if len(ids) == 0 {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	// Cache key basé sur les IDs triés
	cacheKey := "users:" + strings.Join(ids, ",")
	if cached, found := a.cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}

	if err := a.limiter.Wait(r.Context()); err != nil {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Construire l'URL Twitch
	params := url.Values{}
	for _, id := range ids {
		params.Add("id", id)
	}

	twitchURL := "https://api.twitch.tv/helix/users?" + params.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, twitchURL, nil)
	if err != nil {
		log.Printf("create request error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("twitch api error: %v", err)
		http.Error(w, "twitch api error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("read response error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Cache pendant 5 minutes (les infos users changent rarement)
	if resp.StatusCode == http.StatusOK {
		a.cache.Set(cacheKey, body, 5*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// GET /moderated-channels?user_id={id}&access_token={token}
func (a *App) handleModeratedChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	accessToken := r.URL.Query().Get("access_token")

	if userID == "" || accessToken == "" {
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	// Cache key
	cacheKey := "moderated_channels:" + userID
	if cached, found := a.cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}

	if err := a.limiter.Wait(r.Context()); err != nil {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	params := url.Values{}
	params.Set("user_id", userID)

	twitchURL := "https://api.twitch.tv/helix/moderation/channels?" + params.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, twitchURL, nil)
	if err != nil {
		log.Printf("create request error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("twitch api error: %v", err)
		http.Error(w, "twitch api error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("read response error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Cache pendant 2 minutes
	if resp.StatusCode == http.StatusOK {
		a.cache.Set(cacheKey, body, 2*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
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

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
