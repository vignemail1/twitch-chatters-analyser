# Ressources et Coûts Infrastructure

Ce document détaille les besoins en ressources (CPU, RAM, Disk) pour l'ensemble du projet Twitch Chatters Analyser.

## Vue d'ensemble

### Configuration Minimale (Dev)
```
CPU  : 4 vCPU
RAM  : 8 GB
Disk : 50 GB SSD
```

### Configuration Recommandée (Production)
```
CPU  : 8 vCPU
RAM  : 16 GB
Disk : 100 GB SSD
```

### Configuration Haute Performance (> 1000 users)
```
CPU  : 16 vCPU
RAM  : 32 GB
Disk : 200 GB SSD
```

## Détail par Service

### Services Applicatifs

#### Gateway (x2 replicas)
```
Par instance:
  CPU  : 0.5 vCPU (pic: 1 vCPU)
  RAM  : 256 MB (pic: 512 MB)
  Disk : Négligeable (logs uniquement)

Total (2 instances):
  CPU  : 1 vCPU (pic: 2 vCPU)
  RAM  : 512 MB (pic: 1 GB)
```

**Facteurs d'impact** :
- Nombre de requêtes HTTP simultanées
- Sessions actives en mémoire
- Taille des réponses JSON

#### Worker (x3 replicas)
```
Par instance:
  CPU  : 0.3 vCPU (pic: 0.8 vCPU)
  RAM  : 256 MB (pic: 512 MB)
  Disk : Négligeable

Total (3 instances):
  CPU  : 0.9 vCPU (pic: 2.4 vCPU)
  RAM  : 768 MB (pic: 1.5 GB)
```

**Facteurs d'impact** :
- Nombre de jobs en queue
- Taille des payloads (nombre de chatters)
- Fréquence de polling

#### Analysis (x2 replicas)
```
Par instance:
  CPU  : 0.4 vCPU (pic: 1.2 vCPU)
  RAM  : 512 MB (pic: 1 GB)
  Disk : Négligeable

Total (2 instances):
  CPU  : 0.8 vCPU (pic: 2.4 vCPU)
  RAM  : 1 GB (pic: 2 GB)
```

**Facteurs d'impact** :
- Complexité des requêtes SQL d'agrégation
- Taille des datasets (nombre de captures)
- Cache Redis hit rate

#### Twitch-API (x1 instance)
```
CPU  : 0.2 vCPU (pic: 0.5 vCPU)
RAM  : 128 MB (pic: 256 MB)
Disk : Négligeable
```

**Facteurs d'impact** :
- Rate limiting (limité à 10 req/s)
- Nombre de workers actifs

### Infrastructure

#### MariaDB
```
CPU  : 2 vCPU (pic: 4 vCPU)
RAM  : 2 GB (base) + buffer pool 512 MB
Disk : 10 GB (base) + données utilisateurs

Total:
  CPU  : 2-4 vCPU
  RAM  : 2.5 GB (pic: 4 GB)
  Disk : Variable (voir calculs ci-dessous)
```

**Configuration actuelle** :
```yaml
max-connections: 200
innodb-buffer-pool-size: 512M
```

**Croissance disque** :
```
Par utilisateur actif (1000 captures):
  - sessions: ~200 bytes
  - captures: ~100 bytes * 1000 = 100 KB
  - capture_chatters: ~50 bytes * moyenne 50 chatters * 1000 = 2.5 MB
  - twitch_users: ~500 bytes * 200 uniques = 100 KB
  
  Total par user actif: ~2.7 MB

Pour 100 users actifs: ~270 MB
Pour 1000 users actifs: ~2.7 GB
Pour 10000 users actifs: ~27 GB
```

#### Redis
```
CPU  : 0.2 vCPU (pic: 0.5 vCPU)
RAM  : 256 MB (limité à 256 MB)
Disk : 100 MB (persistance)
```

**Configuration actuelle** :
```yaml
maxmemory: 256mb
maxmemory-policy: allkeys-lru
```

**Utilisation par DB** :
- DB 0 (Gateway sessions): ~50-100 MB
- DB 1 (Twitch-API rate limit): ~5-10 MB
- DB 2 (Worker metadata): ~10-20 MB
- DB 3 (Analysis cache): ~50-100 MB

#### Traefik
```
CPU  : 0.3 vCPU (pic: 0.8 vCPU)
RAM  : 128 MB (pic: 256 MB)
Disk : 100 MB (certificats Let's Encrypt)
```

### Monitoring (Optionnel)

#### Prometheus
```
CPU  : 0.5 vCPU (pic: 1 vCPU)
RAM  : 1 GB (pic: 2 GB)
Disk : 5 GB (rétention 30 jours)
```

**Croissance disque** :
```
~200 MB/jour pour 8 targets avec scrape_interval=15s
Rétention 30j = ~6 GB
```

#### Grafana
```
CPU  : 0.2 vCPU (pic: 0.5 vCPU)
RAM  : 256 MB (pic: 512 MB)
Disk : 500 MB (dashboards + config)
```

#### Loki
```
CPU  : 0.3 vCPU (pic: 0.8 vCPU)
RAM  : 512 MB (pic: 1 GB)
Disk : 3 GB (rétention 30 jours)
```

