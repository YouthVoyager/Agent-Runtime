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
    -ldflags "-s -w -X agent-runtime/internal/version.Version=${VERSION} -X agent-runtime/internal/version.Commit=${COMMIT} -X agent-runtime/internal/version.BuildTime=${BUILD_TIME}" \
    -o /out/service ./cmd/${SERVICE}

FROM alpine:3.22

RUN adduser -D -H -u 10001 appuser
USER appuser

COPY --from=build /out/service /service

EXPOSE 8080
ENTRYPOINT ["/service"]
