# Guide de Scalabilité et Performance

Ce document décrit les optimisations de performance et la stratégie de scalabilité du projet.

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

### Services Scalables

Les services suivants **sont prêts pour le scaling** (pas de `container_name`) :

- ✅ **Gateway** : API HTTP, sessions dans Redis (stateless)
- ✅ **Worker** : Consomme jobs depuis MariaDB/Redis (queue distribuée)
- ✅ **Analysis** : Cache dans Redis (stateless)
- ✅ **Twitch-API** : Rate limiting dans Redis (partagé)

### Services Non-Scalables

Ces services restent en **instance unique** :

- 🔒 **MariaDB** : Base de données unique (voir section HA pour read replicas)
- 🔒 **Redis** : Cache unique (suffisant pour la plupart des cas)
- 🔒 **Traefik** : Reverse proxy unique

### Architecture Par Défaut (1 Instance)

```
┌─────────────────────────────┐
│     Load Balancer (Traefik)  │
└─────────┬───────────────────┘
          │
          v
┌─────────────────┐
│ Gateway (x1)    │  Stateless
└─────────────────┘

┌─────────────────┐
│ Worker (x1)     │  Job Queue
└─────────────────┘

┌─────────────────┐
│ Analysis (x1)   │  Cache
└─────────────────┘

┌─────────────────┐
│ Twitch-API (x1) │  Rate Limiting
└─────────────────┘

┌─────────────────┐
│ Redis           │  Cache/Sessions (partagé)
└─────────────────┘

┌─────────────────┐
│ MariaDB         │  Shared State (partagé)
└─────────────────┘
```

**Capacité** : 100-500 utilisateurs simultanés

## Scaling avec Docker Compose

### Méthode 1 : Flag `--scale` (Recommandée pour Dev/Test)

```bash
# Démarrer avec scaling
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

# Vérifier les instances
docker-compose ps
# NAME                              STATUS
# twitch-chatters-analyser-gateway-1    running
# twitch-chatters-analyser-gateway-2    running
# twitch-chatters-analyser-worker-1     running
# twitch-chatters-analyser-worker-2     running
# twitch-chatters-analyser-worker-3     running
# twitch-chatters-analyser-analysis-1   running
# twitch-chatters-analyser-analysis-2   running

# Arrêter
docker-compose down
```

**Avantages** :
- ✅ Simple et rapide
- ✅ Pas de configuration supplémentaire
- ✅ Idéal pour tests de charge

**Inconvénients** :
- ❌ Il faut spécifier `--scale` à chaque `up`
- ❌ Pas de scaling dynamique en cours d'exécution

### Méthode 2 : Docker Swarm (Recommandée pour Production)

```bash
# 1. Initialiser Swarm
docker swarm init

# 2. Déployer la stack
docker stack deploy -c docker-compose.yml twitch-chatters

# 3. Vérifier les services
docker service ls
# ID             NAME                        MODE         REPLICAS
# abc123         twitch-chatters_gateway     replicated   1/1
# def456         twitch-chatters_worker      replicated   1/1

# 4. Scaler dynamiquement (sans redémarrage)
docker service scale twitch-chatters_gateway=3
docker service scale twitch-chatters_worker=5
docker service scale twitch-chatters_analysis=2

# 5. Surveiller
docker service ps twitch-chatters_gateway
# ID             NAME                          NODE      DESIRED STATE   CURRENT STATE
# xyz789         twitch-chatters_gateway.1     manager   Running         Running
# uvw012         twitch-chatters_gateway.2     manager   Running         Running
# rst345         twitch-chatters_gateway.3     manager   Running         Running

# 6. Réduire le nombre de réplicas
docker service scale twitch-chatters_worker=2

# 7. Supprimer la stack
docker stack rm twitch-chatters
```

**Avantages** :
- ✅ Scaling dynamique sans redémarrage
- ✅ Auto-restart des containers
- ✅ Health checks avancés
- ✅ Rolling updates
- ✅ Production-ready

**Inconvénients** :
- ❌ Nécessite Docker Swarm
- ❌ Syntaxe légèrement différente

### Architecture Multi-Réplicas

