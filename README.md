# Twitch Chatters Analyser

Application d'analyse des spectateurs Twitch pour modérateurs et streamers.

## 📊 Vue d'ensemble

Twitch Chatters Analyser permet aux modérateurs de chaînes Twitch de :
- 📋 Capturer périodiquement la liste des chatters actifs
- 📈 Analyser les statistiques de participation
- 🔍 Identifier les nouveaux spectateurs
- 📊 Visualiser l'évolution dans le temps
- 💾 Exporter les données pour analyses avancées

## 🏛️ Architecture

### Services

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                                                                                  │
│                                   INFRASTRUCTURE                                               │
│                                                                                                  │
│  ┌───────────────┐   ┌───────────────┐   ┌───────────────┐   ┌───────────────┐  │
│  │   Gateway     │   │  Twitch-API  │   │   Worker      │   │  Analysis    │  │
│  │  HTTP API    │   │  Rate Limit  │   │  Job Queue   │   │  Stats       │  │
│  └───────┬────────┘   └───────┬────────┘   └───────┬────────┘   └───────┬────────┘  │
│          │                   │               │               │             │
│          v                   v               v               v             │
│  ┌───────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                                                                            │  │
│  │                 ┌─────────────────────────────────────────┐                     │  │
│  │                 │         Backend Network              │                     │  │
│  │                 └─────────────────┬────────────────────────┘                     │  │
│  │                                  │                                                 │  │
│  │                     ┌────────────┼────────────┐                                   │  │
│  │                     │             │            │                                   │  │
│  │                ┌────v───────────── ┌────v───────────┐                              │  │
│  │                │   MariaDB       │ │    Redis      │                              │  │
│  │                │   Database      │ │    Cache      │                              │  │
│  │                └─────────────────┘ └────────────────┘                              │  │
│  │                                                                                            │  │
│  └───────────────────────────────────────────────────────────────────────────────────────┘  │
│                                         │                                                    │
│                                         v                                                    │
│                                  ┌─────────────────┐                                         │
│                                  │    Traefik     │                                         │
│                                  │  HTTPS + TLS  │                                         │
│                                  └────────┬────────┘                                         │
└────────────────────────────────────────────────┬───────────────────────────────────────────────────────┘
                                            │
                                            v
                               https://twitch-chatters.vignemail1.eu
```

- **Gateway** : API HTTP, authentification OAuth Twitch, gestion sessions
- **Twitch-API** : Wrapper API Twitch avec rate limiting
- **Worker** : Traitement asynchrone des jobs (captures, enrichissement)
- **Analysis** : API d'analyse et statistiques
- **MariaDB** : Base de données relationnelle (utilisateurs, sessions, captures)
- **Redis** : Cache distribué, sessions, rate limiting
- **Traefik** : Reverse proxy, terminaison TLS, load balancing

### Stack Technique

- **Backend** : Go 1.25
- **Base de données** : MariaDB 11.2
- **Cache** : Redis 7
- **Reverse Proxy** : Traefik v3.2
- **Containerisation** : Docker + Docker Compose
- **TLS** : Let's Encrypt (automatique)

## 🚀 Quick Start

### Prérequis

- Docker 24+
- Docker Compose v2+
- Go 1.25+ (pour développement)
- Compte Twitch Developer (OAuth app)

### Installation

```bash
# Cloner le repository
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# Copier la configuration
cp .env.example .env

# Éditer les variables d'environnement
vim .env
```

### Configuration

Dans `.env`, configurer :

```bash
# Twitch OAuth
TWITCH_CLIENT_ID=votre_client_id
TWITCH_CLIENT_SECRET=votre_client_secret
TWITCH_REDIRECT_URL=https://twitch-chatters.vignemail1.eu/auth/callback

# Base de données
MYSQL_ROOT_PASSWORD=votre_mot_de_passe_root
MYSQL_PASSWORD=votre_mot_de_passe_app

# Session secret
APP_SESSION_SECRET=$(openssl rand -base64 32)

# Email Let's Encrypt
ACME_EMAIL=votre-email@example.com
```

### DNS

Configurer les enregistrements DNS :

```dns
twitch-chatters.vignemail1.eu      A    <IP_SERVEUR>
twitch-chatters-dev.vignemail1.eu  A    <IP_SERVEUR>
traefik.vignemail1.eu              A    <IP_SERVEUR>
```

### Démarrage

```bash
# Démarrer tous les services
docker-compose up -d

# Vérifier les logs
docker-compose logs -f

# Vérifier l'état
docker-compose ps
```

### Accès

- **Application** : https://twitch-chatters.vignemail1.eu
- **Dashboard Traefik** : https://traefik.vignemail1.eu (admin/changeme)

## 📊 Ressources

### Configuration par Défaut (1 replica par service)

```
CPU  : ~3 vCPU (moyenne)
RAM  : ~4 GB
Disk : ~12 GB + données utilisateurs

Serveur recommandé : 4 vCPU, 8 GB RAM, 50 GB SSD
Coût estimé : ~12€/mois (Hetzner CPX31)
```

### Avec Monitoring (Optionnel)

```bash
# Démarrer avec monitoring
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d

