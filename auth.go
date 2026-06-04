package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"encoding/base64"
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

CREATE TABLE IF NOT EXISTS Sessions (
	TokenId TEXT PRIMARY KEY UNIQUE,
	UserId  INTEGER NOT NULL,
	
	ExpireAt  DATETIME NOT NULL,
	CreatedAt DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS Settings (
	Id TEXT PRIMARY KEY,
	
	SecretSaltHash TEXT
);

`

const (
	AUTH_USERNAME_MAX_LENGTH = 64
	AUTH_PASSWORD_MAX_LENGTH = 256
	
	AUTH_DB_FILENAME = "auth.db"
	AUTH_REQUEST_SIGN_QUERYNAME = "ss"
)

var AUTH_SESSION_TOKEN_COOKIE_NAME = "AYDM_SESSION_AUTH_TOKEN"

const (
	AUTH_ROLE_NONE  = 0
	AUTH_ROLE_ADMIN = 1
)

var G_AUTHDB *sql.DB

type AuthUser struct {
	UserId   uint64 `json:"userid"`
	Username string        `json:"raw_username"`
	UsernameDisplay string `json:"username"`
	SecuredPassword string `json:"-"`
	
	Role int `json:"role"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

func SRNG(Min int, Max int) int {
	numMax := big.NewInt(int64(Max-Min))
	num, _ := rand.Int(rand.Reader, numMax)
	return (int(num.Int64()) + Min)
}

var G_SecretSaltHash string

func GetAuthSecretSaltHash() string {
	if G_SecretSaltHash != "" {
		return G_SecretSaltHash
	}
	
	Row := G_AUTHDB.QueryRow(`
	SELECT SecretSaltHash FROM Settings WHERE Id = "Global"
	`)
	var SecretSaltHash string
	err := Row.Scan(&SecretSaltHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ""
		}
		L_Printf("Error when getting SecretSaltHash from global table, %v\n.\n", err)
		return ""
	}
	
	G_SecretSaltHash = SecretSaltHash
	
	return SecretSaltHash
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

func GetAuthUserFromRequest(r *http.Request) (*AuthUser, error) {
	AuthorizationCookie, err := r.Cookie(AUTH_SESSION_TOKEN_COOKIE_NAME)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		
		return nil, fmt.Errorf("Failed to get auth cookie, error %v", err)
	}
	if AuthorizationCookie != nil {
		SessionToken := AuthorizationCookie.Value
		
		AUser, err := GetAuthUserFromSessionToken(SessionToken)
		if err != nil {
			return nil, fmt.Errorf("Error when getting auth user from session token: %v", err)
		}
		
		if AUser != nil {
			// Auth user was found!
			return AUser, nil
		}
	}
	
	return nil, nil
}

func IsRequestAuthorized(r *http.Request) (bool, error) {
	AuthorizationToken := r.Header.Get("authorization")
	if AuthorizationToken != "" {
		// TODO:
	}
	
	AUser, err := GetAuthUserFromRequest(r)
	if err != nil {
		return false, fmt.Errorf("Could not get auth user, error %v", err)
	}
	
	if AUser == nil {
		return false, nil
	}
	if AUser.Role == AUTH_ROLE_ADMIN {
		return true, nil
	}
	
	return false, nil
}

type SQuery struct {
	Name  string
	Value string
}

func GenerateSignedUserRequest(LocalUrl string, Queries []SQuery) string {
	SecretSaltHash := GetAuthSecretSaltHash()
	
	Path := LocalUrl
	
	Hash := sha3.New224()
	Hash.Write([]byte(LocalUrl))
	Hash.Write([]byte(SecretSaltHash))
	
	Queries = append(Queries, SQuery{
		"sr", fmt.Sprintf("%04d", SRNG(0, 9999)),
	})
	
	QueryId := 0
	for _, Query := range(Queries) {
		Hash.Write([]byte(Query.Value))
		if Query.Value != "" {
			if QueryId == 0 {
				Path += fmt.Sprintf("?%s=%s", Query.Name, Query.Value)
			} else {
				Path += fmt.Sprintf("&%s=%s", Query.Name, Query.Value)
			}
			
			QueryId += 1
		}
	}
	
	ComputedHash := base64.RawURLEncoding.EncodeToString([]byte(Hash.Sum(nil)))
	if QueryId == 0 {
		Path += fmt.Sprintf("?%s=%s", AUTH_REQUEST_SIGN_QUERYNAME, ComputedHash)
	} else {
		Path += fmt.Sprintf("&%s=%s", AUTH_REQUEST_SIGN_QUERYNAME, ComputedHash)
	}
	QueryId += 1
	
	return Path
}

