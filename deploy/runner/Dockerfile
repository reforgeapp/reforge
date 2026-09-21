FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/reforge-runner ./cmd/runner

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git \
 && adduser -D -u 10002 runner
WORKDIR /app
COPY --from=build /out/reforge-runner /app/reforge-runner
USER runner
ENTRYPOINT ["/app/reforge-runner"]
