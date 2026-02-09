# Guide de Scalabilité et Performance

Ce document décrit les optimisations de performance et la stratégie de scalabilité horizontale implémentées dans le projet.

## Table des matières

- [Optimisations Verticales](#optimisations-verticales)
- [Scalabilité Horizontale](#scalabilité-horizontale)
- [Redis - Cache Distribué](#redis---cache-distribué)
- [Configuration](#configuration)
- [Monitoring](#monitoring)

## Optimisations Verticales

### Indexes Base de Données

Des indexes composés ont été ajoutés pour optimiser les requêtes fréquentes :

```sql
-- Sessions : lookup rapide par user + status
INDEX idx_sessions_user_status (user_id, status)

-- Captures : analyses temporelles
INDEX idx_captures_session_captured (session_id, captured_at)

-- Capture chatters : dédoublonnage et lookups
INDEX idx_capture_chatters_capture_user (capture_id, twitch_user_id)

-- Jobs : polling worker optimisé
INDEX idx_jobs_status_created (status, created_at)
```

### Connection Pooling

Configuration du pool de connexions MariaDB par service :

```yaml
# Gateway (nombreuses requêtes utilisateurs)
DB_MAX_OPEN_CONNS: 50
DB_MAX_IDLE_CONNS: 10

# Worker (charges ponctuelles)
DB_MAX_OPEN_CONNS: 20
DB_MAX_IDLE_CONNS: 5

# Analysis (requêtes lourdes)
DB_MAX_OPEN_CONNS: 30
DB_MAX_IDLE_CONNS: 10
```

### Paramètres MariaDB

```yaml
command:
  - --max-connections=200          # Support de tous les services
  - --innodb-buffer-pool-size=512M # Cache des données chaudes
```

## Scalabilité Horizontale

### Architecture Multi-Réplicas

```
┌─────────────────────────────┐
│     Load Balancer (Traefik)  │
└─────────┬───────────────────┘
          │
    ┌─────┼─────┐
    │     │     │
┌───v───v───v──┐
│ Gateway x2      │  Stateless
└─────────────────┘

┌─────────────────┐
│ Worker x3       │  Job Queue
└─────────────────┘

┌─────────────────┐
│ Analysis x2     │  Cache
└─────────────────┘

┌─────────────────┐
│ Redis           │  Cache/Sessions
└─────────────────┘

┌─────────────────┐
│ MariaDB         │  Shared State
└─────────────────┘
```

### Nombre de Réplicas

```yaml
gateway:
  deploy:
    replicas: 2  # 2x capacité HTTP

worker:
  deploy:
    replicas: 3  # 3x throughput jobs

analysis:
  deploy:
    replicas: 2  # 2x capacité analyses
```

### Ajuster les Réplicas

```bash
# Augmenter le nombre de workers
docker-compose up -d --scale worker=5

# Augmenter les gateways
docker-compose up -d --scale gateway=3

# Augmenter analysis
docker-compose up -d --scale analysis=3
```

## Redis - Cache Distribué

### Databases Redis (séparation logique)

```yaml
Gateway:  redis://redis:6379/0  # Sessions web
Twitch-API: redis://redis:6379/1  # Rate limiting distribué
Worker:   redis://redis:6379/2  # Métadonnées jobs
Analysis: redis://redis:6379/3  # Cache des résultats
```

### Utilisation du Cache

#### Dans Analysis (exemple)

```go
import "github.com/vignemail1/twitch-chatters-analyser/internal/redis"

// Initialisation
redisClient, err := redis.NewClient(os.Getenv("REDIS_URL"))
if err != nil {
    log.Fatal(err)
}
defer redisClient.Close()

// Cache des résultats d'analyse
func (a *App) getSessionSummary(sessionUUID string) (*Summary, error) {
    cacheKey := "summary:" + sessionUUID
    
    // 1. Vérifier le cache
    var summary Summary
    err := redisClient.GetJSON(ctx, cacheKey, &summary)
    if err == nil {
        return &summary, nil // Cache hit!
    }
    
    // 2. Calculer depuis la DB
    summary, err = a.computeSummaryFromDB(sessionUUID)
    if err != nil {
        return nil, err
    }
    
    // 3. Mettre en cache (5 minutes)
    ttl := 5 * time.Minute
    _ = redisClient.SetJSON(ctx, cacheKey, summary, ttl)
    
    return &summary, nil
}
```

#### Sessions Web (Gateway)

```go
// Stocker une session
sessionData := map[string]interface{}{
    "user_id": userID,
    "access_token": token,
}
redisClient.SetSession(ctx, sessionID, sessionData, 24*time.Hour)

// Récupérer une session
var session map[string]interface{}
err := redisClient.GetSession(ctx, sessionID, &session)
```

#### Rate Limiting Distribué (Twitch-API)

```go
// Vérifier rate limit (10 req/sec)
allowed, err := redisClient.CheckRateLimit(ctx, "twitch-api", 10, 1*time.Second)
if !allowed {
    return errors.New("rate limit exceeded")
}
```

### Configuration Redis

```yaml
redis:
  command: redis-server
    --maxmemory 256mb              # Limite mémoire
    --maxmemory-policy allkeys-lru # Éviction LRU
```

## Configuration

### Variables d'Environnement

```bash
# Base de données
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=10

# Redis
REDIS_URL=redis://redis:6379/0

# Cache
CACHE_TTL_SECONDS=300  # 5 minutes

# Worker
JOB_POLL_INTERVAL=2    # 2 secondes

# Rate limiting
RATE_LIMIT_REQUESTS_PER_SECOND=10
```

### Ajustements selon la Charge

#### Charge faible (< 100 users)
```yaml
gateway: replicas: 1
worker: replicas: 1
analysis: replicas: 1
```

#### Charge moyenne (100-1000 users)
```yaml
gateway: replicas: 2
worker: replicas: 3
analysis: replicas: 2
```

#### Charge élevée (> 1000 users)
```yaml
gateway: replicas: 4
worker: replicas: 5
analysis: replicas: 3
# + Envisager read replicas MariaDB
```

## Monitoring

### Métriques à Surveiller

```bash
# Queue de jobs
docker exec twitch-chatters-db mariadb -u twitch -p -e \
  "SELECT status, COUNT(*) FROM jobs GROUP BY status;"

# Connexions DB actives
docker exec twitch-chatters-db mariadb -u twitch -p -e \
  "SHOW PROCESSLIST;"

# Utilisation Redis
docker exec twitch-chatters-redis redis-cli INFO memory

# Nombre de réplicas actifs
docker-compose ps
```

### Logs de Performance

```bash
# Logs avec timestamps
docker-compose logs -f --tail=100 gateway
docker-compose logs -f --tail=100 worker

# Filtrer les requêtes lentes
docker-compose logs gateway | grep "in [0-9]\+ms" | awk '$NF > 1000'
```

### Signaux d'Alerte

⚠️ **Augmenter les workers** si :
- Queue de jobs > 100 pendant > 5 minutes
- Jobs `pending` > jobs `running` * 10

⚠️ **Augmenter les gateways** si :
- Latence HTTP > 500ms
- CPU gateway > 80%

⚠️ **Optimiser les requêtes DB** si :
- Connexions DB > 80% de max
- Requêtes > 100ms fréquentes

## Gains de Performance Attendus

### Avec Optimisations Verticales
- **Indexes** : 2-5x plus rapide sur requêtes filtrées
- **Connection pool** : Élimination des timeouts de connexion

### Avec Replicas
- **Gateway x2** : 2x capacité HTTP (req/sec)
- **Worker x3** : 3x throughput jobs
- **Analysis x2** : 2x capacité analyses

### Avec Redis Cache
- **Cache hit** : 100-1000x plus rapide (< 1ms vs 100-1000ms)
- **Réduction charge DB** : 50-80% selon taux de hit
- **Rate limiting distribué** : Cohérence entre toutes les instances

## Évolutions Futures

### Phase Suivante (si nécessaire)

1. **MariaDB Read Replicas**
   - Séparation lecture/écriture
   - Analysis et Gateway utilisent les replicas
   - 2-3x capacité lecture

2. **Galera Cluster**
   - 3 nœuds MariaDB actif-actif
   - Haute disponibilité
   - Élimination du SPOF

3. **Multi-Serveurs**
   - Docker Swarm ou Kubernetes
   - Séparation physique des services
   - Isolation des ressources

4. **Analytics Dédié**
   - ClickHouse pour analytics massifs
   - Data warehouse séparé
   - Exports périodiques depuis MariaDB
