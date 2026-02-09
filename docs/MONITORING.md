# Guide de Monitoring

Ce document décrit la stack de monitoring complète pour Twitch Chatters Analyser.

## Architecture Monitoring

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                              MONITORING STACK                              │
│                                                                            │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │                            GRAFANA                                │  │
│  │              Dashboards + Visualisation + Alertes                 │  │
│  │                https://grafana.vignemail1.eu                        │  │
│  └───────────────────┬────────────────────────────────────────────────┘  │
│                        │                                                  │
│         ┌──────────────┼──────────────┐                                 │
│         │               │              │                                 │
│    ┌────v──────────┐ ┌────v──────────┐                                 │
│    │  PROMETHEUS  │ │    LOKI      │                                 │
│    │  Métriques   │ │    Logs      │                                 │
│    └────┬──────────┘ └────┬──────────┘                                 │
│         │               │                                            │
│         v               v                                            │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │                          EXPORTERS                                │  │
│  │  • Node Exporter     (CPU, RAM, Disk)                            │  │
│  │  • cAdvisor          (Containers Docker)                          │  │
│  │  • Redis Exporter    (Métriques Redis)                           │  │
│  │  • MySQL Exporter    (Métriques MariaDB)                         │  │
│  │  • Promtail          (Collecte logs Docker)                       │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                  │                                       │
│                                  v                                       │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │              SERVICES (Gateway, Worker, Analysis)                │  │
│  └────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────────┘
```

## Composants

### 1. Prometheus
- **Rôle** : Collecte et stockage des métriques time-series
- **Port** : 9090
- **URL** : https://prometheus.vignemail1.eu
- **Rétention** : 30 jours

### 2. Grafana
- **Rôle** : Visualisation, dashboards, alertes
- **Port** : 3000
- **URL** : https://grafana.vignemail1.eu
- **Login par défaut** : admin / admin (changer au premier login)

### 3. Loki
- **Rôle** : Agrégation et indexation des logs
- **Port** : 3100 (interne)
- **Rétention** : 30 jours

### 4. Promtail
- **Rôle** : Collecte des logs Docker et envoi vers Loki
- **Cible** : Tous les containers `twitch-chatters-*`

### 5. Exporters

#### Node Exporter
- Métriques système : CPU, RAM, Disk, Network
- Port : 9100

#### cAdvisor
- Métriques containers Docker
- Port : 8080

#### Redis Exporter
- Métriques Redis : connexions, mémoire, hit rate
- Port : 9121

#### MySQL Exporter
- Métriques MariaDB : connexions, requêtes, locks
- Port : 9104

### 6. Alertmanager
- **Rôle** : Gestion des alertes (routing, grouping, silencing)
- **Port** : 9093
- **URL** : https://alerts.vignemail1.eu

## Déploiement

### Démarrage

```bash
# Démarrer avec monitoring
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d

# Vérifier les services
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml ps

# Logs
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml logs -f grafana prometheus
```

### Configuration DNS

Ajouter ces enregistrements DNS :

```dns
grafana.vignemail1.eu     A    <IP_SERVEUR>
prometheus.vignemail1.eu  A    <IP_SERVEUR>
alerts.vignemail1.eu      A    <IP_SERVEUR>
```

### Premier Accès Grafana

```bash
# Accéder à Grafana
open https://grafana.vignemail1.eu

# Login : admin / admin
# Changer le mot de passe au premier login
```

## Métriques Collectées

### Système (Node Exporter)

```promql
# CPU Usage
100 - (avg by(instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)

# Memory Usage
(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100

# Disk Usage
(1 - (node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})) * 100

# Network Traffic
irate(node_network_receive_bytes_total[5m])
irate(node_network_transmit_bytes_total[5m])
```

### Containers (cAdvisor)

```promql
# CPU par container
rate(container_cpu_usage_seconds_total{name=~"twitch-chatters.*"}[5m]) * 100

# Memory par container
container_memory_usage_bytes{name=~"twitch-chatters.*"} / 1024 / 1024

# Network par container
rate(container_network_receive_bytes_total{name=~"twitch-chatters.*"}[5m])
```

### Redis

```promql
# Connexions actives
redis_connected_clients

# Mémoire utilisée
redis_memory_used_bytes / redis_memory_max_bytes * 100

# Hit rate
rate(redis_keyspace_hits_total[5m]) / (rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m])) * 100

# Operations par seconde
rate(redis_commands_processed_total[5m])
```

### MySQL

```promql
# Connexions actives
mysql_global_status_threads_connected

# Utilisation connexions
(mysql_global_status_threads_connected / mysql_global_variables_max_connections) * 100

# Queries par seconde
rate(mysql_global_status_questions[5m])

# Slow queries
rate(mysql_global_status_slow_queries[5m])

