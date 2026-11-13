# syntax=docker/dockerfile:1.7

# 只构建 SPA，运行时由 Go 内嵌提供页面；不启动 Node SSR，
# 避免 CentOS 7 宿主机上 Node 24 / 新版 glibc 的兼容问题。
FROM node:24-alpine AS web-builder
WORKDIR /src/web

RUN corepack enable && corepack prepare pnpm@10.30.2 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build:spa

FROM golang:1.26-alpine AS server-builder
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src

RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	go mod download

COPY . ./
COPY --from=web-builder /src/web/build/spa ./web/build/spa

ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -trimpath -ldflags="-s -w" -o /out/bbs-go ./main.go

FROM alpine:3.20

ENV BBSGO_ENV=prod \
	TZ=Asia/Shanghai

RUN mkdir -p /app/data /app/logs /app/res/uploads /app/defaults /usr/share/zoneinfo/Asia
COPY deploy/centos7-sqlite/timezone/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai

WORKDIR /app

COPY --from=server-builder /out/bbs-go /app/bbs-go
COPY locales /app/locales
COPY res /app/res
COPY deploy/centos7-sqlite/config/bbs-go.yaml /app/defaults/bbs-go.yaml
COPY deploy/centos7-sqlite/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh \
	&& ln -sf /app/data/bbs-go.yaml /app/bbs-go.yaml

EXPOSE 8082
VOLUME ["/app/data", "/app/logs", "/app/res/uploads"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8082/api/install/status >/dev/null || exit 1

CMD ["/app/entrypoint.sh"]
