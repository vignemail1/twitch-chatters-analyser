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
│  │                 └────────────────┬────────────────────────┘                     │  │
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
- **Reverse Proxy** : Traefik v3.6
- **Containerisation** : Docker + Docker Compose
- **TLS** : Let's Encrypt (automatique)
- **Gestion d'environnement** : mise / fnox

## 🚀 Quick Start

### Prérequis

- Docker 24+
- Docker Compose v2+
- Go 1.25+ (pour développement)
- [mise](https://mise.jdx.dev) ou [fnox](https://fnox.jdx.dev) (recommandé)
- Compte Twitch Developer (OAuth app)

### Installation avec mise/fnox (Recommandé)

```bash
# Installer mise (https://mise.jdx.dev/getting-started.html)
curl https://mise.run | sh

# Ou installer fnox (https://fnox.jdx.dev)
cargo install --locked fnox

# Cloner le repository
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# Activer direnv (optionnel mais recommandé)
direnv allow

# Installer les outils (Go, Docker, etc.)
mise install
```

## 🌐 Environnements (Dev / Prod)

### Profils Disponibles

Le projet utilise **mise** pour gérer plusieurs environnements avec des configurations séparées :

| Profil | Fichier | Usage | Monitoring | Domaine |
|--------|---------|-------|------------|----------|
| **development** | `.env.development` | Développement local | ❌ Désactivé | `twitch-chatters-dev.vignemail1.eu` |
| **staging** | `.env.staging` | Tests pré-prod | ✅ Activé | `twitch-chatters-staging.vignemail1.eu` |
| **production** | `.env.production` | Production | ✅ Activé | `twitch-chatters.vignemail1.eu` |

### Configuration Initiale

```bash
# 1. Générer les secrets
mise run env-generate > secrets.txt

# 2. Créer les fichiers d'environnement
cp .env.example .env.development
cp .env.example .env.production

# 3. Éditer chaque fichier avec des secrets DIFFÉRENTS
vim .env.development
vim .env.production

# 🔒 IMPORTANT: Utiliser des secrets différents pour dev et prod!
# - Apps Twitch séparées
# - Mots de passe MariaDB différents
# - Secrets de session différents
```

### Utilisation des Profils

#### Mode Development (par défaut)

```bash
# Activer le profil development (par défaut)
export MISE_ENV=development
# ou
mise run env:dev

# Vérifier la configuration
mise run env-check
# 🌐 Environnement: development
# 🔗 Redirect URL: https://twitch-chatters-dev.vignemail1.eu/auth/callback
# 📊 Monitoring: false

# Démarrer en mode dev
mise run up
# 🚀 Démarrage sans monitoring (development)

# Ou avec ports exposés pour debug
mise run up:dev
```

#### Mode Production

```bash
# Activer le profil production
export MISE_ENV=production
# ou
mise run env:prod

# Vérifier la configuration
mise run env-check
# 🌐 Environnement: production
# 🔗 Redirect URL: https://twitch-chatters.vignemail1.eu/auth/callback
# 📊 Monitoring: true

# Démarrer en mode prod (avec monitoring)
mise run up
# 📊 Démarrage avec monitoring (production)

# Ou utiliser la tâche dédiée
mise run up:prod
```

#### Mode Staging

```bash
# Activer le profil staging
export MISE_ENV=staging
mise run env:staging

# Démarrer
mise run up
```

### Différences par Environnement

#### Development
```bash
# .env.development
APP_ENV=development
LOG_LEVEL=DEBUG
TWITCH_REDIRECT_URL=https://twitch-chatters-dev.vignemail1.eu/auth/callback
RATE_LIMIT_REQUESTS_PER_SECOND=50  # Plus permissif
JOB_POLL_INTERVAL=1                 # Plus rapide
CACHE_TTL_SECONDS=30                # Cache court
ENABLE_MONITORING=false             # Pas de monitoring
REDIS_PORT=6379                     # Exposé pour debug
MYSQL_PORT=3306                     # Exposé pour debug
```

#### Production
```bash
# .env.production
APP_ENV=production
LOG_LEVEL=INFO
TWITCH_REDIRECT_URL=https://twitch-chatters.vignemail1.eu/auth/callback
RATE_LIMIT_REQUESTS_PER_SECOND=10  # Conservateur
JOB_POLL_INTERVAL=2                 # Standard
CACHE_TTL_SECONDS=300               # Cache long
ENABLE_MONITORING=true              # Monitoring actif
REDIS_PORT=                         # Non exposé
MYSQL_PORT=                         # Non exposé
```

### Changer de Profil

```bash
# Méthode 1: Variable d'environnement
export MISE_ENV=production
cd . # Recharger direnv

# Méthode 2: Tâche mise
mise run env:prod

# Méthode 3: Inline
MISE_ENV=production mise run up
```

## 🔒 Sécurité

### Gestion des Secrets

**✅ Bonnes pratiques implémentées** :
- ❌ **Aucun** mot de passe ou secret en clair dans le code
- ✅ **Fichiers séparés** par environnement (`.env.development`, `.env.production`)
- ✅ **Secrets différents** pour dev et prod (obligatoire)
- ✅ Génération automatique des secrets forts
- ✅ Vérification des variables requises au démarrage
- ✅ Documentation complète dans `.env.example`

### Variables Requises

```bash
# Vérifier que toutes les variables sont définies
mise run env-check

# Générer des secrets forts automatiquement
mise run env-generate
```

### Sécurité Infrastructure

- ✅ TLS automatique via Let's Encrypt
- ✅ Redirection HTTP → HTTPS
- ✅ OAuth Twitch pour authentification
- ✅ Sessions sécurisées (Redis)
- ✅ Rate limiting distribué
- ✅ Mots de passe hashés (bcrypt)
- ✅ Base de données non exposée publiquement (prod)

## 🛠️ Tâches mise

### Tâches Principales

```bash
# Lister toutes les tâches
mise tasks

# Environnement
mise run env:dev        # Activer profil development
mise run env:prod       # Activer profil production
mise run env:staging    # Activer profil staging
mise run env-check      # Vérifier les variables
mise run env-generate   # Générer secrets

# Build & Deploy
mise run install        # go mod download
mise run build          # docker-compose build
mise run build:nocache  # docker-compose build --no-cache
mise run up             # Démarrer (selon profil)
mise run up:dev         # Démarrer mode dev
mise run up:prod        # Démarrer mode prod
mise run down           # Arrêter
mise run down:volumes   # Arrêter + supprimer volumes
mise run restart        # Redémarrer

# Logs & Debug
mise run logs           # Tous les logs
mise run logs:gateway   # Logs gateway
mise run logs:worker    # Logs worker
mise run logs:db        # Logs database
mise run ps             # État des services

# Tests & Qualité
mise run test           # Tests unitaires
mise run test:coverage  # Tests + couverture
mise run lint           # Linter Go
mise run lint:fix       # Linter + fix auto

# Base de données
mise run db-backup      # Backup BDD
mise run db-restore <file.sql>  # Restaurer BDD
mise run db-console     # Console MariaDB
mise run redis-console  # Console Redis

# Maintenance
mise run clean          # Nettoyer fichiers temp
```

### Exemples d'Utilisation

```bash
# Workflow Development
export MISE_ENV=development
mise run env-check
mise run build
mise run up:dev
mise run logs:gateway

# Workflow Production
export MISE_ENV=production
mise run env-check
mise run build:nocache
mise run up:prod
mise run logs

# Backup production
export MISE_ENV=production
mise run db-backup
# backup-production-20260209-143000.sql

# Tester en dev avec dump prod
export MISE_ENV=development
mise run db-restore backup-production-20260209-143000.sql
```

## 🦞 fnox (Alternative à mise)

[fnox](https://fnox.jdx.dev) est **100% compatible** avec la configuration mise :

```bash
# Installer fnox
cargo install --locked fnox

# Utilisation identique
export MISE_ENV=development
fnox install
fnox run up
fnox run logs
fnox run env-check
```

## 📊 Ressources

### Configuration par Défaut (Development, 1 replica)

```
CPU  : ~3 vCPU (moyenne)
RAM  : ~4 GB
Disk : ~12 GB + données utilisateurs

Serveur recommandé : 4 vCPU, 8 GB RAM, 50 GB SSD
Coût estimé : ~12€/mois (Hetzner CPX31)
```

### Production avec Monitoring

```
CPU  : ~5 vCPU
RAM  : ~6 GB
Disk : ~21 GB + données utilisateurs

Serveur recommandé : 8 vCPU, 12 GB RAM, 80 GB SSD
Coût estimé : ~25€/mois (Hetzner CPX41)
```

### Avec Replicas (2 gateway, 3 workers, 2 analysis)

```bash
# Augmenter les replicas (en cas de charge)
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

CPU  : ~6 vCPU
RAM  : ~7 GB

Serveur recommandé : 8 vCPU, 16 GB RAM, 80 GB SSD
Coût estimé : ~30€/mois (Hetzner CPX41)
```

## 📖 Documentation

### Guides Principaux

- [**SCALING.md**](docs/SCALING.md) : Scalabilité et optimisations de performance
- [**TRAEFIK.md**](docs/TRAEFIK.md) : Configuration Traefik et TLS
- [**MONITORING.md**](docs/MONITORING.md) : Stack de monitoring (Prometheus, Grafana, Loki)
- [**RESOURCES.md**](docs/RESOURCES.md) : Besoins en ressources et coûts
- [**DATABASE.md**](docs/DATABASE.md) : Structure BDD et migrations

### Architecture

- `cmd/gateway/` : Point d'entrée HTTP, OAuth, sessions
- `cmd/worker/` : Traitement asynchrone des jobs
- `cmd/analysis/` : API d'analyse et statistiques
- `cmd/twitch-api/` : Wrapper API Twitch avec rate limiting
- `internal/` : Packages partagés (redis, db, utils)
- `dev/` : Scripts de développement et schema SQL

## 🔧 Développement

### Setup Initial

```bash
# 1. Cloner et installer
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser
mise install

# 2. Configurer development
cp .env.example .env.development
mise run env-generate >> .env.development
vim .env.development  # Ajouter TWITCH_CLIENT_ID, etc.

# 3. Vérifier et démarrer
export MISE_ENV=development
mise run env-check
mise run up:dev
```

### Mode Développement

```bash
# Démarrer avec ports exposés
mise run up:dev

# Accès direct aux services
curl http://localhost:8080/healthz  # Gateway
curl http://localhost:8083/healthz  # Analysis
redis-cli -p 6379                   # Redis
mysql -h 127.0.0.1 -P 3306 -u twitch -p  # MariaDB
```

### Tests

```bash
# Tests unitaires
mise run test

# Tests avec couverture
mise run test:coverage

# Linting
mise run lint
mise run lint:fix
```

## 📦 Base de Données

### Migrations

Le schéma est initialisé automatiquement au démarrage via `dev/schema.sql`.

Migrations manuelles dans `dev/migrations/` :

```bash
# Appliquer une migration
docker exec -i twitch-chatters-db mariadb -u root -p"$MYSQL_ROOT_PASSWORD" twitch_chatters < dev/migrations/001_limit_saved_sessions.sql
```

### Backup & Restore

```bash
# Backup automatique (utilise $APP_ENV)
mise run db-backup
# backup-development-20260209-143000.sql

# Restore
mise run db-restore backup-development-20260209-143000.sql
```

### Accès Direct

```bash
# Console MariaDB
mise run db-console

# Console Redis
mise run redis-console
```

## 📊 Monitoring (Production)

### Activation

```bash
# Le monitoring est activé automatiquement en profil production
export MISE_ENV=production
mise run up
# 📊 Démarrage avec monitoring (production)
```

### Services Inclus

- **Prometheus** : Métriques time-series
- **Grafana** : Dashboards et visualisation
- **Loki** : Agrégation logs
- **Alertmanager** : Gestion alertes
- **Exporters** : Node, cAdvisor, Redis, MySQL

### Accès

- **Grafana** : https://grafana.vignemail1.eu (admin/admin)
- **Prometheus** : https://prometheus.vignemail1.eu
- **Alertmanager** : https://alerts.vignemail1.eu

## 🔧 Maintenance

### Mise à Jour

```bash
# Pull derniers changements
git pull

# Rebuild et redémarrer
mise run build:nocache
mise run restart
```

### Nettoyage

```bash
# Arrêter services
mise run down

# Supprimer aussi les volumes (ATTENTION: perte de données)
mise run down:volumes

# Nettoyer images non utilisées
docker system prune -a

# Nettoyer fichiers temp Go
mise run clean
```

## 🚀 Performance

### Capacité Actuelle (Development, 1 replica)

- ✅ 100-500 utilisateurs actifs simultanés
- ✅ 1000-5000 captures/heure
- ✅ 10-50 requêtes HTTP/sec

### Production avec Replicas

```bash
# 2 gateway, 3 workers, 2 analysis
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

# Capacité
- ✅ 500-1000 utilisateurs actifs
- ✅ 5000-20000 captures/heure
- ✅ 50-200 requêtes HTTP/sec
```

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
