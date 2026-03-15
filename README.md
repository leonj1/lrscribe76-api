# lrscribe76-api

Go backend for [LRScribe76](https://lrscribe76-production.up.railway.app) — a medical scribe app that records audio, transcribes it, and generates structured clinical documents using LLMs.

Handles audio recording (proxied to a dedicated audio service), transcription via Gemini, document generation via Requesty/OpenAI, and CRUD for notes and transcriptions backed by Postgres.

## Quick Start

```bash
# Prerequisites: Go 1.21+, PostgreSQL

git clone https://github.com/leonj1/lrscribe76-api.git
cd lrscribe76-api

# Set required env vars (see below)
export DATABASE_URL="postgres://user:pass@localhost:5432/lrscribe76"
export CLERK_SECRET_KEY="sk_test_..."
export REQUESTY_API_KEY="rqsty-sk-..."
export AUDIO_API_URL="https://lrscribe-audio-api-production.up.railway.app"
export AUDIO_API_KEY="your-audio-api-key"

go run notes.go
# Server starts on :8080
```

## Railway Deployment

| Field | Value |
|-------|-------|
| **Project** | `lrscribe-audio-api` |
| **Service** | `lrscribe76-api` |
| **Environment** | `production` |
| **Public Domain** | `https://lrscribe76-api-production.up.railway.app` |
| **Private Domain** | `lrscribe76-api.railway.internal` |

### Sibling Services (same Railway project)

| Service | Domain | Description |
|---------|--------|-------------|
| `lrscribe76` | `lrscribe76-production.up.railway.app` | Next.js frontend + Express middleware |
| `lrscribe-audio-api` | `lrscribe-audio-api-production.up.railway.app` | Audio chunk storage & S3 management |

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `CLERK_SECRET_KEY` | Yes | — | Clerk authentication secret key |
| `REQUESTY_API_KEY` | Yes | — | API key for Requesty (LLM router) |
| `REQUESTY_MODEL` | No | `openai-responses/gpt-5.4` | LLM model for document generation |
| `AUDIO_API_URL` | Yes | — | Base URL of the audio recording service |
| `AUDIO_API_KEY` | Yes | — | Auth key for the audio service |
| `VITE_CONVEX_URL` | Yes | — | Convex deployment URL (for session/user data) |
| `PORT` | No | `8080` | Server listen port |

## API Endpoints

### Health
- `GET /health` — Service health check

### Auth
- `GET /api/auth/user` — Current user info (Clerk JWT)

### Document Generation
- `POST /api/generate-document` — Generate structured clinical document from transcription
- `POST /api/regenerate-section` — Regenerate a single document section

### Transcription
- `POST /api/transcribe` — Transcribe base64-encoded audio via Gemini
- `POST /api/transcribe-from-url` — Transcribe audio from URL or recording ID (Clerk JWT)

### Audio Recording (Convex auth)
- `POST /api/audio/start` — Start a recording session
- `POST /api/audio/chunk/:recordingId` — Upload audio chunk
- `GET /api/audio/status/:recordingId` — Recording status
- `POST /api/audio/complete/:recordingId` — Finalize recording
- `POST /api/audio/trigger-interim/:recordingId` — Trigger interim transcription, start new segment

### Notes & Transcriptions (requires Postgres)
- `GET /api/transcriptions` — List transcriptions
- `POST /api/transcriptions` — Create transcription
- `GET /api/transcriptions/:id` — Get transcription (Clerk JWT)

Full endpoint documentation with request/response examples: [ENDPOINTS.md](./ENDPOINTS.md)

## Tech Stack

- **Go 1.21** with [vestigo](https://github.com/husobee/vestigo) router
- **Clerk** for JWT authentication
- **PostgreSQL** (via `lib/pq`) for notes & transcriptions
- **Convex** for real-time session/user data
- **Requesty** as LLM router (OpenAI, Gemini models)
- **S3** for audio storage (via audio service)

## License

Private repository.
