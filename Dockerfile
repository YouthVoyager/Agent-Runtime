FROM node:22-alpine AS web-build

WORKDIR /src/web-ui

COPY web-ui/package.json ./
COPY web-ui/index.html ./index.html
COPY web-ui/scripts ./scripts
COPY web-ui/src ./src

# 前端构建无外部 npm 依赖，生成 API Service 可直接托管的静态目录。
RUN npm run build

FROM golang:1.26-alpine AS build

ARG SERVICE=api-service
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X stableagent/pkg/version.Version=${VERSION} -X stableagent/pkg/version.Commit=${COMMIT} -X stableagent/pkg/version.BuildTime=${BUILD_TIME}" \
    -o /out/service ./cmd/${SERVICE}

FROM alpine:3.22 AS service-runtime

RUN adduser -D -H -u 10001 appuser
USER appuser

COPY --from=build /out/service /service

EXPOSE 8080
ENTRYPOINT ["/service"]

FROM service-runtime AS api-runtime

ENV API_SERVICE_WEB_STATIC_DIR=/web-ui/dist

COPY --from=web-build /src/web-ui/dist /web-ui/dist
