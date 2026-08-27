package main

import (
	//"context"
	"flag"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
	
	"fmt"
	"net/http"
)

var (
	APPLICATION_VERSION_NUMBER = "v0.21"
	APPLICATION_VERSION_TYPE = "alpha"
	APPLICATION_VERSION = APPLICATION_VERSION_NUMBER + " " + APPLICATION_VERSION_TYPE
	
	CURRENT_WORKING_DIRECTORY = ""
)

var PRINT_NETWORK_REQUESTS = true

var HttpServer *http.Server

func SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		UserAgent := r.Header.Get("User-Agent")
		if len(UserAgent) > 512 {
			L_Printf("Request[%s] Path: %s | Invalid User-Agent.\n", GetIpAddressFromRequest(r), r.URL.Path)
			http.Error(w, "Invalid User-Agent.", http.StatusBadRequest)
			return
		}
		if RateLimitRequest(w, r, RATE_LIMIT_BUCKET_GLOBAL) {
			// This request was rate limited.
			return
		}
		
		if PRINT_NETWORK_REQUESTS {
			L_Printf("-------- %s | %s --------\n", GetIpAddressFromRequest(r), r.RequestURI)
			
			/*
			for Key, Value := range(r.Header) {
				Pn := fmt.Sprintf(" Key: %s: ", Key)
				for _, Str := range(Value) {
					Pn += fmt.Sprintf("%s ", Str)
				}
				
				L_Printf("%s\n", Pn)
			}
			L_Printf("\n")
			*/
		}
		
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		
		next.ServeHTTP(w, r)
	})
}

func StartServer(ServerPort int) {
	Mux := http.NewServeMux()
	
	// Static handlers
	Mux.HandleFunc("/static/", webstatic_ServeStaticContent)
	Mux.HandleFunc("/", webstatic_ServeStaticContent)            // This should serve index.html
	Mux.HandleFunc("/favicon.ico", webstatic_ServeStaticContent) // This should serve favicon.png
	
	Mux.HandleFunc("/api/", ServeApi)
	Mux.HandleFunc("/video-file/", ServeVideoDownload)
	Mux.HandleFunc("/video-stream/", ServeVideoStream)
	Mux.HandleFunc("/db-image/", ServeDBImage)
	
	
	HttpServer = &http.Server{
		Addr: fmt.Sprintf(":%d", ServerPort),
		Handler: SecurityMiddleware(Mux),
	}
	
	L_Printf("Starting server at http://localhost:%d\n", ServerPort)
	
	if err := HttpServer.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			L_Printf("The http server has closed.\n")
		} else {
			L_Printf("Cannot start server because: %v\n", err)
			L_Printf("The server port might be currently occupied... Edit the server port in config.json if you need to change the server port!\n")
			panic(err)
		}
	}
}

func OnProgramExit() {
	DB_Close()
	AuthDB_Close()
}

var ProgramShouldExit = false

func G_ExitProgram() {
	ProgramShouldExit = true
}