func IsUserRequestSignedByServer(r *http.Request, Queries []string) bool {
	SignedHash := r.URL.Query().Get(AUTH_REQUEST_SIGN_QUERYNAME)
	if SignedHash == "" {
		return false
	}
	
	SecretSaltHash := GetAuthSecretSaltHash()
	
	Hash := sha3.New224()
	Hash.Write([]byte(r.URL.Path))
	Hash.Write([]byte(SecretSaltHash))
	for _, Query := range(Queries) {
		Value := r.URL.Query().Get(Query)
		Hash.Write([]byte(Value))
	}
	Sr := r.URL.Query().Get("sr")
	Hash.Write([]byte(Sr))
	
	RawHash := Hash.Sum(nil)
	
	ComputedHash := base64.RawURLEncoding.EncodeToString(RawHash)
	// TODO: TEMP!!!!!!!!! the `SignedHash == fmt.Sprintf("%x", RawHash)` is TEMPORARY!!! Please remove it.
	if SignedHash == ComputedHash || SignedHash == fmt.Sprintf("%x", RawHash) {
		return true
	}
	
	L_Printf("ss: %s, ComputedHash: %s\n", SignedHash, ComputedHash)
	
	return false
}

// This returns the raw bytes of a sha512 hash. Used for bcrypt because it's limited to 72 characters...
func HashRawPassword(RawPassword string) string {
	Sum := sha3.Sum512([]byte(RawPassword))
	return string(fmt.Sprintf("%s", Sum))
}

func AuthLoginRequest(w http.ResponseWriter, r *http.Request) {
	if TestRateLimitForRequest(w, r, RATE_LIMIT_BUCKET_LOGIN) {
		http.Error(w, "Too many log in attempts, try again in a few minutes.", http.StatusTooManyRequests)
		return
	}
	
	// :Login_time_attack
	// This is to prevent timing attacks!
	// If the username is found then it will spend around ~50ms comparing the raw password and the bcrypt password.
	// Forcefully sleeping a random amount of time before responding should stop people from reverse engineering the admin username.
	TimeWait := time.Now().UTC().Add(time.Millisecond * (80))
	TimeWait = TimeWait.Add(time.Microsecond * time.Duration(SRNG(0, 100_000)))
	
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
	const UsernamePasswordMismatch_Message = "Username and password do not match."
	
	if AuthUser == nil {
		// This user does not exist.
		
		time.Sleep(TimeWait.Sub(time.Now().UTC()))  // Sleep for a small amount before returning, see :Login_time_attack
		
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
	
	time.Sleep(TimeWait.Sub(time.Now().UTC()))  // Sleep for a small amount before returning, see :Login_time_attack
	
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
	
	SecretSaltHash := GetAuthSecretSaltHash()
	if SecretSaltHash == "" {
		// Create the secret salt!
		
		Key2Length := SRNG(64, 256)
		NewSaltHash := fmt.Sprintf("TIME_CREATED:%d|%s,%s", time.Now().UnixMicro(), GenerateRandomString(512), GenerateRandomString(Key2Length))
		G_SecretSaltHash = NewSaltHash
		
		_, err := G_AUTHDB.Exec(`
		INSERT OR REPLACE INTO Settings(Id, SecretSaltHash)
		VALUES ("Global", ?)
		`, NewSaltHash)
		if err != nil {
			return fmt.Errorf("Could not set SecretSaltHash in auth database? error: %v", err)
		}
	}
	
	AuthDatabaseUpgrades := []string{
		// Empty for the time being.
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


func init() {
	if APPLICATION_VERSION_TYPE == "debug" {
		AUTH_SESSION_TOKEN_COOKIE_NAME = "AYDM_SESSION_AUTH_TOKEN_DEBUG"
	}
}
