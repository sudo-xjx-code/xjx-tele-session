package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	// "github.com/gotd/td/telegram/session"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

type TelegramSession struct {
	PhoneNumber string `json:"phone_number"`
	Code        string `json:"code"`
}

type authFlow struct {
	client *telegram.Client
	codeCh chan string
	errCh  chan error
}

var logger *zap.Logger

var sessions = make(map[string]*telegram.Client)
var appID int
var appHash, sessionDir string

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	appID = mustGetIntEnv("APP_ID")
	appHash = mustGetEnv("APP_HASH")
	sessionDir = mustGetEnv("SESSION_DIR")

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		err := os.Mkdir(sessionDir, 0755)
		if err != nil {
			log.Fatalf("Error creating session directory: %v", err)
		}
	}

	r := mux.NewRouter()
	r.HandleFunc("/login", loginHandler).Methods("POST")
	r.HandleFunc("/verify", verifyHandler).Methods("POST")
	r.HandleFunc("/backup", backupHandler).Methods("GET")

	http.Handle("/", r)

	// Restore sessions from files
	restoreSessions()

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Error: Environment variable %s not set", key)
	}
	return value
}

func mustGetIntEnv(key string) int {
	value := mustGetEnv(key)
	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("Error: Environment variable %s must be an integer", key)
	}
	return intValue
}

func isSessionFile(name string) bool {
	return len(name) > 8 && name[len(name)-8:] == ".session"
}

func getPhoneFromFileName(name string) string {
	return name[:len(name)-8]
}

func restoreSessions() {
	files, err := os.ReadDir(sessionDir)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() || !isSessionFile(file.Name()) {
			continue
		}

		phone := getPhoneFromFileName(file.Name())
		client := telegram.NewClient(
			appID,
			appHash,
			telegram.Options{
				SessionStorage: &session.FileStorage{
					Path: filepath.Join(sessionDir, file.Name()),
				},
			},
		)

		err := client.Run(context.Background(), func(ctx context.Context) error {
			// Create a new API client
			api := tg.NewClient(client)

			// Verify if the session is valid
			me, err := api.UsersGetFullUser(ctx, &tg.InputUserSelf{})
			if err != nil {
				return fmt.Errorf("failed to get user info: %w", err)
			}

			// Print user information
			fmt.Printf("Logged in as: %s\n", &me.FullUser)
			fmt.Printf("Phone: %s\n", phone)
			return nil
		})

		if err != nil {
			log.Printf("Failed to restore session for %s: %v", phone, err)
			continue
		}

		sessions[phone] = client
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var teleSession TelegramSession
	err := json.NewDecoder(r.Body).Decode(&sessions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, exists := sessions[teleSession.PhoneNumber]; exists {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Session already active for phone number %s", teleSession.PhoneNumber)
		return
	}

	client := telegram.NewClient(
		appID,
		appHash,
		telegram.Options{
			Logger: logger,
			Device: telegram.DeviceConfig{
				SystemVersion:  "Windows 10",
				AppVersion:     "5.2.3 x64",
				DeviceModel:    "Desktop",
				SystemLangCode: "en-US",
				LangCode:       "en",
				LangPack:       "tdesktop",
			},
		},
	)

	codeCh := make(chan string)
	errCh := make(chan error)

	flow := &authFlow{
		client: client,
		codeCh: codeCh,
		errCh:  errCh,
	}

	sessions[teleSession.PhoneNumber] = flow.client

	go func() {
		err := client.Run(context.Background(), func(ctx context.Context) error {
			return client.Auth().IfNecessary(ctx, auth.NewFlow(
				auth.SendCodeOptions(teleSession.PhoneNumber, auth.CodeAuthenticatorFunc(func(ctx context.Context) (string, error) {
					select {
					case code := <-codeCh:
						return code, nil
					case err := <-errCh:
						return "", err
					case <-ctx.Done():
						return "", ctx.Err()
					}
				})),
				auth.SendCodeOptions{},
			))
		})

		if err != nil {
			errCh <- err
		}
	}()
	authFlow.codeCh <- teleSession.Code
	
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Verification code received for phone number %s", teleSession.PhoneNumber)
}

func verifyHandler(w http.ResponseWriter, r *http.Request) {
	var teleSession TelegramSession
	err := json.NewDecoder(r.Body).Decode(&teleSession)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	client, exists := sessions[teleSession.PhoneNumber]
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	authFlow, exists := authFlows[session.PhoneNumber]
	if !exists {
		http.Error(w, "Auth flow not found", http.StatusNotFound)
		return
	}

	err = client.Run(r.Context(), func(ctx telegram.AuthContext) error {
		return client.Auth().IfNecessary(ctx, auth.CodeAuth{
			PhoneNumber: session.PhoneNumber,
			Code:        session.Code,
		})
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	me, err := client.Self(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Printf("Logged in as %s\n", me.Username)

	delete(authFlows, session.PhoneNumber) // Cleanup auth flow
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Login verified for phone number %s", session.PhoneNumber)
}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	client, exists := sessions[phone]
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	sessionData, err := client.Session().Marshal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	me, err := client.Self(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileName := fmt.Sprintf("%s.session", phone)
	err = os.WriteFile(filepath.Join(sessionDir, fileName), sessionData, 0600)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionInfo := map[string]interface{}{
		"session_file":     fileName,
		"phone":            phone,
		"app_id":           appID,
		"app_hash":         appHash,
		"sdk":              "Windows 10",
		"app_version":      "5.2.3 x64",
		"device":           "Desktop",
		"lang_pack":        "tdesktop",
		"system_lang_pack": "en-US",
		"username":         me.Username,
		"ipv6":             false,
		"first_name":       me.FirstName,
		"last_name":        me.LastName,
		"register_time":    time.Now().Unix(),
		"sex":              nil,
		"last_check_time":  time.Now().Unix(),
		"lang_code":        "en",
		"avatar":           "img/default.png",
		"proxy":            nil,
		"twoFA":            "",
		"block":            false,
		"system_lang_code": "en-US",
		"id":               me.ID,
	}

	jsonFileName := fmt.Sprintf("%s.json", phone)
	jsonData, err := json.Marshal(sessionInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.WriteFile(filepath.Join(sessionDir, jsonFileName), jsonData, 0600)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Session backed up to %s and %s", fileName, jsonFileName)
}
