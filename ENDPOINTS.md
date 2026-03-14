# LRScribe76 API Endpoints

All endpoints are served from the Express server (default port 5000). Authentication is via **Clerk** (JWT Bearer token) for most endpoints, and **Convex user ID** (`X-User-Id` header or `userId` in body) for audio endpoints.

---

## Table of Contents

1. [Auth](#auth)
2. [Transcriptions (Drizzle/Postgres)](#transcriptions)
3. [Document Generation](#document-generation)
4. [Audio Recording](#audio-recording)
5. [Transcription from Audio](#transcription-from-audio)
6. [Convex Queries & Mutations](#convex-queries--mutations)

---

## Auth

### GET /api/auth/user

Returns the authenticated user's info from Clerk.

**Auth:** Clerk JWT (Bearer token)

**Request:**
```
GET /api/auth/user
Authorization: Bearer <clerk_jwt_token>
```

**Response (200):**
```json
{
  "id": "user_2abc123",
  "email": null,
  "firstName": null,
  "lastName": null,
  "profileImageUrl": null,
  "createdAt": null,
  "updatedAt": null
}
```

**Response (401):**
```json
{ "message": "Unauthorized" }
```

---

## Transcriptions

### GET /api/transcriptions

List all transcriptions for the authenticated user.

**Auth:** Clerk JWT

**Request:**
```
GET /api/transcriptions
Authorization: Bearer <clerk_jwt_token>
```

**Response (200):**
```json
[
  {
    "id": 1,
    "userId": "user_2abc123",
    "title": "Patient Visit - March 2026",
    "audioUrl": "https://s3.amazonaws.com/bucket/audio123.webm",
    "content": "Transcribed text content...",
    "status": "completed",
    "createdAt": "2026-03-14T20:00:00.000Z"
  }
]
```

---

### POST /api/transcriptions

Create a new transcription record.

**Auth:** Clerk JWT

**Request:**
```
POST /api/transcriptions
Authorization: Bearer <clerk_jwt_token>
Content-Type: application/json

{
  "userId": "user_2abc123",
  "title": "Patient Visit - March 2026",
  "audioUrl": "https://s3.amazonaws.com/bucket/audio123.webm",
  "content": "Optional initial transcription text"
}
```

**Response (201):**
```json
{
  "id": 2,
  "userId": "user_2abc123",
  "title": "Patient Visit - March 2026",
  "audioUrl": "https://s3.amazonaws.com/bucket/audio123.webm",
  "content": "Optional initial transcription text",
  "status": "pending",
  "createdAt": "2026-03-14T21:00:00.000Z"
}
```

**Response (400 — validation error):**
```json
{
  "message": "Required",
  "field": "title"
}
```

---

### GET /api/transcriptions/:id

Get a specific transcription by ID (must belong to the authenticated user).

**Auth:** Clerk JWT

**Request:**
```
GET /api/transcriptions/1
Authorization: Bearer <clerk_jwt_token>
```

**Response (200):**
```json
{
  "id": 1,
  "userId": "user_2abc123",
  "title": "Patient Visit - March 2026",
  "audioUrl": "https://s3.amazonaws.com/bucket/audio123.webm",
  "content": "Transcribed text...",
  "status": "completed",
  "createdAt": "2026-03-14T20:00:00.000Z"
}
```

**Response (404):**
```json
{ "message": "Transcription not found" }
```

**Response (401):**
```json
{ "message": "Unauthorized" }
```

---

## Document Generation

### POST /api/generate-document

Generate a structured medical document from a transcription/notes using an LLM, based on template sections.

**Auth:** Clerk JWT

**Request:**
```
POST /api/generate-document
Authorization: Bearer <clerk_jwt_token>
Content-Type: application/json

{
  "transcription": "Doctor: Good morning. How are you feeling today?\nPatient: I've been having headaches for about a week...",
  "notes": "Patient reports persistent headaches. BP 130/85.",
  "patientName": "Jane Smith",
  "sessionTitle": "Follow-up Visit",
  "sessionId": "j57a8b9c0d1e2f3g4h5i6j7k",
  "model": "openai-responses/gpt-5.4",
  "templateSections": [
    {
      "name": "Chief Complaint",
      "description": "Primary reason for the visit",
      "order": 0,
      "examples": ["Patient presents with..."],
      "adhereToFormatting": false,
      "allowAssessment": false
    },
    {
      "name": "Vital Signs",
      "description": "Recorded vital signs",
      "order": 1,
      "adhereToFormatting": true,
      "formatTemplate": "BP: {{blood_pressure}}\nHR: {{heart_rate}}\nTemp: {{temperature}}",
      "allowAssessment": false
    },
    {
      "name": "Assessment",
      "description": "Clinical assessment and diagnosis",
      "order": 2,
      "examples": [],
      "adhereToFormatting": false,
      "allowAssessment": true
    }
  ]
}
```

**Response (200):**
```json
{
  "document": "## Chief Complaint\n\nPatient presents with persistent headaches lasting one week.\n\n## Vital Signs\n\nBP: 130/85\nHR: Not documented\nTemp: Not documented\n\n## Assessment\n\nPatient experiencing tension-type headaches...",
  "documentGeneratedAt": 1710450000000,
  "modelUsed": "openai-responses/gpt-5.4"
}
```

**Response (400):**
```json
{ "error": "At least one of transcription or notes, plus template sections, are required" }
```

**Response (400):**
```json
{ "error": "sessionId is required" }
```

---

### POST /api/regenerate-section

Regenerate a single section of a previously generated document.

**Auth:** Clerk JWT

**Request:**
```
POST /api/regenerate-section
Authorization: Bearer <clerk_jwt_token>
Content-Type: application/json

{
  "transcription": "Doctor: Good morning...",
  "notes": "Patient reports persistent headaches.",
  "patientName": "Jane Smith",
  "sessionTitle": "Follow-up Visit",
  "sessionId": "j57a8b9c0d1e2f3g4h5i6j7k",
  "model": "openai-responses/gpt-5.4",
  "section": {
    "name": "Chief Complaint",
    "description": "Primary reason for the visit",
    "examples": ["Patient presents with..."],
    "adhereToFormatting": false,
    "allowAssessment": false
  }
}
```

**Response (200):**
```json
{
  "content": "Patient presents with a one-week history of persistent headaches, primarily frontal in location."
}
```

**Response (400):**
```json
{ "error": "Source content and section are required" }
```

---

## Audio Recording

All audio endpoints use **Convex authentication** (`X-User-Id` header or `userId` in body). These proxy to an external Audio API service.

### POST /api/audio/start

Start a new audio recording session.

**Auth:** Convex User ID

**Request:**
```
POST /api/audio/start
X-User-Id: j57a8b9c0d1e2f3g4h5i6j7k
Content-Type: application/json

{
  "sessionId": "j57a8b9c0d1e2f3g4h5i6j7k",
  "userId": "j57a8b9c0d1e2f3g4h5i6j7k",
  "mimeType": "audio/webm",
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "metadata": { "source": "browser" }
}
```

*Note: `id` is optional (client-supplied UUID). If omitted, the Audio API generates one.*

**Response (201):**
```json
{
  "recordingId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "recording"
}
```

**Response (400):**
```json
{ "error": "sessionId, userId, and mimeType are required" }
```

**Response (401):**
```json
{ "error": "Not authenticated: no userId provided" }
```

---

### POST /api/audio/chunk/:recordingId

Upload an audio chunk for an active recording.

**Auth:** Convex User ID

**Request:**
```
POST /api/audio/chunk/550e8400-e29b-41d4-a716-446655440000
X-User-Id: j57a8b9c0d1e2f3g4h5i6j7k
Content-Type: application/octet-stream
X-Chunk-Index: 0

<binary audio data>
```

**Response (200):**
```json
{
  "chunkIndex": 0,
  "received": true
}
```

**Response (400):**
```json
{ "error": "X-Chunk-Index header is required" }
```

---

### POST /api/audio/trigger-interim/:recordingId

Complete the current recording segment, start a new one, and trigger background transcription of the completed segment.

**Auth:** Convex User ID

**Request:**
```
POST /api/audio/trigger-interim/550e8400-e29b-41d4-a716-446655440000
X-User-Id: j57a8b9c0d1e2f3g4h5i6j7k
Content-Type: application/json

{
  "totalChunks": 5,
  "sessionId": "j57a8b9c0d1e2f3g4h5i6j7k",
  "mimeType": "audio/webm"
}
```

**Response (200):**
```json
{
  "newRecordingId": "660e8400-e29b-41d4-a716-446655440001"
}
```

*Note: Background transcription runs async — the completed segment's audio is transcribed via Gemini and appended to the session's transcription in Convex.*

**Response (400):**
```json
{ "error": "totalChunks and sessionId are required" }
```

---

### POST /api/audio/complete/:recordingId

Finalize a recording (all chunks uploaded).

**Auth:** Convex User ID

**Request:**
```
POST /api/audio/complete/550e8400-e29b-41d4-a716-446655440000
X-User-Id: j57a8b9c0d1e2f3g4h5i6j7k
Content-Type: application/json

{
  "totalChunks": 10
}
```

**Response (200):**
```json
{
  "status": "completed",
  "audioUrl": "https://s3.amazonaws.com/bucket/recordings/550e8400.webm"
}
```

**Response (400):**
```json
{ "error": "totalChunks is required" }
```

---

### GET /api/audio/status/:recordingId

Get the status of a recording.

**Auth:** Convex User ID

**Request:**
```
GET /api/audio/status/550e8400-e29b-41d4-a716-446655440000
X-User-Id: j57a8b9c0d1e2f3g4h5i6j7k
```

**Response (200):**
```json
{
  "recordingId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "audioUrl": "https://s3.amazonaws.com/bucket/recordings/550e8400.webm",
  "totalChunks": 10,
  "createdAt": "2026-03-14T20:00:00.000Z"
}
```

---

## Transcription from Audio

### POST /api/transcribe-from-url

Transcribe audio from a URL (fetches from Audio API by recording ID, or uses a direct URL). Uses Gemini for transcription.

**Auth:** Clerk JWT

**Request (by recording ID — preferred):**
```
POST /api/transcribe-from-url
Authorization: Bearer <clerk_jwt_token>
Content-Type: application/json

{
  "audioApiRecordingId": "550e8400-e29b-41d4-a716-446655440000",
  "mimeType": "audio/webm"
}
```

**Request (by direct URL — fallback):**
```
POST /api/transcribe-from-url
Authorization: Bearer <clerk_jwt_token>
Content-Type: application/json

{
  "audioUrl": "https://s3.amazonaws.com/bucket/audio.webm",
  "mimeType": "audio/webm"
}
```

**Response (200):**
```json
{
  "transcription": "Good morning. How are you feeling today? I've been having headaches for about a week now..."
}
```

**Response (400):**
```json
{ "error": "audioApiRecordingId or audioUrl is required" }
```

---

### POST /api/transcribe

Transcribe audio from base64-encoded data. Uses Gemini for transcription.

**Auth:** Clerk JWT

**Request:**
```
POST /api/transcribe
Authorization: Bearer <clerk_jwt_token>
Content-Type: application/json

{
  "audioData": "UklGRiQAAABXQVZFZm10IBAAAAABAAEA...",
  "mimeType": "audio/webm"
}
```

**Response (200):**
```json
{
  "transcription": "Good morning. How are you feeling today?..."
}
```

**Response (400):**
```json
{ "error": "Audio data is required" }
```

---

## Convex Queries & Mutations

These are not HTTP endpoints — they're Convex serverless functions called from the client via the Convex SDK. Listed here for completeness.

### Users

| Function | Type | Args | Description |
|----------|------|------|-------------|
| `users.getOrCreateUser` | mutation | `{ email, name?, profileImageUrl?, googleId }` | Find or create user by Google ID |
| `users.getCurrentUser` | query | `{ googleId }` | Get user by Google ID |
| `users.getUserById` | query | `{ id }` | Get user by Convex ID |

### Recording Sessions

| Function | Type | Args | Description |
|----------|------|------|-------------|
| `recordingSessions.list` | query | `{ userId }` | List all sessions for user (desc order) |
| `recordingSessions.get` | query | `{ id }` | Get session by ID |
| `recordingSessions.create` | mutation | `{ userId, patientName, title, recordingDate, audioUrl?, transcription?, duration?, notes?, templateId? }` | Create new session |
| `recordingSessions.update` | mutation | `{ id, userId, patientName?, title?, audioUrl?, audioData?, audioMimeType?, audioApiRecordingId?, audioApiTotalChunks?, transcription?, generatedDocument?, documentGeneratedAt?, documentGenerating?, modelUsed?, status?, duration?, notes?, templateId?, clearTemplateId?, clearAudioData? }` | Update session fields |
| `recordingSessions.remove` | mutation | `{ id, userId }` | Delete session |

### Templates

| Function | Type | Args | Description |
|----------|------|------|-------------|
| `templates.list` | query | `{ userId }` | List templates (sorted by order) |
| `templates.get` | query | `{ id }` | Get template by ID |
| `templates.create` | mutation | `{ userId, name, description? }` | Create template (auto-adds "Title" section) |
| `templates.update` | mutation | `{ id, userId, name?, description? }` | Update template |
| `templates.remove` | mutation | `{ id, userId }` | Delete template + all sections |
| `templates.clone` | mutation | `{ id, userId }` | Duplicate template with all sections |
| `templates.getSections` | query | `{ templateId }` | List sections for template |
| `templates.addSection` | mutation | `{ templateId, userId, name, description, examples?, adhereToFormatting?, formatTemplate?, doubleSpaceOutput?, allowAssessment? }` | Add section to template |
| `templates.updateSection` | mutation | `{ id, userId, name, description, examples, adhereToFormatting?, formatTemplate?, doubleSpaceOutput?, allowAssessment? }` | Update section |
| `templates.deleteSection` | mutation | `{ id, userId }` | Delete section |
| `templates.reorderSections` | mutation | `{ templateId, userId, sectionIds[] }` | Reorder sections |
| `templates.reorderTemplates` | mutation | `{ userId, templateIds[] }` | Reorder templates |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `5000` | Server port |
| `HOST` | `0.0.0.0` | Server bind address |
| `REQUESTY_API_KEY` | — | API key for Requesty (LLM router) |
| `REQUESTY_MODEL` | `openai-responses/gpt-5.4` | Default LLM model for doc generation |
| `GEMINI_TRANSCRIPTION_MODEL` | `google/gemini-3.1-pro-preview` | Model for audio transcription |
| `VITE_CONVEX_URL` | `https://robust-labrador-493.convex.cloud` | Convex deployment URL |
| `AUDIO_API_URL` | `https://lrscribe-audio-api-production.up.railway.app` | External Audio API base URL |
| `AUDIO_API_KEY` | — | Auth key for Audio API |
| `CLERK_SECRET_KEY` | — | Clerk authentication secret |
