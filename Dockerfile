# syntax=docker/dockerfile:1.7

#############################
# Stage 1: Builder
#############################
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# buildx 会注入 TARGETOS/TARGETARCH，用于构建目标架构二进制。
ARG TARGETOS
ARG TARGETARCH

# 构建所需基础工具（git 用于拉取可能的私有依赖）
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# 先复制依赖描述文件，利用 Docker layer cache
COPY go.mod go.sum ./
RUN go mod download

# 再复制业务代码
COPY . .

# 按目标平台编译静态二进制（当前部署目标为 linux/amd64）。
ENV CGO_ENABLED=0
RUN GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/orbitterm-server ./main.go

#############################
# Stage 2: Runtime
#############################
FROM alpine:latest

# 仅保留必要运行时依赖：CA 证书（HTTPS/邮件等 TLS 连接）
RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app

# 关键：声明镜像源码仓库，GHCR 会据此自动关联到 GitHub Repository 的 Packages。
LABEL org.opencontainers.image.source="https://github.com/bighard-1/OrbitTerm-Master"
LABEL org.opencontainers.image.description="OrbitTerm backend server image"

WORKDIR /app

# 只拷贝编译产物，保持镜像最小化
COPY --from=builder /out/orbitterm-server /app/orbitterm-server

USER app

EXPOSE 8080

# 服务启动命令
ENTRYPOINT ["/app/orbitterm-server"]