```
┌─────────────────────────────┐
│     Load Balancer (Traefik)  │
└─────────┬───────────────────┘
          │
    ┌─────┼─────┐
    │     │     │
┌───v─────v─────v──┐
│ Gateway (x3)    │  Stateless
└─────────────────┘

┌─────────────────┐
│ Worker (x5)     │  Job Queue
└─────────────────┘

┌─────────────────┐
│ Analysis (x2)   │  Cache
└─────────────────┘

┌─────────────────┐
│ Twitch-API (x1) │  Rate Limiting (pas besoin de scale)
└─────────────────┘

┌─────────────────┐
│ Redis           │  Cache/Sessions (partagé)
└─────────────────┘

┌─────────────────┐
│ MariaDB         │  Shared State (partagé)
└─────────────────┘
```

**Capacité** : 500-1000 utilisateurs simultanés

## Redis - Cache Distribué

### Databases Redis (séparation logique)

```yaml
Gateway:    redis://redis:6379/0  # Sessions web
Twitch-API: redis://redis:6379/1  # Rate limiting distribué
Worker:     redis://redis:6379/2  # Métadonnées jobs
Analysis:   redis://redis:6379/3  # Cache des résultats
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

```bash
# Configuration par défaut (1 replica par service)
docker-compose up -d
```

**Capacité** : 100-500 utilisateurs simultanés

#### Charge moyenne (100-1000 users)

```bash
# Méthode 1: Compose --scale
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

# Méthode 2: Swarm
docker swarm init
docker stack deploy -c docker-compose.yml twitch-chatters
docker service scale twitch-chatters_gateway=2
docker service scale twitch-chatters_worker=3
docker service scale twitch-chatters_analysis=2
```

**Capacité** : 500-1000 utilisateurs simultanés

#### Charge élevée (> 1000 users)

```bash
# Swarm recommandé
docker service scale twitch-chatters_gateway=4
docker service scale twitch-chatters_worker=5
docker service scale twitch-chatters_analysis=3
# + Envisager read replicas MariaDB
```

**Capacité** : > 1000 utilisateurs simultanés

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

# Services actifs
docker-compose ps
# Ou en mode Swarm:
docker service ls
```

### Logs de Performance

```bash
# Logs avec timestamps (Compose)
docker-compose logs -f --tail=100 gateway
docker-compose logs -f --tail=100 worker

# Logs avec timestamps (Swarm)
docker service logs -f twitch-chatters_gateway
docker service logs -f twitch-chatters_worker

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
- **Redis cache** : 100-1000x plus rapide (< 1ms vs 100-1000ms)

### Avec Replicas

- **Gateway x2** : 2x capacité HTTP (req/sec)
- **Worker x3** : 3x throughput jobs
- **Analysis x2** : 2x capacité analyses

### Avec Redis Cache

- **Cache hit** : 100-1000x plus rapide (< 1ms vs 100-1000ms)
- **Réduction charge DB** : 50-80% selon taux de hit
- **Rate limiting distribué** : Cohérence entre toutes les instances

## Évolutions Futures

### Étape 1 : Auto-Scaling (Optionnel)

Pour scaling automatique basé sur la charge :

1. **Kubernetes** : HorizontalPodAutoscaler
2. **Docker Swarm + Prometheus** : Scripts custom
3. **Cloud** : AWS ECS, GCP Cloud Run

### Étape 2 : Haute Disponibilité

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

## Exemples de Configuration

### Configuration 1 : Dev/Test (Par Défaut)

```bash
docker-compose up -d
```

**Ressources** :
- CPU : 4 vCPU
- RAM : 4 GB
- Capacité : 100-500 users

### Configuration 2 : Production Moyenne

```bash
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2
```

**Ressources** :
- CPU : 8 vCPU
- RAM : 12 GB
- Capacité : 500-1000 users

### Configuration 3 : Production Haute Charge

```bash
docker swarm init
docker stack deploy -c docker-compose.yml twitch-chatters
docker service scale twitch-chatters_gateway=4
docker service scale twitch-chatters_worker=5
docker service scale twitch-chatters_analysis=3
```

**Ressources** :
- CPU : 16 vCPU
- RAM : 24 GB
- Capacité : > 1000 users
