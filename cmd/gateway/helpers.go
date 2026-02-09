package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
)

// getenv récupère une variable d'environnement avec une valeur par défaut
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// randomHex génère une chaîne hexadécimale aléatoire de n octets
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
