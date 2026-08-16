package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/oogle99/chirpy/internal/auth"
	"github.com/oogle99/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
	secret         string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, req)
	})
}

func handlerReadiness(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) handlerFileserverHits(w http.ResponseWriter, _ *http.Request) {
	hits := cfg.fileserverHits.Load()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	msg := fmt.Sprintf(`
		<html>
		<body>
			<h1>Welcome, Chirpy Admin</h1>
			<p>Chirpy has been visited %d times!</p>
		</body>
		</html>`, hits)
	w.Write([]byte(msg))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, req *http.Request) {
	if cfg.platform != "dev" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	cfg.fileserverHits.Store(0)

	err := cfg.queries.ResetDB(req.Context())
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}
}

func (cfg *apiConfig) handlerChirps(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	type chirp struct {
		Body string `json:"body"`
	}

	c := chirp{}
	err := decodeJSON(req, &c)
	if respondWithErrorIfErr(w, http.StatusBadRequest, err) {
		return
	}
	if len([]rune(c.Body)) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	token, err := auth.GetBearerToken(req.Header)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.secret)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	chirpDb, err := cfg.queries.CreateChirp(req.Context(), database.CreateChirpParams{
		Body:   c.Body,
		UserID: userId,
	})
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	respondWithJSON(w, 201, map[string]string{
		"id":         chirpDb.ID.String(),
		"created_at": chirpDb.CreatedAt.String(),
		"updated_at": chirpDb.UpdatedAt.String(),
		"body":       replaceProfane(chirpDb.Body),
		"user_id":    chirpDb.UserID.String(),
	})
}

func (cfg *apiConfig) handlerGetAllChirps(w http.ResponseWriter, req *http.Request) {
	chirps, err := cfg.queries.GetAllChirps(req.Context())
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	type chirpResponse struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body      string `json:"body"`
		UserID    string `json:"user_id"`
	}

	responses := make([]chirpResponse, 0, len(chirps))

	for _, chirp := range chirps {
		responses = append(responses, chirpResponse{
			ID:        chirp.ID.String(),
			CreatedAt: chirp.CreatedAt.String(),
			UpdatedAt: chirp.UpdatedAt.String(),
			Body:      chirp.Body,
			UserID:    chirp.UserID.String(),
		})
	}

	respondWithJSON(w, 200, responses)
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, req *http.Request) {
	pathUUID, err := uuid.Parse(req.PathValue("chirpID"))
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	chirpDb, err := cfg.queries.GetChirp(req.Context(), pathUUID)
	if respondWithErrorIfErr(w, http.StatusNotFound, err) {
		return
	}

	respondWithJSON(w, 200, map[string]string{
		"id":         chirpDb.ID.String(),
		"created_at": chirpDb.CreatedAt.String(),
		"updated_at": chirpDb.UpdatedAt.String(),
		"body":       chirpDb.Body,
		"user_id":    chirpDb.UserID.String(),
	})
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, req *http.Request) {
	type body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	e := body{}
	err := decodeJSON(req, &e)
	if respondWithErrorIfErr(w, http.StatusBadRequest, err) {
		return
	}

	hashedPassword, err := auth.HashPassword(e.Password)
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	user, err := cfg.queries.CreateUser(req.Context(), database.CreateUserParams{
		Email:          e.Email,
		HashedPassword: hashedPassword,
	})
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	respondWithJSON(w, 201, map[string]string{
		"id":         user.ID.String(),
		"created_at": user.CreatedAt.String(),
		"updated_at": user.UpdatedAt.String(),
		"email":      user.Email,
	})
}

func (cfg *apiConfig) handlerLoginUser(w http.ResponseWriter, req *http.Request) {
	type body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	e := body{}
	err := decodeJSON(req, &e)
	if respondWithErrorIfErr(w, http.StatusBadRequest, err) {
		return
	}

	user, err := cfg.queries.GetUser(req.Context(), e.Email)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	match, err := auth.CheckPasswordHash(e.Password, user.HashedPassword)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	if !match {
		respondWithErrorIfErr(w, http.StatusUnauthorized, errors.New("Incorrect email or password"))
		return
	}

	jwToken, err := auth.MakeJWT(user.ID, cfg.secret)
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	const refreshTokenDuration = 24 * 60 * time.Hour
	refToken := auth.MakeRefreshToken()
	refTokenDb, err := cfg.queries.CreateRefreshToken(req.Context(), database.CreateRefreshTokenParams{
		Token:     refToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(refreshTokenDuration),
	})
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	if err := cfg.queries.UpdateUser(req.Context(), user.ID); err != nil {
		respondWithErrorIfErr(w, http.StatusInternalServerError, err)
		return
	}

	respondWithJSON(w, 200, map[string]string{
		"id":            user.ID.String(),
		"created_at":    user.CreatedAt.String(),
		"updated_at":    user.UpdatedAt.String(),
		"email":         user.Email,
		"token":         jwToken,
		"refresh_token": refTokenDb.Token,
	})
}

func (cfg *apiConfig) handlerRefreshToken(w http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	userId, err := cfg.queries.GetUserFromRefreshToken(req.Context(), token)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	if err := cfg.queries.UpdateUser(req.Context(), userId); err != nil {
		respondWithErrorIfErr(w, http.StatusInternalServerError, err)
		return
	}

	jwToken, err := auth.MakeJWT(userId, cfg.secret)
	if respondWithErrorIfErr(w, http.StatusInternalServerError, err) {
		return
	}

	respondWithJSON(w, 200, map[string]string{
		"token": jwToken,
	})
}

func (cfg *apiConfig) handlerRevokeToken(w http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	userId, err := cfg.queries.GetUserFromRefreshToken(req.Context(), token)
	if respondWithErrorIfErr(w, http.StatusUnauthorized, err) {
		return
	}

	if err := cfg.queries.RevokeRefreshToken(req.Context(), userId); err != nil {
		respondWithErrorIfErr(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func replaceProfane(msg string) string {
	profanity := []string{"kerfuffle", "sharbert", "fornax"}

	splitMsg := strings.Split(msg, " ")
	for idx, word := range splitMsg {
		for _, profaneWord := range profanity {
			loweredWord := strings.ToLower(word)
			if loweredWord == profaneWord {
				splitMsg[idx] = "****"
			}
		}
	}

	return strings.Join(splitMsg, " ")
}

func decodeJSON[T any](req *http.Request, v *T) error {
	return json.NewDecoder(req.Body).Decode(v)
}

func respondWithErrorIfErr(w http.ResponseWriter, status int, err error) bool {
	if err != nil {
		respondWithError(w, status, err.Error())
		return true
	}
	return false
}

func respondWithError(w http.ResponseWriter, code int, msg string) error {
	return respondWithJSON(w, code, map[string]string{"error": msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) error {
	response, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	w.Write(response)
	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	envSecret := os.Getenv("SECRET")
	envPlatform := os.Getenv("PLATFORM")
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("error opening db connection: %v", err)
	}
	dbQueries := database.New(db)

	const port = "8080"
	const filepathRoot = "."

	apiCfg := apiConfig{
		queries:  dbQueries,
		platform: envPlatform,
		secret:   envSecret,
	}

	mux := &http.ServeMux{}
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))))

	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerFileserverHits)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerChirps)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLoginUser)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetAllChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirp)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefreshToken)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevokeToken)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving files from %s on port: %s\n", filepathRoot, port)
	log.Fatal(srv.ListenAndServe())
}
