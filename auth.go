package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/sha3"

	_ "github.com/mattn/go-sqlite3"
)

const AUTH_db_SQL_Header = `
PRAGMA foreign_keys = ON;
/* PRAGMA journal_mode = WAL; */
PRAGMA secure_delete = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = normal;

CREATE TABLE IF NOT EXISTS Users (
	UserId   INTEGER PRIMARY KEY AUTOINCREMENT,
	Username TEXT NOT NULL UNIQUE,	/* lowercase username */
	UsernameDisplay TEXT NOT NULL,
	SecuredPassword TEXT NOT NULL,
	
	Role INTEGER NOT NULL DEFAULT 0,
	
	CreatedAt DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME
);
CREATE INDEX IF NOT EXISTS idx_users_username ON Users(Username);

CREATE TABLE IF NOT EXISTS Sessions (
	TokenId TEXT PRIMARY KEY UNIQUE,
	UserId  INTEGER NOT NULL,
	
	ExpireAt  DATETIME NOT NULL,
	CreatedAt DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_userid ON Sessions(UserId);

`

const (
	AUTH_USERNAME_MAX_LENGTH = 64
	AUTH_PASSWORD_MAX_LENGTH = 256
	
	AUTH_DB_FILENAME = "auth.db"
	
	AUTH_SESSION_TOKEN_COOKIE_NAME = "AYDM_SESSION_AUTH_TOKEN"
)

const (
	AUTH_ROLE_NONE  = 0
	AUTH_ROLE_ADMIN = 1
)

var G_AUTHDB *sql.DB

type AuthUser struct {
	UserId   uint64
	Username string
	UsernameDisplay string
	SecuredPassword string
	
	Role int
	
	CreatedAt time.Time
	UpdatedAt time.Time
}
type AuthSession struct {
	TokenId string
	UserId  uint64
	
	ExpireAt  time.Time
	CreatedAt time.Time
}

const RNGCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

func GenerateRandomString(length int) []byte {
	result := make([]byte, length)
	numMax := big.NewInt(int64(len(RNGCharacters)))
	for i := 0; i < length; i++ {
		num, _ := rand.Int(rand.Reader, numMax)
		
		result[i] = RNGCharacters[num.Int64()]
	}
	
	return result
}

func GetAuthUserFromUserId(UserId uint64) (*AuthUser, error) {
	AUserRow := G_AUTHDB.QueryRow(`
	SELECT UserId, Username, UsernameDisplay, SecuredPassword, Role, CreatedAt, UpdatedAt FROM Users WHERE UserId = ?
	`, UserId)
	
	AUser := &AuthUser{}
	err := AUserRow.Scan(
		&AUser.UserId,
		&AUser.Username,
		&AUser.UsernameDisplay,
		&AUser.SecuredPassword,
		
		&AUser.Role,
		
		&AUser.CreatedAt,
		&AUser.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to find user, error: %v\n", err)
	}
	
	return AUser, nil
}

func GetAuthUserFromUsername(Username string) (*AuthUser, error) {
	Row := G_AUTHDB.QueryRow(`
	SELECT UserId FROM Users WHERE Username = ?
	`, strings.ToLower(Username))
	var UserId uint64
	err := Row.Scan(&UserId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to find UserId, error: %v\n", err)
	}
	
	AUser, err := GetAuthUserFromUserId(UserId)
	if err != nil {
		return nil, err
	}
	
	return AUser, nil
}

func GetAuthUserFromSessionToken(Token string) (*AuthUser, error) {
	SessionRow := G_AUTHDB.QueryRow(`
	SELECT TokenId, UserId, ExpireAt, CreatedAt FROM Sessions WHERE TokenId = ?
	`, Token)
	
	Session := &AuthSession{}
	err := SessionRow.Scan(
		&Session.TokenId,
		&Session.UserId,
		
		&Session.ExpireAt,
		&Session.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to find session from database, error: %v\n", err)
	}
	
	if time.Now().UnixMilli() > Session.ExpireAt.UnixMilli() {
		// Expired...
		return nil, nil //fmt.Errorf("Session is expired.")
	}
	
	AUser, err := GetAuthUserFromUserId(Session.UserId)
	if err != nil {
		return nil, fmt.Errorf("Failed to find user from userid, error: %v\n", err)
	}
	
	if AUser == nil {
		// User doesn't exist?
		return nil, nil
	}
	
	return AUser, nil
}