# InnoDB buffer pool hit rate
(mysql_global_status_innodb_buffer_pool_read_requests - mysql_global_status_innodb_buffer_pool_reads) / mysql_global_status_innodb_buffer_pool_read_requests * 100
```

## Dashboards Grafana

### Dashboard Principal : Twitch Chatters Overview

Un dashboard pré-configuré est fourni avec :

1. **Vue Système**
   - CPU Usage
   - Memory Usage
   - Disk Usage
   - Network Traffic

2. **État Services**
   - Gateway Status (UP/DOWN)
   - MySQL Status
   - Redis Status
   - Worker Status

3. **Métriques Application**
   - Connexions MySQL
   - Redis Operations
   - Container Resources

### Importer des Dashboards Communautaires

Grafana propose des dashboards prêt à l'emploi :

```
1. Node Exporter Full         : ID 1860
2. Docker Container Metrics   : ID 193
3. MySQL Overview             : ID 7362
4. Redis Dashboard            : ID 11835
5. Traefik Dashboard          : ID 12250
```

**Import** :
1. Grafana → Dashboards → Import
2. Entrer l'ID
3. Sélectionner datasource Prometheus
4. Import

## Alertes

### Alertes Configurées

#### Système

| Alerte | Condition | Durée | Sévérité |
|--------|-----------|------|----------|
| HighCPUUsage | CPU > 80% | 5min | Warning |
| HighMemoryUsage | RAM > 85% | 5min | Warning |
| DiskSpaceLow | Disk < 15% | 5min | Warning |

#### Containers

| Alerte | Condition | Durée | Sévérité |
|--------|-----------|------|----------|
| ContainerDown | up == 0 | 2min | Critical |
| ContainerHighCPU | CPU > 80% | 5min | Warning |
| ContainerHighMemory | RAM > 85% | 5min | Warning |

#### Base de Données

| Alerte | Condition | Durée | Sévérité |
|--------|-----------|------|----------|
| MySQLDown | mysql_up == 0 | 1min | Critical |
| MySQLTooManyConnections | Connections > 80% | 5min | Warning |
| MySQLSlowQueries | Slow queries > 5/s | 5min | Warning |
| RedisDown | redis_up == 0 | 1min | Critical |
| RedisHighMemory | Memory > 90% | 5min | Warning |

### Configuration Notifications

Pour recevoir les alertes, éditer `monitoring/alertmanager/alertmanager.yml` :

#### Email

```yaml
receivers:
  - name: 'critical'
    email_configs:
      - to: 'admin@vignemail1.eu'
        from: 'alertmanager@vignemail1.eu'
        smarthost: 'smtp.gmail.com:587'
        auth_username: 'your-email@gmail.com'
        auth_password: 'your-app-password'
```

#### Webhook (Discord, Slack, etc.)

```yaml
receivers:
  - name: 'critical'
    webhook_configs:
      - url: 'https://discord.com/api/webhooks/YOUR_WEBHOOK'
        send_resolved: true
```

#### Grafana Alerting

Grafana peut aussi envoyer des alertes directement :

1. Grafana → Alerting → Contact points
2. New contact point
3. Choisir le type (Email, Slack, Discord, etc.)
4. Configurer

## Logs (Loki)

### Requêter les Logs

Dans Grafana, onglet **Explore** avec datasource **Loki** :

```logql
# Tous les logs gateway
{container="gateway"}

# Logs d'erreur
{container=~"gateway|worker|analysis"} |= "error"

# Logs avec niveau ERROR
{container="gateway"} | json | level="ERROR"

# Requêtes HTTP lentes
{container="gateway"} | json | duration > 1000

# Logs des 5 dernières minutes
{container="worker"} [5m]
```

### Agrégations

```logql
# Nombre d'erreurs par minute
sum(rate({container=~"gateway|worker|analysis"} |= "error" [1m])) by (container)

# Top 10 endpoints
topk(10, sum by (path) (rate({container="gateway"} | json [5m])))
```

## Maintenance

### Backup Métriques

```bash
# Backup Prometheus data
docker cp twitch-chatters-prometheus:/prometheus ./backup/prometheus-data

# Backup Grafana dashboards
docker cp twitch-chatters-grafana:/var/lib/grafana ./backup/grafana-data
```

### Nettoyage

```bash
# Libérer l'espace Prometheus (si besoin)
docker exec twitch-chatters-prometheus rm -rf /prometheus/wal

# Libérer l'espace Loki
docker exec twitch-chatters-loki rm -rf /loki/chunks/fake
```

### Redémarrage

```bash
# Redémarrer un service
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml restart prometheus

# Redémarrer toute la stack monitoring
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml restart
```

## Optimisations

### Réduire l'Utilisation Disque

Dans `monitoring/prometheus/prometheus.yml` :

```yaml
global:
  scrape_interval: 30s      # Au lieu de 15s
  
command:
  - '--storage.tsdb.retention.time=15d'  # Au lieu de 30d
```

Dans `monitoring/loki/loki.yml` :

```yaml
limits_config:
  retention_period: 15d     # Au lieu de 30d
```

### Réduire la Charge CPU

```yaml
# Réduire la fréquence de scrape pour certains jobs
scrape_configs:
  - job_name: 'node-exporter'
    scrape_interval: 60s    # Au lieu de 15s
```

## Troubleshooting

### Prometheus ne scrape pas les targets

```bash
# Vérifier les targets
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[].health'

# Logs Prometheus
docker-compose logs prometheus | grep -i error
```

### Grafana ne se connecte pas à Prometheus

```bash
# Tester depuis Grafana
docker-compose exec grafana wget -qO- http://prometheus:9090/api/v1/query?query=up

# Vérifier la datasource
# Grafana → Configuration → Data Sources → Prometheus → Test
```

### Loki ne reçoit pas de logs

```bash
# Vérifier Promtail
docker-compose logs promtail | grep -i error

# Tester Loki
curl -s http://localhost:3100/ready

# Vérifier les logs collectés
curl -s 'http://localhost:3100/loki/api/v1/label/__name__/values'
```

## Ressources Utiles

- [Prometheus Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
- [Grafana Dashboards](https://grafana.com/grafana/dashboards/)
- [LogQL Cheat Sheet](https://grafana.com/docs/loki/latest/logql/)
- [Alertmanager Configuration](https://prometheus.io/docs/alerting/latest/configuration/)
