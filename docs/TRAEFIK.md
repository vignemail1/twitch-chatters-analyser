# Traefik - Reverse Proxy et TLS

Ce document explique la configuration Traefik avec terminaison TLS automatique via Let's Encrypt.

## Vue d'ensemble

```
                    Internet
                       │
                       v
              ┌───────────┐
              │   Traefik   │  Ports 80/443
              │  + Let's    │  TLS auto
              │  Encrypt    │
              └─────┬──────┘
                   │
        ┌──────────┼─────────┐
        │           │          │
        v           v          v
   Gateway     Twitch-API   Analysis
    :8080       :8081       :8083
```

## Domaines Configurés

### Production
- **Application** : `https://twitch-chatters.vignemail1.eu`
- **Dashboard Traefik** : `https://traefik.vignemail1.eu`
  - User: `admin`
  - Password: (défini via `TRAEFIK_AUTH` dans `.env`)

### Développement
- **Application** : `https://twitch-chatters-dev.vignemail1.eu`
- **Dashboard Traefik** : `http://localhost:8090` (pas d'auth)

## Configuration DNS Requise

Avant de déployer, configure ces enregistrements DNS chez ton registrar :

```
# Production
twitch-chatters.vignemail1.eu    A      <IP_SERVEUR>
traefik.vignemail1.eu            A      <IP_SERVEUR>

# Développement (si serveur de dev séparé)
twitch-chatters-dev.vignemail1.eu A      <IP_SERVEUR_DEV>
```

## Let's Encrypt

### Configuration

Traefik est configuré pour obtenir automatiquement des certificats Let's Encrypt via HTTP-01 challenge.

```yaml
certificatesresolvers.letsencrypt.acme.email: admin@vignemail1.eu
certificatesresolvers.letsencrypt.acme.httpchallenge: true
```

### Certificats Stockés

Les certificats sont stockés dans le volume Docker `traefik_letsencrypt` :

```bash
# Voir le contenu (JSON)
docker exec twitch-chatters-traefik cat /letsencrypt/acme.json

# Backup des certificats
docker cp twitch-chatters-traefik:/letsencrypt/acme.json ./backup-acme.json
```

### Renouvellement Automatique

Let's Encrypt renouvelle automatiquement les certificats avant expiration (90 jours).

## Déploiement

### 1. Configuration Initiale

```bash
# Copier l'exemple de configuration
cp .env.example .env

# Éditer les variables
vim .env
```

Variables importantes :
```bash
APP_ENV=production
ACME_EMAIL=ton-email@example.com
TWITCH_REDIRECT_URL=https://twitch-chatters.vignemail1.eu/auth/callback
```

### 2. Générer le mot de passe Dashboard

```bash
# Installer htpasswd (si pas déjà installé)
# Debian/Ubuntu:
sudo apt-get install apache2-utils

# macOS:
brew install httpd

# Générer le hash (remplacer 'ton_mot_de_passe')
echo $(htpasswd -nB admin) | sed -e s/\$/\$\$/g

# Copier le résultat dans .env
TRAEFIK_AUTH=admin:$$2y$$05$$...
```

### 3. Démarrer les Services

```bash
# Production (avec TLS)
docker-compose up -d

# Vérifier les logs Traefik
docker-compose logs -f traefik

# Vérifier l'obtention des certificats
docker-compose logs traefik | grep -i "certificate"
```

### 4. Vérification

```bash
# Tester l'accès HTTPS
curl -I https://twitch-chatters.vignemail1.eu

# Vérifier le certificat
openssl s_client -connect twitch-chatters.vignemail1.eu:443 -servername twitch-chatters.vignemail1.eu < /dev/null | grep -A 2 "Verify return code"

# Dashboard Traefik
curl -u admin:ton_mot_de_passe https://traefik.vignemail1.eu
```

## Mode Développement

### Démarrage

```bash
# Démarrer en mode dev
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d

# Accès
curl http://localhost:8080          # Gateway direct
curl http://localhost:8090          # Dashboard Traefik
```

### Certificats Auto-Signés (Optionnel)

Si tu veux tester HTTPS en local sans Let's Encrypt :

```bash
# Générer certificat auto-signé
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout traefik/certs/local-key.pem \
  -out traefik/certs/local-cert.pem \
  -subj "/CN=twitch-chatters-dev.vignemail1.eu"
```

## Load Balancing

### Configuration Actuelle (Single Instance)

Traefik route toutes les requêtes vers l'instance unique du gateway :

```yaml
gateway:
  container_name: twitch-chatters-gateway  # 1 instance
  labels:
    - "traefik.http.services.tca-gateway.loadbalancer.server.port=8080"
```

### Health Checks

Traefik vérifie la santé de chaque instance :

```yaml
traefik.http.services.tca-gateway.loadbalancer.healthcheck.path: /healthz
traefik.http.services.tca-gateway.loadbalancer.healthcheck.interval: 10s
```

Si une instance échoue au health check, Traefik arrête de router vers elle.

### Load Balancing Multi-Réplicas (Future)

Si vous migrez vers un système multi-réplicas (voir [SCALING.md](SCALING.md)), Traefik fera automatiquement du load balancing entre les replicas :

```yaml
gateway:
  # container_name retiré pour permettre le scaling
  deploy:
    replicas: 2  # 2 instances
  labels:
    - "traefik.http.services.tca-gateway.loadbalancer.server.port=8080"
```

Traefik détecte automatiquement les nouvelles instances via Docker labels et les ajoute au pool de load balancing.

## Sécurité

### Redirection HTTP → HTTPS

Automatique en production :

```yaml
entrypoints.web.http.redirections.entrypoint.to: websecure
entrypoints.web.http.redirections.entrypoint.scheme: https
```

### Headers de Sécurité (Recommandé)

Ajouter dans les labels du gateway :

```yaml
labels:
  # Headers de sécurité
  - "traefik.http.middlewares.security-headers.headers.stsSeconds=31536000"
  - "traefik.http.middlewares.security-headers.headers.stsIncludeSubdomains=true"
  - "traefik.http.middlewares.security-headers.headers.stsPreload=true"
  - "traefik.http.middlewares.security-headers.headers.forceSTSHeader=true"
  - "traefik.http.middlewares.security-headers.headers.customResponseHeaders.X-Frame-Options=SAMEORIGIN"
  - "traefik.http.middlewares.security-headers.headers.customResponseHeaders.X-Content-Type-Options=nosniff"
  - "traefik.http.routers.tca-gateway-prod.middlewares=security-headers"
```

## Monitoring

### Dashboard Traefik

Access : `https://traefik.vignemail1.eu`

Affiche :
- État des services (UP/DOWN)
- Certificats actifs
- Routers configurés
- Middlewares actifs

### Logs

```bash
# Logs Traefik
docker-compose logs -f traefik

# Filtrer les erreurs
docker-compose logs traefik | grep -i error

# Accès logs (requêtes HTTP)
docker-compose logs traefik | grep "GET\|POST"
```

## Dépannage

### Certificat non obtenu

**Problème** : Let's Encrypt n'arrive pas à valider le domaine

**Solutions** :

1. Vérifier DNS
```bash
dig twitch-chatters.vignemail1.eu
# Doit pointer vers l'IP du serveur
```

2. Vérifier port 80 ouvert (HTTP challenge)
```bash
curl -I http://twitch-chatters.vignemail1.eu
```

3. Consulter les logs ACME
```bash
docker-compose logs traefik | grep -i acme
```

4. Supprimer acme.json et redémarrer (si problème persistant)
```bash
docker-compose down
docker volume rm twitch-chatters-analyser_traefik_letsencrypt
docker-compose up -d
```

### Rate Limit Let's Encrypt

Let's Encrypt limite à **5 certificats/semaine** par domaine.

**Staging** (pour tests) :

```yaml
# Dans docker-compose.yml, remplacer temporairement :
certificatesresolvers.letsencrypt.acme.caServer: https://acme-staging-v02.api.letsencrypt.org/directory
```

### Gateway non accessible

**Vérifications** :

```bash
# Vérifier que Traefik voit le service
docker-compose exec traefik wget -qO- http://localhost:8080/api/http/routers

# Vérifier que le gateway répond
docker-compose exec gateway wget -qO- http://localhost:8080/healthz

# Vérifier les labels
docker inspect twitch-chatters-gateway | grep traefik
```

## Backup et Restauration

### Backup

```bash
# Certificats Let's Encrypt
docker cp twitch-chatters-traefik:/letsencrypt/acme.json ./backup/

# Configuration
cp docker-compose.yml .env ./backup/
```

### Restauration

```bash
# Restaurer les certificats
docker cp ./backup/acme.json twitch-chatters-traefik:/letsencrypt/
docker-compose restart traefik
```

## Ressources

- [Documentation Traefik](https://doc.traefik.io/traefik/)
- [Let's Encrypt Rate Limits](https://letsencrypt.org/docs/rate-limits/)
- [Traefik + Docker](https://doc.traefik.io/traefik/providers/docker/)
