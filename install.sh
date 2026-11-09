#!/bin/sh
set -e

# ops 安装脚本 (Linux & macOS)
# 用法: curl -fsSL https://raw.githubusercontent.com/ConstasJ/ops/main/install.sh | sh

REPO="ConstasJ/ops"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="ops"

# 颜色控制
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[SUCCESS]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
    exit 1
}

# 1. 探测系统与架构
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux)
        TARGET_OS="linux"
        ;;
    Darwin)
        TARGET_OS="darwin"
        ;;
    *)
        error "不支持的操作系统: $OS (仅支持 Linux 与 macOS)"
        ;;
esac

case "$ARCH" in
    x86_64|amd64)
        TARGET_ARCH="amd64"
        ;;
    aarch64|arm64)
        TARGET_ARCH="arm64"
        ;;
    *)
        error "不支持的系统架构: $ARCH (仅支持 amd64 与 arm64)"
        ;;
esac

info "检测到系统平台: ${TARGET_OS}/${TARGET_ARCH}"

# 2. 检查权限
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        error "安装需要 root 权限，且系统中未找到 sudo 命令"
    fi
fi

# 3. 确定版本
VERSION="${OPS_VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
    info "正在查询最新版本..."
    LATEST_TAG=$(curl -fsSL -H "Accept: application/vnd.github.v3+json" "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)
    if [ -n "$LATEST_TAG" ]; then
        VERSION="$LATEST_TAG"
    else
        VERSION="v1.0.0"
    fi
fi
info "拟安装版本: ${VERSION}"

# 4. 下载目标二进制
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/ops_${VERSION#v}_${TARGET_OS}_${TARGET_ARCH}.tar.gz"
# 备用直连二进制命名规范
FALLBACK_URL="https://github.com/${REPO}/releases/download/${VERSION}/ops-${TARGET_OS}-${TARGET_ARCH}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

info "正在下载 ${BINARY_NAME}..."
if curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/ops.tar.gz" 2>/dev/null; then
    tar -xzf "$TMP_DIR/ops.tar.gz" -C "$TMP_DIR"
    BIN_SRC="$TMP_DIR/$BINARY_NAME"
elif curl -fsSL "$FALLBACK_URL" -o "$TMP_DIR/$BINARY_NAME" 2>/dev/null; then
    BIN_SRC="$TMP_DIR/$BINARY_NAME"
else
    error "下载失败，请确认发布资产中存在 ${DOWNLOAD_URL}"
fi

# 5. 安装到目标目录
info "正在安装到 ${INSTALL_DIR}/${BINARY_NAME}..."
chmod +x "$BIN_SRC"
$SUDO mkdir -p "$INSTALL_DIR"
$SUDO install -m 755 "$BIN_SRC" "$INSTALL_DIR/$BINARY_NAME"

# 6. 验证运行
if command -v "$BINARY_NAME" >/dev/null 2>&1; then
    success "${BINARY_NAME} 安装完成！"
    "$BINARY_NAME" version || true
else
    error "安装完成但在 PATH 中未找到 ${BINARY_NAME}，请检查 ${INSTALL_DIR} 是否在系统的 PATH 中"
fi
