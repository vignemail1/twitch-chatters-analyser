package main

import (
	"html/template"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"database/sql"
)

func main() {
	port := getenv("APP_PORT", "8080")

	// DSN MySQL
	dbUser := getenv("DB_USER", "twitch")
	dbPass := getenv("DB_PASSWORD", "twitchpass")
	dbHost := getenv("DB_HOST", "db")
	dbPort := getenv("DB_PORT", "3306")
	dbName := getenv("DB_NAME", "twitch_chatters")

	dsn := dbUser + ":" + dbPass + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

	twitchClientID := getenv("TWITCH_CLIENT_ID", "")
	twitchClientSecret := getenv("TWITCH_CLIENT_SECRET", "")
	twitchRedirectURL := getenv("TWITCH_REDIRECT_URL", "")

	if twitchClientID == "" || twitchClientSecret == "" || twitchRedirectURL == "" {
		log.Println("warning: TWITCH_CLIENT_ID/SECRET/REDIRECT_URL not fully set; auth will not work correctly")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("cannot open DB: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("cannot ping DB: %v", err)
	}

	// Ajouter les fonctions personnalisées pour les templates
	funcMap := template.FuncMap{
		"add": func(a, b int64) int64 { return a + b },
		"mul": func(a, b int64) int64 { return a * b },
		"div": func(a, b int64) int64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"contains": func(slice []string, item string) bool {
			for _, s := range slice {
				if s == item {
					return true
				}
			}
			return false
		},
	}

	tmpls, err := template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		log.Fatalf("cannot load templates: %v", err)
	}

	analysisBaseURL := getenv("ANALYSIS_BASE_URL", "http://analysis:8083")

	app := &App{
		addr:               ":" + port,
		db:                 db,
		templates:          tmpls,
		twitchClientID:     twitchClientID,
		twitchClientSecret: twitchClientSecret,
		twitchRedirectURL:  twitchRedirectURL,
		analysisBaseURL:    analysisBaseURL,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/analysis", app.handleAnalysis)
	mux.HandleFunc("/analysis/export", app.handleAnalysisExport)
	mux.HandleFunc("/analysis/saved/", app.handleSavedAnalysis)
	mux.HandleFunc("/sessions", app.handleSessions)
	mux.HandleFunc("/sessions/capture", app.handleCreateCapture)
	mux.HandleFunc("/sessions/save", app.handleSaveSession)
	mux.HandleFunc("/sessions/delete", app.handleDeleteSession)
	mux.HandleFunc("/sessions/purge", app.handlePurgeSession)
	mux.HandleFunc("/sessions/export/", app.handleSessionExport)
	mux.HandleFunc("/channels", app.handleChannels)
	mux.HandleFunc("/accounts/", app.handleAccountHistory)
	mux.HandleFunc("/auth/login", app.handleAuthLogin)
	mux.HandleFunc("/auth/callback", app.handleAuthCallback)
	mux.HandleFunc("/auth/logout", app.handleLogout)
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/", app.handleIndex)

	fileServer := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	handler := loggingMiddleware(app.loadCurrentUser(mux))

	httpServer := &http.Server{
		Addr:              app.addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("gateway listening on %s", app.addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
