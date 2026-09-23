FROM python:3.14-slim@sha256:caaf356f40667c496d405780745b9ac25771c189a51dfcc42430d531ea09f8a2 AS build
WORKDIR /src
COPY mkdocs.yml ./
COPY content ./content
RUN pip install --no-cache-dir "mkdocs-material==9.7.0" \
 && mkdocs build --strict --site-dir /site

FROM nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10
COPY --from=build /site /opt/docs-site
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh
ENV DOCS_VERSION=dev
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