func CreateAuthSession(AUser *AuthUser) (*AuthSession, error) {
	Session := &AuthSession{}
	Session.UserId = AUser.UserId
	Session.TokenId = fmt.Sprintf("%012d|%s", AUser.UserId, GenerateRandomString(128))
	
	Session.CreatedAt = time.Now().UTC()
	Session.ExpireAt = time.Now().UTC().Add(time.Second*60*60*24*365)  // Expire in 1 year
	
	_, err := G_AUTHDB.Exec(`
	INSERT INTO Sessions(TokenId, UserId, ExpireAt, CreatedAt)
	VALUES (?, ?, ?, ?)
	`, Session.TokenId, Session.UserId, Session.ExpireAt, Session.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("Failed to insert session into database, error: %v", err)
	}
	
	return Session, nil
}

func IsRequestAuthorized(r *http.Request) (bool, error) {
	AuthorizationToken := r.Header.Get("authorization")
	if AuthorizationToken != "" {
		// TODO:
	}
	
	AuthorizationCookie, err := r.Cookie(AUTH_SESSION_TOKEN_COOKIE_NAME)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return false, nil
		}
		
		return false, fmt.Errorf("Failed to get auth cookie, error %v", err)
	}
	if AuthorizationCookie != nil {
		SessionToken := AuthorizationCookie.Value
		
		AUser, err := GetAuthUserFromSessionToken(SessionToken)
		if err != nil {
			return false, fmt.Errorf("Error when getting auth user from session token: %v", err)
		}
		
		if AUser == nil {
			// Could not find user?
			return false, nil
		}
		if AUser != nil {
			if AUser.Role == AUTH_ROLE_ADMIN {
				return true, nil
			}
		}
	}
	
	return false, nil
}


func IsUserRequestSignedByServer(r *http.Request, Queries []string) bool {
	// TODO:
	
	return false
}

// This returns the raw bytes of a sha512 hash. Used for bcrypt because it is limited to 72 characters...
func HashRawPassword(RawPassword string) string {
	Sum := sha3.Sum512([]byte(RawPassword))
	return string(fmt.Sprintf("%s", Sum))
}

