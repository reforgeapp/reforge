FROM node:26-bookworm AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.27-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/reforge ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -o /out/reforge-migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -o /out/reforge-evidence ./cmd/evidence

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 reforge \
 && mkdir -p /app/var/artifacts \
 && chown -R reforge:reforge /app/var
WORKDIR /app
COPY --from=gobuild /out/reforge /out/reforge-migrate /out/reforge-evidence /app/
COPY --from=web /src/web/dist /app/web/dist
COPY internal/store/migrations /app/internal/store/migrations
USER reforge
ENV REFORGE_WEB_DIR=/app/web/dist \
    REFORGE_MIGRATION_DIR=/app/internal/store/migrations \
    REFORGE_ARTIFACT_DIRECTORY=/app/var/artifacts
EXPOSE 8080
CMD ["/app/reforge"]
