FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/reforge-runner ./cmd/runner \
 && CGO_ENABLED=0 go build -trimpath -o /out/reforge-sandbox-tool ./cmd/sandbox-tool

FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS images
ARG GVISOR_RELEASE=20260914.0
ARG GVISOR_SHA512=ff1c0577c8daa0e3511aedad554c3d5e20fa184b4ddd422454f573d60a8b9774f972755ca5c7f56588c5b4f42345b60b2ead077f35d7d3235e23f57c0fa194ca
RUN apt-get update \
 && apt-get install -y --no-install-recommends binutils bzip2 python3 \
 && rm -rf /var/lib/apt/lists/*
COPY --from=node:26-bookworm@sha256:2aaae6d91f99fee84cfc92da9b52c22a185752d247746052bbc3f961e44478c6 /usr/local/bin/node /usr/local/bin/node
COPY --from=node:26-bookworm@sha256:2aaae6d91f99fee84cfc92da9b52c22a185752d247746052bbc3f961e44478c6 /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
COPY scripts/build-runner-images.py /src/build-runner-images.py
RUN for stack in go javascript python; do python3 /src/build-runner-images.py --stack "$stack" --output "/out/images/$stack"; done
RUN curl -fsSLo /tmp/gvisor.tar.bz2 "https://storage.googleapis.com/gvisor/releases/release/${GVISOR_RELEASE}/x86_64/gvisor.tar.bz2" \
 && echo "${GVISOR_SHA512}  /tmp/gvisor.tar.bz2" | sha512sum -c - \
 && mkdir -p /out/gvisor \
 && tar -xjf /tmp/gvisor.tar.bz2 -C /out/gvisor runsc gvisor-bin \
 && chmod -R go-w /out/gvisor

FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
RUN apk upgrade --no-cache \
 && apk add --no-cache ca-certificates git \
 && adduser -D -u 10002 runner \
 && mkdir -m 0750 /builtin \
 && chown 10002:10001 /builtin
WORKDIR /app
COPY --from=build /out/reforge-runner /out/reforge-sandbox-tool /app/
COPY --from=images /out/images /app/images
COPY --from=images /out/gvisor /app/gvisor
COPY third_party/third-party-notices.txt /usr/share/doc/reforge/third-party-notices.txt
USER runner
ENTRYPOINT ["/app/reforge-runner"]