func AuthLoginRequest(w http.ResponseWriter, r *http.Request) {
	// TODO: IP based rate limiting
	
	var Body struct{
		Username    string `json:"username"`
		RawPassword string `json:"password"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	RequestUsername := Body.Username
	RequestPassword := Body.RawPassword
	
	if RequestUsername == "" {
		http.Error(w, "Empty username.", http.StatusBadRequest)
		return
	}
	if RequestPassword == "" {
		http.Error(w, "Empty password.", http.StatusBadRequest)
		return
	}
	
	if len(RequestUsername) > 1024 || len(RequestPassword) > 1024 {
		http.Error(w, "Username or password request is too long.", http.StatusBadRequest)
		return
	}
	
	AuthUser, err := GetAuthUserFromUsername(strings.ToLower(RequestUsername))
	if err != nil {
		L_Printf("Failed to find auth user, error: %v\n", err)
		http.Error(w, "Internal error when fetching user from database...", http.StatusInternalServerError)
		return
	}
	const UsernamePasswordMismatch_Message = "Username or password do not match."
	
	if AuthUser == nil {
		// This user does not exist.
		http.Error(w, UsernamePasswordMismatch_Message, http.StatusUnauthorized)
		return
	}
	
	PasswordErr := bcrypt.CompareHashAndPassword([]byte(AuthUser.SecuredPassword), []byte(HashRawPassword(RequestPassword)))
	if PasswordErr == nil {
		// This is the same password!
		// Create a new session token.
		NewSession, err := CreateAuthSession(AuthUser)
		if err != nil {
			// Could not create session cookie...
			L_Printf("Could not create auth session token, error: %v\n", err)
			http.Error(w, "Error when creating session token. Please try again.", http.StatusInternalServerError)
			return
		}
		
		http.SetCookie(w, &http.Cookie{
			Name:     AUTH_SESSION_TOKEN_COOKIE_NAME,
			Value:    NewSession.TokenId,
			Expires:  NewSession.ExpireAt,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"LoggedIn\":true}"))
		return
	}
	
	http.Error(w, UsernamePasswordMismatch_Message, http.StatusBadRequest)
	return
}
func AuthLogoutRequest(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    AUTH_SESSION_TOKEN_COOKIE_NAME,
		Value:    "",
		Expires:  time.Unix(0, 0),  // Expire the cookie NOW!!!
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func IsByteAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func IsByteNum(b byte) bool {
	return (b >= '0' && b <= '9')
}

func ValidateUsername(Username string) error {
	if len(Username) > AUTH_USERNAME_MAX_LENGTH {
		return fmt.Errorf("Username is too long! Must be below %d characters.", AUTH_USERNAME_MAX_LENGTH)
	}
	if len(Username) < 3 {
		return fmt.Errorf("Username is too short! Must 3 characters or more.")
	}
	
	// Check for illegal characters.
	for i := 0; i < len(Username); i++ {
		Character := Username[i]
		if Character != '_' && Character != '-' && !IsByteAlpha(Character) && !IsByteNum(Character) {
			return fmt.Errorf("Username contains invalid character! Only (a-Z, 0-9, _, -) are allowed!")
		}
	}
	
	// Username can be used
	
	return nil
}

func AuthCreateUserRequest(w http.ResponseWriter, r *http.Request, UserRole int) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Cannot parse form.", http.StatusBadRequest)
		return
	}
	
	var Body struct{
		Username    string `json:"username"`
		RawPassword string `json:"password"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	RequestUsername := Body.Username
	RequestPassword := Body.RawPassword
	
	UsernameErr := ValidateUsername(RequestUsername)
	if UsernameErr != nil {
		http.Error(w, UsernameErr.Error(), http.StatusBadRequest)
		return
	}
	
	if len(RequestPassword) >= AUTH_PASSWORD_MAX_LENGTH {
		http.Error(w, "Password is too long.", http.StatusBadRequest)
		return
	}
	if len(RequestPassword) < 8 {
		http.Error(w, "Password must be 8 characters or more.", http.StatusBadRequest)
		return
	}
	
	SecuredPassword, err := bcrypt.GenerateFromPassword([]byte(HashRawPassword(RequestPassword)), bcrypt.DefaultCost)
	if err != nil {
		L_Printf("Could not generate new password, error: %v\n", err)
		http.Error(w, "Error when creating user???", http.StatusInternalServerError)
		return
	}
	NewUser := &AuthUser{
		Username: strings.ToLower(RequestUsername),
		UsernameDisplay: RequestUsername,
		
		SecuredPassword: string(SecuredPassword),
		
		Role: UserRole,
		
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	
	Result, err := G_AUTHDB.Exec(`
	INSERT INTO Users(Username, UsernameDisplay, SecuredPassword, Role, CreatedAt, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?)
	`, NewUser.Username, NewUser.UsernameDisplay, NewUser.SecuredPassword, NewUser.Role, NewUser.CreatedAt, NewUser.UpdatedAt)
	if err != nil {
		L_Printf("Could not create new user, database error: %v\n", err)
		
		http.Error(w, "Internal error when creating user...", http.StatusInternalServerError)
		return
	}
	
	UserId, err := Result.LastInsertId()
	if err != nil {
		L_Printf("Could not create new user, database error: %v\n", err)
		
		http.Error(w, "Internal error when creating user...", http.StatusInternalServerError)
		return
	}
	NewUser.UserId = uint64(UserId)
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
	return
}

func DoesAdminAccountExist() (bool, error) {
	// TODO: this is temp, find a better way of checking if an admin account exists.
	AUser, err := GetAuthUserFromUserId(1)
	if err != nil {
		L_Printf("GetAuthUserFromUserId error: %v\n", err)
		return false, err
	}
	
	if AUser != nil {
		return true, nil
	}
	
	return false, nil
}


func OpenAuthDB() error {
	db, err := sql.Open("sqlite3", AUTH_DB_FILENAME)
	if err != nil {
		return fmt.Errorf("Failed to open auth database '%s' Error: %v\n", AUTH_DB_FILENAME, err)
	}
	
	G_AUTHDB = db
	
	_, err = db.Exec(AUTH_db_SQL_Header)
	if err != nil {
		return fmt.Errorf("Failed run auth database header '%s' Error: %v\n", AUTH_DB_FILENAME, err)
	}
	
	AuthDatabaseUpgrades := []string{
		// TODO: TEMP!
		"ALTER TABLE Users ADD COLUMN UsernameDisplay TEXT NOT NULL",
		"ALTER TABLE Users ADD COLUMN SecuredPassword TEXT NOT NULL",
	}
	
	for i, Upgrade := range(AuthDatabaseUpgrades) {
		_, err = db.Exec(Upgrade)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			L_Printf("Upgrade[%d] failed, error: %v\n", i, err)
		}
	}
	
	G_AUTHDB = db
	
	return nil
}

