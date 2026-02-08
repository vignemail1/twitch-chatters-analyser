# Twitch API Proxy Service

Service proxy centralisé pour toutes les requêtes vers l'API Twitch Helix. Gère le rate limiting, le cache et la gestion d'erreurs.

## 🎯 Objectif

Centraliser tous les appels à l'API Twitch pour :
- **Rate limiting global** : Respecter les limites de 800 req/min de Twitch
- **Cache intelligent** : Éviter les appels redondants
- **Retry logic** : Gestion automatique des erreurs temporaires
- **Monitoring** : Logs centralisés des appels API

## 🔌 Routes disponibles

### `GET /healthz`
Health check du service.

**Réponse** : `200 OK` avec body `ok`

---

### `GET /chatters`
Proxy vers `https://api.twitch.tv/helix/chat/chatters`

**Paramètres query** :
- `broadcaster_id` (required) : ID du broadcaster
- `moderator_id` (required) : ID du modérateur
- `first` (optional) : Nombre de résultats par page (max 1000)
- `after` (optional) : Cursor de pagination

**Headers** :
- `Authorization: Bearer {token}` (required)

**Réponse** : JSON conforme à l'API Twitch Helix

**Exemple** :
```bash
curl -H "Authorization: Bearer abc123" \
  "http://twitch-api:8081/chatters?broadcaster_id=123&moderator_id=456"
```

---

### `GET /users`
Proxy vers `https://api.twitch.tv/helix/users`

**Paramètres query** :
- `id` (repeatable) : User ID(s) Twitch (max 100)
- `login` (repeatable) : Username(s) Twitch (max 100)

**Headers** :
- `Authorization: Bearer {token}` (required)

**Cache** : 5 minutes

**Réponse** : JSON conforme à l'API Twitch Helix + header `X-Cache: HIT|MISS`

**Exemple** :
```bash
curl -H "Authorization: Bearer abc123" \
  "http://twitch-api:8081/users?id=123&id=456&id=789"
```

---

### `GET /moderated-channels`
Proxy vers `https://api.twitch.tv/helix/moderation/channels`

**Paramètres query** :
- `user_id` (required) : ID de l'utilisateur

**Headers** :
- `Authorization: Bearer {token}` (required)

**Cache** : 1 minute

**Réponse** : JSON conforme à l'API Twitch Helix + header `X-Cache: HIT|MISS`

**Exemple** :
```bash
curl -H "Authorization: Bearer abc123" \
  "http://twitch-api:8081/moderated-channels?user_id=123"
```

---

## ⚙️ Configuration

### Variables d'environnement

| Variable | Description | Défaut |
|----------|-------------|--------|
| `APP_PORT` | Port d'écoute du service | `8081` |
| `TWITCH_CLIENT_ID` | Client ID de l'app Twitch | *required* |
| `TWITCH_CLIENT_SECRET` | Client Secret de l'app Twitch | *required* |
| `RATE_LIMIT_REQUESTS_PER_SECOND` | Limite de requêtes par seconde | `10` (600/min) |

### Rate Limiting

- **Limite Twitch** : 800 req/min pour app tokens
- **Configuration par défaut** : 10 req/sec = 600 req/min (marge de sécurité)
- **Burst** : 2x la limite par seconde (20 requêtes)
- **Comportement** : Bloque la requête jusqu'à disponibilité de quota

### Cache

Cache en mémoire simple avec TTL :

| Endpoint | TTL | Justification |
|----------|-----|---------------|
| `/users` | 5 min | Infos utilisateurs changent rarement |
| `/moderated-channels` | 1 min | Peut changer fréquemment |
| `/chatters` | Pas de cache | Données temps réel |

Le cache est nettoyé automatiquement toutes les 5 minutes.

---

## 📦 Déploiement

### Docker Compose

```yaml
twitch-api:
  build:
    context: .
    dockerfile: ./cmd/twitch-api/Dockerfile
  environment:
    TWITCH_CLIENT_ID: ${TWITCH_CLIENT_ID}
    TWITCH_CLIENT_SECRET: ${TWITCH_CLIENT_SECRET}
    RATE_LIMIT_REQUESTS_PER_SECOND: "10"
  networks:
    - backend
```

### Build manuel

```bash
go build -o twitch-api ./cmd/twitch-api
./twitch-api
```

---

## 📊 Monitoring

### Logs

Le service log automatiquement :
- Chaque requête traitée (method, path, duration)
- Les erreurs API Twitch
- Les changements de cache (HIT/MISS)

**Exemple de log** :
```
2026/02/08 23:00:00 twitch-api listening on :8081 (rate: 10 req/s, burst: 20)
2026/02/08 23:00:05 GET /users from 172.18.0.5:54321 in 245ms
2026/02/08 23:00:10 twitch API error: 429 Too Many Requests - {"error":"..."}
```

### Health Check

```bash
curl http://twitch-api:8081/healthz
# Réponse: ok
```

---

## 🔧 Améliorations futures

- [ ] **Redis cache** : Remplacer le cache mémoire par Redis pour partager entre instances
- [ ] **Prometheus metrics** : Exporter métriques (nb requêtes, latence, cache hit rate)
- [ ] **Circuit breaker** : Détecter et gérer les pannes API Twitch
- [ ] **App token auto-refresh** : Générer et renouveler automatiquement un app token
- [ ] **Retry automatique** : Retry intelligent avec backoff exponentiel
- [ ] **Compression** : Gzip des réponses pour réduire la bande passante

---

## 🐛 Dépannage

### Erreur: `rate limit context error`

**Cause** : Trop de requêtes simultanées dépassent la limite configurée.

**Solution** :
1. Augmenter `RATE_LIMIT_REQUESTS_PER_SECOND`
2. Optimiser les appels côté client (batch, cache)

### Erreur: `twitch API error: 401 Unauthorized`

**Cause** : Token invalide ou expiré.

**Solution** :
1. Vérifier que le token passé dans `Authorization` est valide
2. Renouveler le token côté client (gateway/worker)

### Cache ne fonctionne pas

**Cause** : Paramètres de requête différents entre appels.

**Vérification** : Le cache est basé sur l'URL complète (endpoint + query params). Assurez-vous que les paramètres sont identiques.

---

## 📚 Références

- [Twitch API Documentation](https://dev.twitch.tv/docs/api/)
- [Rate Limits Twitch](https://dev.twitch.tv/docs/api/guide#rate-limits)
- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate)
