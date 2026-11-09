# ops — 面向 Docker Compose 的受控运维与安全更新工具箱

[![Go Report Card](https://goreportcard.com/badge/github.com/ConstasJ/ops)](https://goreportcard.com/report/github.com/ConstasJ/ops)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ConstasJ/ops)](https://github.com/ConstasJ/ops/releases)

`ops` 是一个轻量级、面向 Docker 与 Docker Compose 生产环境的受控运维与持续升级工具箱。
不同于传统的 Watchtower 或 WUD 等常驻外部守护进程，`ops` 是一个**零常驻后台、纯静态单二进制**的工程利器，原生通过 Docker Engine Socket (`/var/run/docker.sock`) 与 Compose Go SDK 协同工作。

[English Documentation](README.md) | [中文说明](README_zh.md)

---

## 🌟 核心特性

- 🔒 **前置快照门禁 (Pre-upgrade Snapshot Gate)**：针对 PostgreSQL、Redis、SQLite、Gitea、MariaDB 等核心有状态服务，升级前**强制自动在 Docker 容器内部直通管道执行流式 `zstd` 逻辑备份**，快照失败立即熔断阻断升级。
- 🛡️ **声明式保护级别 (5-Level Protection Guard)**：
  - `db`：强数据门禁，自动快照，严格健康探针校验；
  - `assisted`：常规服务更新，自动改写 Tag，健康探针校验；
  - `auto`：自动化组，可由 cron/CI 安全全自动批处理升级；
  - `locked`：锁定通道，禁止升级；
  - `frozen`：架构与数据强绑定，严禁自动动刀。
- 🏷️ **Docker Labels 声明式策略**：零外部配置文件即可开箱即用，直接在 `docker-compose.yml` 中声明 `ops.guard="db"`、`ops.probe.http="http://:8080/health"`。
- 🔄 **高精度探针与事务级自动回滚 (Zero-Downtime Probe & Auto Rollback)**：升级后执行毫秒级 TCP/HTTP 存活探测与稳定期门禁等待；若探针超时或容器异常退出，**自动触发原子回滚**，恢复原镜像并自动还原 Compose YAML 文件。
- 🚀 **原生 Registry 秒级探测**：集成 GGCR，直接通过宿主并发网络 IO 探测 Docker Hub、GHCR 与私有 Harbor，自动继承宿主凭据助手（如 `docker-credential-pass`）。
- 📝 **全量升级留痕与多渠道通知**：执行批量升级自动归档 Markdown 报告至 `/var/lib/ops/reports/`，并支持飞书、钉钉、Telegram、Slack 与 SMTP 邮件多渠道广播。

---

## 🚀 快速安装

### 方式 1: 一键安装脚本 (Linux & macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/ConstasJ/ops/main/install.sh | sh
```

### 方式 2: 下载纯静态二进制

前往 [GitHub Releases](https://github.com/ConstasJ/ops/releases) 下载对应平台与架构的预编译产物：
- `ops-linux-amd64` (标准 x86_64 Linux 服务器)
- `ops-linux-arm64` (ARM64 云服务器 / 树莓派)
- `ops-windows-amd64.exe` (Windows Server)
- `ops-darwin-arm64` (Apple Silicon Mac)

```bash
# 以 Linux amd64 为例
sudo curl -Lo /usr/local/bin/ops https://github.com/ConstasJ/ops/releases/latest/download/ops-linux-amd64
sudo chmod +x /usr/local/bin/ops
```

### 方式 3: Docker Sidecar 容器模式

如果倾向于容器化运行，可直接挂载 Docker Socket 作为 Sidecar：

```bash
docker run --rm -it \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/ops:/var/lib/ops \
  ghcr.io/constasj/ops:latest status
```

---

## 📖 核心命令速查

| 命令 | 功能说明 | 典型场景 |
|---|---|---|
| `ops status` | 查看全机容器状态、健康指标与重启计数 | 日常快速巡检 |
| `ops ls` | 列出所有受控容器、类型、保护级别与注意事项 | 资产审计与架构梳理 |
| `ops doctor` | 全面体检：检测异常退出、探针失败与快照新鲜度 | 故障诊断与日常巡视 |
| `ops pending` | 并发探测上游 Registry，输出待更新版本对比表 | 升级前版本扫描 |
| `ops update <target>` | 对单个服务执行前置快照、Tag 改写与隔离重建 | 单点受控安全更新 |
| `ops upgrade` | 批量升级全机（或自动组）可用新版本并生成留痕报告 | 周期性批量维护 |
| `ops rollback <target>` | 依据 `state.json` 将服务原子回退到上一稳定版本 | 升级翻车紧急自愈 |
| `ops dump <target>` | 手工触发单个容器的原生流式快照备份 | 重大变更前手工快照 |

---

## 🏷️ Docker Compose 声明式标签规范

你可以在业务项目的 `docker-compose.yml` 中直接声明 `ops` 保护策略，无需修改中央配置：

```yaml
version: '3.8'

services:
  app:
    image: myorg/api-server:1.2.3
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
      - "ops.notes=核心生产数据库，升级前必须预演"
```

---

## ⚙️ 配置文件说明 (`containers.json`)

除了 Docker Labels，`ops` 同样支持中央知识库 `containers.json`（默认位于 `/etc/ops/containers.json` 或 `/opt/ops/containers.json`）：

```json
{
  "_comment": "ops 容器知识库配置",
  "linknet-db-1": {
    "kind": "postgres",
    "guard": "db",
    "probe": {
      "tcp": ":5432"
    },
    "timeout": 180,
    "settle": 15,
    "notes": "生产 PostgreSQL 主库"
  },
  "suwayomi": {
    "guard": "auto",
    "timeout": 60
  }
}
```

> **配置优先级**：Docker Labels > `containers.json` > 启发式推断。

---

## 📢 多渠道通知配置

在运行 `ops upgrade --notify` 时，可通过环境变量接入主流告警通道：

```bash
# 飞书 Webhook
export OPS_FEISHU_WEBHOOK="https://open.feishu.cn/open-apis/bot/v2/hook/xxxx"

# 钉钉 Webhook
export OPS_DINGTALK_WEBHOOK="https://oapi.dingtalk.com/robot/send?access_token=xxxx"

# Telegram Bot
export OPS_TELEGRAM_BOT_TOKEN="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
export OPS_TELEGRAM_CHAT_ID="-100123456789"

# SMTP 邮件
export SMTP_HOST="smtp.example.com"
export SMTP_PORT="587"
export SMTP_USER="alert@example.com"
export SMTP_PASS="secret"
export SMTP_TO="admin@example.com"
```

---

## 📄 开源许可证

本项目基于 [MIT 许可证](LICENSE) 开源。
