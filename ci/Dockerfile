# Version matches go.mod; the Linux binary is built against an older glibc.
FROM golang:1.25.5-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test -race ./... && go build -o /out/tts2mic-mcp ./cmd/tts2mic-mcp

# Keep this version in sync with e2e/package{,-lock}.json.
FROM mcr.microsoft.com/playwright:v1.58.2-noble
USER root
RUN apt-get update && apt-get install -y --no-install-recommends pulseaudio pulseaudio-utils \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /work
COPY --chown=pwuser:pwuser e2e/package*.json ./e2e/
RUN npm --prefix e2e ci --ignore-scripts --no-audit --no-fund
COPY --from=build /out/tts2mic-mcp /usr/local/bin/tts2mic-mcp
COPY --chown=pwuser:pwuser ci ./ci
COPY --chown=pwuser:pwuser e2e ./e2e
RUN mkdir -p /work/artifacts && chown -R pwuser:pwuser /work
USER pwuser
ENV TTS2MIC_BIN=/usr/local/bin/tts2mic-mcp TTS_PROVIDER=stub
ENTRYPOINT ["bash", "ci/with-audio.sh"]