# Ressources supplémentaires
CPU  : +1.5 vCPU
RAM  : +2 GB
Disk : +9 GB

Serveur recommandé : 8 vCPU, 12 GB RAM, 80 GB SSD
```

### Scalabilité Horizontale

```bash
# Augmenter les replicas (en cas de charge)
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

# Ressources avec replicas
CPU  : ~6 vCPU
RAM  : ~7 GB

Serveur recommandé : 8 vCPU, 16 GB RAM, 80 GB SSD
Coût estimé : ~25€/mois (Hetzner CPX41)
```

## 📖 Documentation

### Guides Principaux

- [**SCALING.md**](docs/SCALING.md) : Scalabilité et optimisations de performance
- [**TRAEFIK.md**](docs/TRAEFIK.md) : Configuration Traefik et TLS
- [**MONITORING.md**](docs/MONITORING.md) : Stack de monitoring (Prometheus, Grafana, Loki)
- [**RESOURCES.md**](docs/RESOURCES.md) : Besoins en ressources et coûts

### Architecture

- `cmd/gateway/` : Point d'entrée HTTP, OAuth, sessions
- `cmd/worker/` : Traitement asynchrone des jobs
- `cmd/analysis/` : API d'analyse et statistiques
- `cmd/twitch-api/` : Wrapper API Twitch avec rate limiting
- `internal/` : Packages partagés (redis, db, utils)
- `dev/` : Scripts de développement et schema SQL

## 🔧 Développement

### Mode Développement

```bash
# Démarrer en mode dev (1 replica, ports exposés)
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d

# Accès direct
curl http://localhost:8080  # Gateway
curl http://localhost:8083  # Analysis
```

### Build Local

```bash
# Builder les services
docker-compose build

# Ou builder un service spécifique
docker-compose build gateway

# Rebuild sans cache
docker-compose build --no-cache
```

### Tests

```bash
# Tests unitaires
go test ./...

# Tests avec couverture
go test -cover ./...

# Linting
golangci-lint run
```

## 📦 Base de Données

### Migrations

Le schéma est initialisé automatiquement au démarrage via `dev/schema.sql`.

### Backup

```bash
# Backup complet
docker exec twitch-chatters-db mariadb-dump -u root -p twitch_chatters > backup.sql

# Restauration
docker exec -i twitch-chatters-db mariadb -u root -p twitch_chatters < backup.sql
```

### Accès Direct

```bash
# Console MariaDB
docker exec -it twitch-chatters-db mariadb -u twitch -p

# Console Redis
docker exec -it twitch-chatters-redis redis-cli
```

## 🔒 Sécurité

- ✅ TLS automatique via Let's Encrypt
- ✅ Redirection HTTP → HTTPS
- ✅ OAuth Twitch pour authentification
- ✅ Sessions sécurisées (Redis)
- ✅ Rate limiting distribué
- ✅ Mots de passe hashés (bcrypt)
- ✅ Secrets en variables d'environnement

## 📊 Monitoring (Optionnel)

### Services Inclus

- **Prometheus** : Métriques time-series
- **Grafana** : Dashboards et visualisation
- **Loki** : Agrégation logs
- **Alertmanager** : Gestion alertes
- **Exporters** : Node, cAdvisor, Redis, MySQL

### Accès Monitoring

- **Grafana** : https://grafana.vignemail1.eu (admin/admin)
- **Prometheus** : https://prometheus.vignemail1.eu
- **Alertmanager** : https://alerts.vignemail1.eu

## 🔧 Maintenance

### Mise à Jour

```bash
# Pull derniers changements
git pull

# Rebuild et redémarrer
docker-compose build
docker-compose up -d
```

### Nettoyage

```bash
# Arrêter et supprimer les containers
docker-compose down

# Supprimer aussi les volumes (ATTENTION : perte de données)
docker-compose down -v

# Nettoyer images non utilisées
docker system prune -a
```

### Logs

```bash
# Tous les logs
docker-compose logs -f

# Logs d'un service
docker-compose logs -f gateway

# Filtrer les erreurs
docker-compose logs gateway | grep -i error
```

## 🚀 Performance

### Capacité Actuelle (1 replica)

- ✅ 100-500 utilisateurs actifs simultanés
- ✅ 1000-5000 captures/heure
- ✅ 10-50 requêtes HTTP/sec

### Avec Replicas (2 gateway, 3 workers, 2 analysis)

- ✅ 500-1000 utilisateurs actifs
- ✅ 5000-20000 captures/heure
- ✅ 50-200 requêtes HTTP/sec

## 📝 Licence

MIT License - Voir [LICENSE](LICENSE) pour plus de détails

## 👥 Auteur

**vignemail1**
- GitHub: [@vignemail1](https://github.com/vignemail1)
- Email: vignemail1@gmail.com

## 🚀 Roadmap

- [ ] Interface web frontend (React/Vue)
- [ ] Exports CSV/JSON des analyses
- [ ] Webhooks Discord/Slack
- [ ] API publique avec clés d'API
- [ ] Support multi-chaînes
- [ ] Détection des raids
- [ ] Analyse sentiment (IA)

---

**👍 Vous utilisez ce projet ?** N'hésitez pas à ⭐ star le repo !
