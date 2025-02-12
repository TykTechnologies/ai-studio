FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy pre-built binaries and assets
COPY midsommar-amd64 midsommar-arm64 ./
COPY docker-entrypoint.sh ./
COPY templates ./templates
COPY ui/admin-frontend/build ./ui/admin-frontend/build
COPY docs/site/public ./docs/site/public

# Expose the required ports
EXPOSE 8080 9090

# Run the appropriate binary based on architecture
ENTRYPOINT ["./docker-entrypoint.sh"]
