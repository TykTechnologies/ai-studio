FROM debian:bookworm-slim

RUN apt-get update && \
	apt-get install -y --no-install-recommends \
	ca-certificates \
	libsqlite3-0 \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Set up architecture-specific binary selection
ARG TARGETARCH
# Copy pre-built binary and assets
COPY midsommar-${TARGETARCH} ./midsommar
COPY templates ./templates
COPY ui/admin-frontend/build ./ui/admin-frontend/build
COPY docs/site/public ./docs/site/public

# Expose the required ports
EXPOSE 8080 9090

# Run the binary directly
ENTRYPOINT ["./midsommar"]
