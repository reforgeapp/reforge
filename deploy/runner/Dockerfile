FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/reforge-runner ./cmd/runner \
 && CGO_ENABLED=0 go build -trimpath -o /out/reforge-sandbox-tool ./cmd/sandbox-tool

FROM node:26-bookworm@sha256:2aaae6d91f99fee84cfc92da9b52c22a185752d247746052bbc3f961e44478c6 AS npm
RUN cd /usr/local/lib/node_modules/npm/node_modules \
 && for package in brace-expansion@5.0.11 undici@6.28.1; do \
      name="${package%@*}" && file="$(npm pack --silent --pack-destination /tmp "$package")" \
      && rm -rf "$name" && mkdir "$name" && tar -xzf "/tmp/$file" -C "$name" --strip-components=1; \
    done

FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS images
ARG GVISOR_RELEASE=20260914.0
ARG GVISOR_SHA512=ff1c0577c8daa0e3511aedad554c3d5e20fa184b4ddd422454f573d60a8b9774f972755ca5c7f56588c5b4f42345b60b2ead077f35d7d3235e23f57c0fa194ca
RUN apt-get update \
 && apt-get install -y --no-install-recommends binutils bzip2 coreutils findutils grep sed python3 \
 && rm -rf /var/lib/apt/lists/*
COPY --from=node:26-bookworm@sha256:2aaae6d91f99fee84cfc92da9b52c22a185752d247746052bbc3f961e44478c6 /usr/local/bin/node /usr/local/bin/node
COPY --from=npm /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
COPY scripts/build-runner-images.py /src/build-runner-images.py
RUN mkdir -p /out/toolchain-licenses \
 && cp /usr/share/doc/coreutils/copyright /out/toolchain-licenses/coreutils.copyright \
 && cp /usr/share/doc/findutils/copyright /out/toolchain-licenses/findutils.copyright \
 && cp /usr/share/doc/grep/copyright /out/toolchain-licenses/grep.copyright \
 && cp /usr/share/doc/sed/copyright /out/toolchain-licenses/sed.copyright \
 && cp /usr/share/doc/libc6/copyright /out/toolchain-licenses/libc6.copyright \
 && cp /usr/share/common-licenses/LGPL-2.1 /out/toolchain-licenses/LGPL-2.1 \
 && cp /usr/share/common-licenses/GPL-3 /out/toolchain-licenses/GPL-3
RUN for stack in go javascript python; do python3 /src/build-runner-images.py --stack "$stack" --output "/out/images/$stack"; done
RUN curl -fsSLo /tmp/gvisor.tar.bz2 "https://storage.googleapis.com/gvisor/releases/release/${GVISOR_RELEASE}/x86_64/gvisor.tar.bz2" \
 && echo "${GVISOR_SHA512}  /tmp/gvisor.tar.bz2" | sha512sum -c - \
 && mkdir -p /out/gvisor \
 && tar -xjf /tmp/gvisor.tar.bz2 -C /out/gvisor runsc gvisor-bin \
 && chmod -R go-w /out/gvisor

FROM scratch AS workspace-go
COPY --from=images /out/images/go/ /
COPY --from=build /out/reforge-sandbox-tool /opt/reforge/tool
COPY --from=images /out/toolchain-licenses /usr/share/doc/reforge/toolchain-licenses
COPY third_party/third-party-notices.txt /usr/share/doc/reforge/third-party-notices.txt
WORKDIR /workspace
USER 65532:65532
ENTRYPOINT ["/opt/reforge/tool", "hold"]

FROM scratch AS workspace-javascript
COPY --from=images /out/images/javascript/ /
COPY --from=build /out/reforge-sandbox-tool /opt/reforge/tool
COPY --from=images /out/toolchain-licenses /usr/share/doc/reforge/toolchain-licenses
COPY third_party/third-party-notices.txt /usr/share/doc/reforge/third-party-notices.txt
WORKDIR /workspace
USER 65532:65532
ENTRYPOINT ["/opt/reforge/tool", "hold"]

FROM scratch AS workspace-python
COPY --from=images /out/images/python/ /
COPY --from=build /out/reforge-sandbox-tool /opt/reforge/tool
COPY --from=images /out/toolchain-licenses /usr/share/doc/reforge/toolchain-licenses
COPY third_party/third-party-notices.txt /usr/share/doc/reforge/third-party-notices.txt
WORKDIR /workspace
USER 65532:65532
ENTRYPOINT ["/opt/reforge/tool", "hold"]


FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
ARG BASE_REFRESH
RUN : "${BASE_REFRESH}" \
 && apk upgrade --no-cache \
 && apk add --no-cache ca-certificates git \
 && adduser -D -u 10002 runner \
 && mkdir -m 0750 /builtin \
 && chown 10002:10001 /builtin
WORKDIR /app
COPY --from=build /out/reforge-runner /out/reforge-sandbox-tool /app/
COPY --from=images /out/images /app/images
COPY --from=images /out/gvisor /app/gvisor
COPY --from=images /out/toolchain-licenses /usr/share/doc/reforge/toolchain-licenses
COPY third_party/third-party-notices.txt /usr/share/doc/reforge/third-party-notices.txt
USER runner
ENTRYPOINT ["/app/reforge-runner"]
