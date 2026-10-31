FROM node:26-bookworm@sha256:2aaae6d91f99fee84cfc92da9b52c22a185752d247746052bbc3f961e44478c6 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/reforge ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -o /out/reforge-migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -o /out/reforge-evidence ./cmd/evidence

FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
ARG BASE_REFRESH
RUN : "${BASE_REFRESH}" \
 && apk upgrade --no-cache \
 && apk add --no-cache ca-certificates \
 && adduser -D -u 10001 reforge \
 && mkdir -p /app/var/artifacts \
 && chown -R reforge:reforge /app/var \
 && chmod 0700 /app/var /app/var/artifacts
WORKDIR /app
COPY --from=gobuild /out/reforge /out/reforge-migrate /out/reforge-evidence /app/
COPY --from=web /src/web/dist /app/web/dist
COPY pkg/store/migrations /app/pkg/store/migrations
COPY third_party/third-party-notices.txt /usr/share/doc/reforge/third-party-notices.txt
USER reforge
ENV REFORGE_WEB_DIR=/app/web/dist \
    REFORGE_MIGRATION_DIR=/app/pkg/store/migrations \
    REFORGE_ARTIFACT_DIRECTORY=/app/var/artifacts
EXPOSE 8080
CMD ["/app/reforge"]