**Croissance disque** :
```
~100 MB/jour pour 10 containers actifs
Rétention 30j = ~3 GB
```

#### Promtail
```
CPU  : 0.1 vCPU
RAM  : 128 MB
Disk : Négligeable
```

#### Node Exporter + cAdvisor + Exporters
```
CPU  : 0.3 vCPU total
RAM  : 256 MB total
Disk : Négligeable
```

#### Alertmanager
```
CPU  : 0.1 vCPU
RAM  : 64 MB
Disk : 100 MB
```

## Récapitulatif Global

### Configuration Production (Sans Monitoring)

```
┌───────────────────────┬───────────┬───────────┬───────────────┐
│ Composant             │ CPU (avg) │ RAM (avg) │ Disk          │
├───────────────────────┼───────────┼───────────┼───────────────┤
│ Gateway (x2)          │ 1.0 vCPU  │ 512 MB    │ <10 MB        │
│ Worker (x3)           │ 0.9 vCPU  │ 768 MB    │ <10 MB        │
│ Analysis (x2)         │ 0.8 vCPU  │ 1 GB      │ <10 MB        │
│ Twitch-API (x1)       │ 0.2 vCPU  │ 128 MB    │ <10 MB        │
│ MariaDB               │ 2.0 vCPU  │ 2.5 GB    │ 10 GB + data  │
│ Redis                 │ 0.2 vCPU  │ 256 MB    │ 100 MB        │
│ Traefik               │ 0.3 vCPU  │ 128 MB    │ 100 MB        │
├───────────────────────┼───────────┼───────────┼───────────────┤
│ TOTAL                 │ 5.4 vCPU  │ 5.3 GB    │ ~12 GB        │
│ Recommandé (marge)    │ 8 vCPU    │ 8 GB      │ 50 GB SSD     │
└───────────────────────┴───────────┴───────────┴───────────────┘
```

### Configuration Production (Avec Monitoring)

```
┌───────────────────────┬───────────┬───────────┬───────────────┐
│ Composant             │ CPU (avg) │ RAM (avg) │ Disk          │
├───────────────────────┼───────────┼───────────┼───────────────┤
│ Services App          │ 5.4 vCPU  │ 5.3 GB    │ ~12 GB        │
│ Prometheus            │ 0.5 vCPU  │ 1 GB      │ 5 GB          │
│ Grafana               │ 0.2 vCPU  │ 256 MB    │ 500 MB        │
│ Loki                  │ 0.3 vCPU  │ 512 MB    │ 3 GB          │
│ Promtail              │ 0.1 vCPU  │ 128 MB    │ <10 MB        │
│ Exporters             │ 0.3 vCPU  │ 256 MB    │ <10 MB        │
│ Alertmanager          │ 0.1 vCPU  │ 64 MB     │ 100 MB        │
├───────────────────────┼───────────┼───────────┼───────────────┤
│ TOTAL                 │ 6.9 vCPU  │ 7.5 GB    │ ~21 GB        │
│ Recommandé (marge)    │ 12 vCPU   │ 12 GB     │ 80 GB SSD     │
└───────────────────────┴───────────┴───────────┴───────────────┘
```

## Projections selon la Charge

### Charge Faible (< 100 users actifs)
```
CPU  : 4-6 vCPU suffisants
RAM  : 6-8 GB
Disk : 30 GB (10 GB base + 1 GB données + logs)

Exemples VPS:
- OVH VPS Comfort: 4 vCPU, 8 GB RAM, 160 GB SSD ~ 12€/mois
- Hetzner CPX31: 4 vCPU, 8 GB RAM, 160 GB SSD ~ 12€/mois
```

### Charge Moyenne (100-1000 users)
```
CPU  : 8-12 vCPU
RAM  : 12-16 GB
Disk : 80 GB (10 GB base + 20 GB données + 50 GB monitoring)

Exemples VPS:
- OVH VPS Elite: 8 vCPU, 16 GB RAM, 320 GB SSD ~ 24€/mois
- Hetzner CPX51: 16 vCPU, 32 GB RAM, 360 GB SSD ~ 50€/mois
```

### Charge Élevée (> 1000 users)
```
CPU  : 16-32 vCPU
RAM  : 32-64 GB
Disk : 200 GB (10 GB base + 100 GB données + 50 GB monitoring + 40 GB marge)

Exemples Serveurs Dédiés:
- OVH Advance-1: 8c/16t, 32 GB RAM, 2x 512 GB NVMe ~ 60€/mois
- Hetzner AX52: 12c/24t, 64 GB RAM, 2x 1 TB NVMe ~ 60€/mois

+ Envisager read replicas MariaDB
```

## Optimisations Possibles

### Réduire Consommation CPU

```yaml
# Réduire nombre de replicas (dev)
gateway:
  deploy:
    replicas: 1  # Au lieu de 2
worker:
  deploy:
    replicas: 1  # Au lieu de 3
analysis:
  deploy:
    replicas: 1  # Au lieu de 2

# Économie: ~2 vCPU
```

