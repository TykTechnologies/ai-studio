FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    libsqlite3-0 \
    poppler-utils \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy templates directory
COPY templates ./templates

# Copy docs_links.json file
COPY config/docs_links.json ./config/docs_links.json

# Set up edition and architecture-specific binary selection
ARG EDITION=ce
ARG TARGETARCH
# Copy pre-built binary (static files are embedded)
# Binary naming: tyk-ai-studio-{edition}-{arch} (e.g., tyk-ai-studio-ce-amd64)
COPY tyk-ai-studio-${EDITION}-${TARGETARCH} ./tyk-ai-studio

# Expose the required ports
EXPOSE 8080 9090

# Run the binary directly
ENTRYPOINT ["./tyk-ai-studio"]
