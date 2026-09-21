FROM python:3.14-slim AS build
WORKDIR /src
COPY mkdocs.yml ./
COPY content ./content
RUN pip install --no-cache-dir "mkdocs-material==9.7.0" \
 && mkdocs build --strict --site-dir /site

FROM nginx:1.27-alpine
COPY --from=build /site /opt/docs-site
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh
ENV DOCS_VERSION=dev
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