```yaml
# Réduire fréquence monitoring
prometheus:
  scrape_interval: 60s  # Au lieu de 15s
  
# Économie: ~0.3 vCPU
```

### Réduire Consommation RAM

```yaml
# Réduire buffer pool MariaDB
mariadb:
  command:
    - --innodb-buffer-pool-size=256M  # Au lieu de 512M
    
# Économie: 256 MB
```

```yaml
# Réduire mémoire Prometheus
prometheus:
  command:
    - --storage.tsdb.retention.time=7d  # Au lieu de 30d
    
# Économie: ~500 MB
```

### Réduire Consommation Disk

```yaml
# Réduire rétention logs Loki
loki:
  limits_config:
    retention_period: 7d  # Au lieu de 30d
    
# Économie: ~2.5 GB
```

```yaml
# Nettoyer anciennes sessions
mariadb:
  # Script de nettoyage (cron)
  DELETE FROM sessions WHERE expires_at < NOW() - INTERVAL 7 DAY;
  DELETE FROM web_sessions WHERE expires_at < NOW() - INTERVAL 1 DAY;
```

## Coûts Estimatifs Mensuels

### Configuration Minimale (Dev/Test)
```
Serveur: 4 vCPU, 8 GB RAM, 80 GB SSD
Fournisseurs:
- OVH VPS Comfort:     ~12€/mois
- Hetzner CPX31:       ~12€/mois
- Scaleway DEV1-L:     ~12€/mois
- Contabo VPS L:       ~10€/mois

Total: ~12€/mois
```

### Configuration Production (100-1000 users)
```
Serveur: 8 vCPU, 16 GB RAM, 160 GB SSD
Fournisseurs:
- OVH VPS Elite:       ~24€/mois
- Hetzner CPX41:       ~24€/mois
- Scaleway PRO2-M:     ~30€/mois

Domaines (vignemail1.eu): Gratuit (si déjà possédé)
Certificats SSL:          Gratuit (Let's Encrypt)

Total: ~25€/mois
```

### Configuration Haute Performance (> 1000 users)
```
Serveur Dédié: 16c/32t, 64 GB RAM, 2x 1 TB NVMe
Fournisseurs:
- OVH Advance-2:       ~80€/mois
- Hetzner AX102:       ~80€/mois

Backup (500 GB):       ~10€/mois

Total: ~90€/mois
```

## Recommandations VPS

### Pour Développement
**Hetzner CPX21** : 3 vCPU, 4 GB RAM, 80 GB SSD - 7€/mois
- ✅ Bon rapport qualité/prix
- ✅ Réseau rapide (20 Gbps)
- ⚠️ Sans monitoring (trop juste)

### Pour Production (Recommandé)
**Hetzner CPX41** : 8 vCPU, 16 GB RAM, 240 GB SSD - 24€/mois
- ✅ Suffisant pour 100-1000 users
- ✅ Monitoring inclus
- ✅ Réseau rapide (20 Gbps)
- ✅ Snapshots gratuits

### Pour Haute Charge
**Hetzner AX52** : 12c/24t, 64 GB RAM, 2x 1 TB NVMe - 60€/mois
- ✅ Serveur dédié (performances stables)
- ✅ Large marge de scalabilité
- ✅ 1 Gbps garanti

## Monitoring des Ressources

### Commandes Utiles

```bash
# CPU usage par container
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"

# Disk usage
df -h
docker system df

# Memory usage détaillé
docker stats --no-stream

# Top processes
docker exec twitch-chatters-db top -bn1 | head -20

# MariaDB buffer pool
docker exec twitch-chatters-db mariadb -u root -p -e "SHOW VARIABLES LIKE 'innodb_buffer_pool_size';"

# Taille des tables
docker exec twitch-chatters-db mariadb -u twitch -p twitch_chatters -e "SELECT table_name, ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'Size (MB)' FROM information_schema.TABLES WHERE table_schema = 'twitch_chatters' ORDER BY (data_length + index_length) DESC;"
```

### Dashboard Grafana

Dans le dashboard **Twitch Chatters - Overview**, surveiller :
- CPU Usage (système)
- Memory Usage (système)
- Container CPU (par service)
- Container Memory (par service)
- Disk Usage
- MySQL Connections
- Redis Memory

## Conclusion

### Résumé Configurations

| Usage | CPU | RAM | Disk | Coût/mois |
|-------|-----|-----|------|----------|
| **Dev** | 4 vCPU | 8 GB | 50 GB | ~12€ |
| **Production (100-1000 users)** | 8 vCPU | 16 GB | 80 GB | ~25€ |
| **Haute Charge (> 1000 users)** | 16 vCPU | 32 GB | 200 GB | ~90€ |

### Scalabilité

Le projet est conçu pour scaler horizontalement :
- ✅ Ajouter des replicas (gateway, worker, analysis)
- ✅ Ajouter des read replicas MariaDB
- ✅ Distribuer sur plusieurs serveurs (Docker Swarm/Kubernetes)

Avec la configuration actuelle, tu peux facilement supporter **500-1000 utilisateurs actifs** sur un serveur 8 vCPU / 16 GB RAM.
