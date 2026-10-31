FROM python:3.14-slim@sha256:caaf356f40667c496d405780745b9ac25771c189a51dfcc42430d531ea09f8a2 AS build
WORKDIR /src
COPY requirements.lock ./
COPY mkdocs.yml ./
COPY content ./content
RUN case "$(uname -m)" in x86_64|aarch64) ;; *) printf "unsupported docs build architecture: %s\n" "$(uname -m)" >&2; exit 1 ;; esac \
 && head -n 2 requirements.lock > /tmp/python-build-tools.lock \
 && pip install --no-cache-dir --require-hashes --no-deps -r /tmp/python-build-tools.lock \
 && pip install --no-cache-dir --require-hashes --no-build-isolation -r requirements.lock \
 && mkdocs build --strict --site-dir /site

FROM nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10
COPY --from=build /site /opt/docs-site
COPY nginx.conf /etc/nginx/nginx.conf
COPY entrypoint.sh /entrypoint.sh
ARG BASE_REFRESH
RUN : "${BASE_REFRESH}" && apk upgrade --no-cache && rm /etc/nginx/conf.d/default.conf && chmod 0755 /entrypoint.sh
ENV DOCS_VERSION=dev
EXPOSE 8080
USER 101:101
ENTRYPOINT ["/entrypoint.sh"]
