#!/usr/bin/env zsh

set -euo pipefail

# deploy_local.sh
# 功能：
# 1) 先执行 Go 编译校验，确保代码可构建；
# 2) 使用 Docker Buildx 构建 ARM64 镜像；
# 3) 打上 ghcr.io/<用户名>/orbitterm-server:latest 标签；
# 4) 交互式询问是否 push 到 GHCR。

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

if ! command -v go >/dev/null 2>&1; then
  echo "[错误] 未检测到 Go，请先安装 Go。"
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "[错误] 未检测到 Docker，请先安装并启动 Docker Desktop。"
  exit 1
fi

echo "[1/4] 执行 Go 编译校验..."
go build ./...
echo "[完成] Go 编译校验通过。"

DEFAULT_GHCR_USER="your-github-username"
GHCR_USER="${GHCR_USERNAME:-$DEFAULT_GHCR_USER}"

if [[ "$GHCR_USER" == "$DEFAULT_GHCR_USER" ]]; then
  echo -n "请输入 GitHub 用户名（用于镜像标签 ghcr.io/<用户名>/...）: "
  read GHCR_INPUT
  GHCR_INPUT="${GHCR_INPUT// /}"
  if [[ -n "$GHCR_INPUT" ]]; then
    GHCR_USER="$GHCR_INPUT"
  fi
fi

IMAGE_TAG="ghcr.io/${GHCR_USER}/orbitterm-server:latest"

echo "[2/4] 构建 Docker 镜像（linux/arm64）..."
# --load: 将 buildx 构建结果加载到本地 Docker images
# 若你后续希望直接推送多架构镜像，可改为 --push 并配合多平台参数

docker buildx build \
  --platform linux/arm64 \
  --tag "$IMAGE_TAG" \
  --load \
  .

echo "[3/4] 镜像构建完成：$IMAGE_TAG"

echo -n "[4/4] 是否执行 docker push 到 GitHub Container Registry? (y/N): "
read PUSH_ANSWER
PUSH_ANSWER="${PUSH_ANSWER:l}"

if [[ "$PUSH_ANSWER" == "y" || "$PUSH_ANSWER" == "yes" ]]; then
  echo "开始推送镜像：$IMAGE_TAG"
  echo "提示：若未登录，请先执行 docker login ghcr.io"
  docker push "$IMAGE_TAG"
  echo "镜像推送完成。"
else
  echo "已跳过 push。"
fi
