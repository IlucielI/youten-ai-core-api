FROM mcr.microsoft.com/playwright:v1.50.0-noble

# Install pulse audio and ffmpeg for audio sink and stream capture
RUN apt-get update && apt-get install -y --no-install-recommends \
    pulseaudio \
    pulseaudio-utils \
    ffmpeg \
    xvfb \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package*.json ./
RUN npm ci --omit=dev

COPY src/ ./src/

# Create non-root user directory for PulseAudio
ENV HOME=/home/pwuser
RUN mkdir -p /home/pwuser/.config/pulse && chown -R pwuser:pwuser /home/pwuser /app

USER pwuser

# Healthcheck
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD node -e "process.exit(0)"

ENTRYPOINT ["node", "src/index.js"]
