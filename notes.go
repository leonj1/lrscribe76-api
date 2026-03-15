package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"notes/models"
	"notes/routes"
	"os"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/husobee/vestigo"
)

func envOrFlag(envKey string, flagVal *string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if flagVal != nil {
		return *flagVal
	}
	return ""
}

func main() {
	var serverPort = flag.String("port", "", "server port")
	flag.Parse()

	databaseURL := os.Getenv("DATABASE_URL")
	databaseReady := false
	srvPort := envOrFlag("PORT", serverPort)
	if srvPort == "" {
		srvPort = "8080"
	}

	if databaseURL != "" {
		if err := models.InitDB(databaseURL); err != nil {
			log.Printf("Warning: database connection unavailable: %v", err)
		} else {
			databaseReady = true
		}
	} else {
		log.Println("Warning: DATABASE_URL not configured, database endpoints will not work")
	}
	clerk.SetKey(os.Getenv("CLERK_SECRET_KEY"))

	router := vestigo.NewRouter()
	clerkMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			clerkhttp.WithHeaderAuthorization(
				clerkhttp.AuthorizationFailureHandler(http.HandlerFunc(routes.UnauthorizedJSON)),
			)(next).ServeHTTP(w, r)
		}
	}

	router.Get("/api/auth/user", routes.AuthUser)
	router.Get("/health", routes.Health)
	router.Post("/api/generate-document", routes.GenerateDocument)
	router.Post("/api/regenerate-section", routes.RegenerateSection)
	router.Post("/api/transcribe", routes.Transcribe)
	router.Post("/api/transcribe-from-url", routes.TranscribeFromURL, clerkMiddleware)

	// audio
	router.Post("/api/audio/start", routes.ConvexAuth(routes.AudioStart))
	router.Post("/api/audio/chunk/:recordingId", routes.ConvexAuth(routes.AudioChunk))
	router.Get("/api/audio/status/:recordingId", routes.ConvexAuth(routes.AudioStatus))

	// audio
	router.Post("/api/audio/complete/:recordingId", routes.WithConvexAuth(routes.AudioComplete))
	router.Post("/api/audio/trigger-interim/:recordingId", routes.WithConvexAuth(routes.AudioTriggerInterim))

	if databaseReady {
		router.Get("/api/transcriptions/:id", routes.GetTranscription, clerkMiddleware)
		router.Post("/api/transcriptions", routes.CreateTranscription)
		router.Get("/api/transcriptions", routes.ListTranscriptions)
		router.Get("/activenotes", routes.ActiveNotes)
		router.Get("/tags/:key/:value", routes.FilterNotesByTag)
	}

	log.Println("Starting web server")
	log.Printf("Starting on port %s", srvPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", srvPort), router))
}
