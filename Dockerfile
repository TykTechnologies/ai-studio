FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache poppler-utils

# Copy pre-built binaries and assets
COPY midsommar-amd64 midsommar-arm64 ./
COPY docker-entrypoint.sh /usr/local/bin/
COPY templates ./templates
COPY ui/admin-frontend/build ./ui/admin-frontend/build
COPY docs/site/public ./docs/site/public

# Make entrypoint executable
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Expose the required ports
EXPOSE 8080 9090

# Run the appropriate binary based on architecture
ENTRYPOINT ["docker-entrypoint.sh"]
