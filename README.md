# ops — Controlled Operations & Safe Upgrade Toolkit for Docker Compose

[![Go Report Card](https://goreportcard.com/badge/github.com/ConstasJ/ops)](https://goreportcard.com/report/github.com/ConstasJ/ops)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ConstasJ/ops)](https://github.com/ConstasJ/ops/releases)

`ops` is a lightweight, single-binary operations and controlled upgrade toolkit designed for Docker and Docker Compose in production environments.
Unlike background daemons like Watchtower or WUD, `ops` is a **zero-daemon, standalone CLI tool** that communicates natively with Docker Engine via `/var/run/docker.sock` and the Docker Compose Go SDK.

[English Documentation](README.md) | [中文说明](README_zh.md)

---

## 🌟 Key Features

- 🔒 **Pre-Upgrade Snapshot Gate**: For stateful services (PostgreSQL, Redis, SQLite, Gitea, MariaDB), `ops` automatically triggers an in-container streaming `zstd` logical snapshot before modifying containers. Any snapshot failure halts the upgrade immediately.
- 🛡️ **5-Level Protection Guard**:
  - `db`: Strict snapshot requirement, health probe verification, atomic rollback.
  - `assisted`: Standard safe upgrade with Tag rewrite and probe verification.
  - `auto`: Autonomous upgrade for stateless services via cron or CI.
  - `locked`: Locked version channel; upgrades are blocked.
  - `frozen`: Pinned schema and version; never upgrade automatically.
- 🏷️ **Declarative Docker Labels**: Zero external config required. Declare policies directly in `docker-compose.yml` (`ops.guard="db"`, `ops.probe.http="http://:8080/health"`).
- 🔄 **High-Precision Health Probes & Atomic Rollback**: Millisecond-level HTTP/TCP liveness checks and settle-window validation. If a probe fails, `ops` rolls back both the container and the compose YAML file automatically.
- 🚀 **Native Registry Scanner**: Sub-second remote image tag and manifest digest scanning with GGCR. Inherits local Docker credentials (`~/.docker/config.json`, `docker-credential-pass`).
- 📝 **Audit Trail & Multi-Channel Alerts**: Automatically generates structured Markdown upgrade reports in `/var/lib/ops/reports/` with rolling retention (last 20), supporting Webhooks (Feishu, DingTalk, Telegram, Slack) and SMTP email alerts.

---

## 🚀 Quick Start

### Method 1: One-Line Installer (Linux & macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/ConstasJ/ops/main/install.sh | sh
```

### Method 2: Standalone Static Binaries

Download pre-compiled static binaries from [GitHub Releases](https://github.com/ConstasJ/ops/releases):
- `ops-linux-amd64` (Standard x86_64 Linux servers)
- `ops-linux-arm64` (ARM64 servers / Raspberry Pi)
- `ops-windows-amd64.exe` (Windows Server)
- `ops-darwin-arm64` (Apple Silicon Mac)

```bash
# Example for Linux amd64
sudo curl -Lo /usr/local/bin/ops https://github.com/ConstasJ/ops/releases/latest/download/ops-linux-amd64
sudo chmod +x /usr/local/bin/ops
```

### Method 3: Docker Sidecar

Run as an ephemeral container mounting the Docker socket:

```bash
docker run --rm -it \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/ops:/var/lib/ops \
  ghcr.io/constasj/ops:latest status
```

---

## 📖 CLI Cheatsheet

| Command | Description | Typical Use Case |
|---|---|---|
| `ops status` | Overview of all containers, health gates & restart counts | Daily rapid inspection |
| `ops ls` | List managed containers with protection levels and notes | Asset and policy audit |
| `ops doctor` | Comprehensive health check (unhealthy, crash-loops, stale snapshots) | Troubleshooting |
| `ops pending` | Scan upstream registries and display available updates | Pre-upgrade discovery |
| `ops update <target>` | Controlled single-service upgrade with snapshot and probe gates | Granular service update |
| `ops upgrade` | Batch upgrade available services with Markdown audit report | Periodic maintenance |
| `ops rollback <target>` | Atomically revert a service and compose YAML to last known good state | Emergency recovery |
| `ops dump <target>` | Trigger an on-demand container logical snapshot | Manual pre-change backup |

---

## 🏷️ Docker Compose Labels

Declare operational policies directly inside your service definition:

```yaml
version: '3.8'

services:
  web:
    image: myorg/web-app:1.2.0
    labels:
      - "ops.guard=assisted"
      - "ops.probe.http=http://:8080/health"
      - "ops.timeout=60s"
      - "ops.settle=10s"

  database:
    image: postgres:16-alpine
    labels:
      - "ops.guard=db"
      - "ops.kind=postgres"
      - "ops.probe.tcp=:5432"
      - "ops.notes=Primary database; pre-upgrade snapshot mandatory"
```

---

## ⚙️ Configuration File (`containers.json`)

`ops` also supports a centralized policy repository (`/etc/ops/containers.json` or `/opt/ops/containers.json`):

```json
{
  "_comment": "ops central containers registry",
  "linknet-db-1": {
    "kind": "postgres",
    "guard": "db",
    "probe": {
      "tcp": ":5432"
    },
    "timeout": 180,
    "settle": 15,
    "notes": "Production PostgreSQL cluster"
  }
}
```

> **Precedence**: Docker Labels > `containers.json` > Heuristic Inference.

---

## 📢 Notification Configuration

When executing `ops upgrade --notify`, configure environment variables to broadcast audit reports:

```bash
# Feishu / Lark Webhook
export OPS_FEISHU_WEBHOOK="https://open.feishu.cn/open-apis/bot/v2/hook/xxxx"

# DingTalk Webhook
export OPS_DINGTALK_WEBHOOK="https://oapi.dingtalk.com/robot/send?access_token=xxxx"

# Telegram Bot
export OPS_TELEGRAM_BOT_TOKEN="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
export OPS_TELEGRAM_CHAT_ID="-100123456789"

# SMTP Email
export SMTP_HOST="smtp.example.com"
export SMTP_PORT="587"
export SMTP_USER="alert@example.com"
export SMTP_PASS="secret"
export SMTP_TO="admin@example.com"
```

---

## 📄 License

Licensed under the [MIT License](LICENSE).