func main() {
	fmt.Printf("------- auto yt-dlp manager -------\n")
	InitLogPrint()
	L_Printf("APPLICATION_VERSION: %s\n\n", APPLICATION_VERSION)
	
	var err error
	CURRENT_WORKING_DIRECTORY, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	
	ConfigPath := CONFIG_PATH
	if APPLICATION_VERSION_TYPE == "debug" {
		ConfigPath = CONFIG_PATH_DEBUG
	}
	
	err = OpenConfig(ConfigPath)
	if err != nil {
		panic(err)
	}
	
	G_Config.YtDlp_Path_Real     = FindCommand(G_Config.YtDlp_Path)
	G_Config.YtArchive_Path_Real = FindCommand(G_Config.YtArchive_Path)
	G_Config.FFmpeg_Path_Real    = FindCommand(G_Config.FFmpeg_Path)
	G_Config.Deno_Path_Real      = FindCommand(G_Config.Deno_Path)
	
	Exit := false
	
	if !CommandExists(G_Config.YtDlp_Path_Real) {
		Exit = true
		L_Printf("Could not find program '%s' Get the dependency here: https://github.com/yt-dlp/yt-dlp \n\n", G_Config.YtDlp_Path)
	}
	if !CommandExists(G_Config.YtArchive_Path_Real) {
		Exit = true
		L_Printf("Could not find program '%s' Get the dependency here: https://github.com/dreammu/ytarchive\n\n", G_Config.YtArchive_Path)
	}
	if !CommandExists(G_Config.FFmpeg_Path_Real) {
		Exit = true
		L_Printf("Could not find program '%s' Get the dependency here: https://www.ffmpeg.org/ \n\n", G_Config.FFmpeg_Path)
	}
	
	if !CommandExists(G_Config.Deno_Path_Real) {
		L_Printf("Could not find optional dependency 'deno' (Used by yt-dlp) It's recommended that you get this for yt-dlp to fully work properly.\n")
		L_Printf("If you experience long wait times for yt-dlp or missing formats, then getting deno *might* fix the issue...: https://github.com/denoland/deno/releases\n\n")
	}
	
	if Exit {
		L_Printf("The program will exit now due to unavailable dependencies...\n")
		L_Printf("If you already have these dependencies, edit the paths in ./config.json to the correct location\n")
		time.Sleep(time.Second * 10)
		return
	}
	
	err = OpenDB()
	if err != nil {
		L_Printf("Database failed to open | %v\n", err)
		panic(err)
		return
	}
	err = OpenAuthDB()
	if err != nil {
		L_Printf("Auth database failed to open | %v\n", err)
		panic(err)
		return
	}
	
	defer OnProgramExit()
	
	{
		// Handle command arguments
		CreateAdminUsername := flag.String("create-admin-username", "", "Create the admin account username, must be used with '--create-admin-password'")
		CreateAdminPassword := flag.String("create-admin-password", "", "Create the admin account password, must be used with '--create-admin-username'")
		
		flag.Parse()
		
		if *CreateAdminUsername != "" && *CreateAdminPassword == "" {
			L_Printf("Could not create admin account: '--create-admin-username' must be used along with '--create-admin-password'!\n")
			return
		} else if *CreateAdminUsername == "" && *CreateAdminPassword != "" {
			L_Printf("Could not create admin account: '--create-admin-password' must be used along with '--create-admin-username'!\n")
			return
		}
		if *CreateAdminUsername != "" && *CreateAdminPassword != "" {
			AdminAccountExists, err := DoesAdminAccountExist()
			if err != nil {
				L_Printf("Failed to check if admin account already exists? error: %v\n", err)
				return
			}
			if AdminAccountExists {
				L_Printf("Admin account already exists!!! Please run the program normally without the '--create-admin-username' and '--create-admin-password' command arguments!\n")
				return
			}
			NewUser, err := AuthCreateUser(*CreateAdminUsername, *CreateAdminPassword, AUTH_ROLE_ADMIN)
			if err != nil {
				L_Printf("\nFailed to create admin account! error: %v\n\n", err)
				return
			}
			L_Printf("Admin account created! You can now log in on the webui. \"Username\": %s\n", NewUser.UsernameDisplay)
		}
	}
	
	go InitDownloading()
	
	// TODO: THIS IS TEMP! go yt_chat_Run("https://www.youtube.com/watch?v=G5oz2dQLi00", "./test-chat-output.json", nil)
	
	go StartServer(int(G_Config.ServerPort))
	
	ExitSignal := make(chan os.Signal, 1)
	signal.Notify(ExitSignal, syscall.SIGINT, syscall.SIGTERM)
	
	//StartSystray()
	
	for {
		select {
		case sig := <-ExitSignal:
			// 😈
			if sig != nil {
				Exit = true
			}
		default:
		}
		if Exit || ProgramShouldExit {
			break
		}
		
		time.Sleep(50 * time.Millisecond)
	}
	
	// The program will exit now.
	
	/*
	err = HttpServer.Shutdown(context.Background())
	if err != nil {
		L_Printf("Failed to shutdown http server. Error: %v\n", err)
	}
	*/
	//ExitProgram()
}