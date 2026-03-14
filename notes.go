package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"notes/models"
	"notes/routes"
	"os"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	_ "github.com/go-sql-driver/mysql"
	"github.com/husobee/vestigo"
)

type Env struct {
	db *sql.DB
}

func main() {
	var userName = flag.String("user", "", "db username")
	var password = flag.String("pass", "", "db password")
	var databaseName = flag.String("db", "", "db name")
	var serverPort = flag.String("port", "", "server port")
	flag.Parse()

	// open connection to db
	connectionString := fmt.Sprintf("%s:%s@/%s?parseTime=true", *userName, *password, *databaseName)
	models.InitDB(connectionString)
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
	router.Get("/notes", routes.AllNotes)
	router.Post("/notes", routes.AddNote)
	router.Put("/notes/:id", routes.AddTags)
	router.Delete("/notes/:id", routes.DeleteNote)
	router.Get("/api/transcriptions/:id", routes.GetTranscription, clerkMiddleware)
	router.Post("/api/transcriptions", routes.CreateTranscription)
	router.Get("/api/transcriptions", routes.ListTranscriptions)

	// common queries
	router.Get("/activenotes", routes.ActiveNotes)

	// health
	router.Get("/health", routes.Health)
	router.Post("/api/generate-document", routes.GenerateDocument)
	router.Post("/api/regenerate-section", routes.RegenerateSection)
	router.Post("/api/transcribe", routes.Transcribe)
	router.Post("/api/transcribe-from-url", routes.TranscribeFromURL, clerkMiddleware)

	// audio
	router.Post("/api/audio/chunk/:recordingId", routes.ConvexAuth(routes.AudioChunk))

	// audio
	router.Post("/api/audio/start", routes.ConvexAuth(routes.AudioStart))

	// filters
	router.Get("/tags/:key/:value", routes.FilterNotesByTag)

	// audio
	router.Post("/api/audio/complete/:recordingId", routes.WithConvexAuth(routes.AudioComplete))

	log.Println("Starting web server")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", *serverPort), router))
}
