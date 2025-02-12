FROM --platform=linux/arm64 gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy pre-built binary and assets
COPY midsommar-arm64 ./midsommar
COPY templates ./templates
COPY ui/admin-frontend/build ./ui/admin-frontend/build
COPY docs/site/public ./docs/site/public

# Expose the required ports
EXPOSE 8080 9090

# Run the binary directly
ENTRYPOINT ["./midsommar"]
