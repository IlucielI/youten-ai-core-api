# Google Meet Bot Runner Daemon

A standalone container daemon powered by **Playwright (Chromium)**, **PulseAudio**, and **FFmpeg** responsible for executing virtual meeting attendance and recording on Google Meet, triggering the end-to-end automated meeting summarization pipeline in `ai-core-api`.

---

## 1. Architecture & Responsibilities

1. **Queue Consumer**: Listens on RabbitMQ queue `bot.google_meet.dispatch_queue` (bound to exchange `amq.topic` with routing key `bot.google_meet.dispatch`).
2. **Browser Automation**: Launches headless Chromium via Playwright, navigates to the meeting room URL, and handles entry modals.
3. **Audio & Media Safety**: Mutes the microphone and turns off the camera by default before requesting to join.
4. **Virtual Audio Capture**: Records the incoming WebRTC audio stream from the virtual PulseAudio sink to high-fidelity mono audio (`WAV 16kHz 16-bit PCM`) using FFmpeg.
5. **Real-time Lifecycle Webhooks**: Dispatches status updates to `ai-core-api` with shared-secret signature headers:
   - `waiting_admit`: Bot is at the lobby / waiting to be admitted by the host.
   - `joined`: Bot has entered the call.
   - `recording`: FFmpeg audio capture is actively streaming.
   - `completed`: Meeting concluded or stop requested; audio uploaded to S3.
   - `failed`: Entry denied by host, timeout waiting in lobby, or unrecoverable error.
6. **Object Storage Upload**: Uploads the final audio file directly to AWS S3 / MinIO and triggers asynchronous STT, speaker diarization, and LLM summarization.

---

## 2. Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | Connection string for RabbitMQ message broker |
| `API_ENDPOINT` | `http://localhost:8080` | Base URL of `ai-core-api` for sending webhook callbacks |
| `GOOGLE_MEET_BOT_WEBHOOK_SECRET` | *(empty)* | Shared secret signature passed via `X-Bot-Webhook-Secret` header |
| `S3_ENDPOINT` | `http://localhost:9000` | Endpoint URL for AWS S3 or MinIO storage |
| `S3_BUCKET_NAME` | `code-base-golang` | Target bucket name where recording audio is stored |
| `S3_ACCESS_KEY` | `minioadmin` | S3 / MinIO access key ID |
| `S3_SECRET_KEY` | `minioadmin` | S3 / MinIO secret access key |
| `GOOGLE_MEET_BOT_NAME` | `AI Meeting Assistant` | Display name of the guest bot requesting entry |
| `GOOGLE_MEET_ADMIT_TIMEOUT_SECONDS` | `600` | Maximum seconds to wait for host admission before timing out (default: 10 mins) |
| `GOOGLE_MEET_MAX_DURATION_SECONDS` | `7200` | Maximum allowed duration for a single recording session (default: 2 hours) |
| `GOOGLE_MEET_EMPTY_ROOM_TIMEOUT_SECONDS` | `180` | Automatic exit delay when the bot is the sole participant remaining (default: 3 mins) |
| `RUNNER_CONCURRENCY` | `2` | Maximum concurrent meeting sessions processed per runner instance |

---

## 3. Running with Docker Compose

The runner service is configured under the `bot-runner` profile in `deployment/docker-compose.yaml`:

```bash
# Start all dependencies plus the bot runner
docker compose --profile bot-runner up -d

# Check runner logs
docker compose logs -f meet-runner
```

---

## 4. Running Standalone

### Local Development
```bash
cd deployment/bot-runner

# Install dependencies
npm install

# Start in development mode with auto-reload
npm run dev
```

### Standalone Docker Container
```bash
# Build Docker image
docker build -t google-meet-bot-runner:latest deployment/bot-runner

# Run container connected to the application network
docker run -d \
  --name google-meet-bot-runner \
  --network code-base-golang_default \
  -e RABBITMQ_URL="amqp://guest:guest@rabbitmq:5672/" \
  -e API_ENDPOINT="http://api:8080" \
  -e S3_ENDPOINT="http://minio:9000" \
  -e S3_BUCKET_NAME="code-base-golang" \
  -e GOOGLE_MEET_BOT_WEBHOOK_SECRET="your-secure-webhook-secret" \
  google-meet-bot-runner:latest
```

---

## 5. Lifecycle & Fault Tolerance

- **Host Denies Admission**: The runner immediately detects the `"You can't join this call"` notification, cleanly exits the browser, and sends `failed` with an explicit reason (`admission denied by host`).
- **Lobby Timeout**: If the host neither admits nor denies the bot within `GOOGLE_MEET_ADMIT_TIMEOUT_SECONDS`, the session fails gracefully to prevent hanging compute resources.
- **Empty Room Auto-Exit**: When all human participants leave the meeting for more than `GOOGLE_MEET_EMPTY_ROOM_TIMEOUT_SECONDS`, the bot concludes recording, uploads the captured audio, and notifies the API as `completed`.
- **Graceful Termination**: Responds to `SIGTERM` / `SIGINT` signals by cleanly finalizing in-flight audio recordings and closing active browser contexts before terminating.
