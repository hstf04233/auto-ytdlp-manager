package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	RATE_LIMIT_BUCKET_GLOBAL = 0
	RATE_LIMIT_BUCKET_API    = 1
	RATE_LIMIT_BUCKET_LOGIN  = 2
)

const (
	MAX_GLOBAL_REQUESTS_PER_MINUTE = 7_500
	MAX_API_REQUESTS_PER_MINUTE    = 1_000
)

type RateLimitInfo struct {
	mutex sync.Mutex
	
	Ip string
	
	TotalRequests int
	ApiRequests   int
	LoginRequests int
}

var NETWORKING_RATELIMIT_INFOS = NewCache(time.Second * 60)


func GetRateLimitInfoFromRequest(r *http.Request) *RateLimitInfo {
	IpAddress := GetIpAddressFromRequest(r)
	
	var RInfo *RateLimitInfo
	
	NETWORKING_RATELIMIT_INFOS.mutex.Lock()
	defer NETWORKING_RATELIMIT_INFOS.mutex.Unlock()
	
	RateLimitCache, Exists := NETWORKING_RATELIMIT_INFOS.GetNoMutex(IpAddress)
	if Exists {
		RInfo = RateLimitCache.(*RateLimitInfo)
	} else {
		RInfo = &RateLimitInfo{
			Ip: IpAddress,
			
			TotalRequests: 0,
			LoginRequests: 0,
		}
		NETWORKING_RATELIMIT_INFOS.SetNoMutex(IpAddress, RInfo)
	}
	
	return RInfo
}

// Returns true if rate limited.
func TestRateLimitForRequest(w http.ResponseWriter, r *http.Request, Bucket int) bool {
	RInfo := GetRateLimitInfoFromRequest(r)
	RInfo.mutex.Lock()
	defer RInfo.mutex.Unlock()
	
	if RInfo.TotalRequests > MAX_GLOBAL_REQUESTS_PER_MINUTE {
		return true
	}
	if Bucket == RATE_LIMIT_BUCKET_GLOBAL {
		RInfo.TotalRequests++
	} else if Bucket == RATE_LIMIT_BUCKET_API {
		if RInfo.LoginRequests > MAX_API_REQUESTS_PER_MINUTE {
			return true
		}
		RInfo.LoginRequests++
	} else if Bucket == RATE_LIMIT_BUCKET_LOGIN {
		if RInfo.LoginRequests > 20 {
			return true
		}
		RInfo.LoginRequests++
	}
	
	return false
}

// Returns true if rate limited.
func RateLimitRequest(w http.ResponseWriter, r *http.Request, Bucket int) bool {
	// Handle rate limit errors automatically.
	if TestRateLimitForRequest(w, r, Bucket) {
		L_Printf("Request[%s] Path: %s was rate limited\n", GetIpAddressFromRequest(r), r.URL.Path)
		http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
		return true
	}
	
	return false
}

// ThrottledResponseWriter wraps http.ResponseWriter with bandwidth limiting
type ThrottledResponseWriter struct {
	http.ResponseWriter
	kbps   int64
	bytesUntilSleep int64
	mu     sync.Mutex
	
	ticker *time.Ticker
}

func NewThrottledResponseWriter(w http.ResponseWriter, kbps int) *ThrottledResponseWriter {
	if kbps <= 0 {
		kbps = 1000
	}
	return &ThrottledResponseWriter{
		ResponseWriter: w,
		kbps: int64(kbps),
		
		ticker: time.NewTicker(50 * time.Millisecond),
	}
}

// Write applies bandwidth throttling
func (t *ThrottledResponseWriter) Write(ToWrite []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// bytes allowed per 50ms
	bytesPerTick := (t.kbps * 1024 * 50) / 1000
	totalWritten := 0
	
	for len(ToWrite) > 0 {
		AllowedCount := bytesPerTick
		if int64(len(ToWrite)) < AllowedCount {
			AllowedCount = int64(len(ToWrite))
		}
		
		n, err := t.ResponseWriter.Write(ToWrite[:AllowedCount])
		totalWritten += n
		ToWrite = ToWrite[n:]
		
		if err != nil {
			return totalWritten, err
		}
		
		t.bytesUntilSleep -= int64(n)
		
		if t.bytesUntilSleep <= 0 {
			t.bytesUntilSleep = bytesPerTick
			//time.Sleep(time.Millisecond * 50)
			
			<-t.ticker.C  // Wait for 50ms
		}
	}
	
	return totalWritten, nil
}

func (t *ThrottledResponseWriter) Close() {
	t.ticker.Stop()
}

func (t *ThrottledResponseWriter) Header() http.Header {
	return t.ResponseWriter.Header()
}
func (t *ThrottledResponseWriter) WriteHeader(statusCode int) {
	t.ResponseWriter.WriteHeader(statusCode)
}

func GetIpAddressFromRequest(r *http.Request) string {
	switch G_Config.IpStrategy {
	case IP_STRATEGY_CLOUDFLARE:
		if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
			return strings.TrimSpace(cfIP)
		}
	case IP_STRATEGY_REALIP:
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return strings.TrimSpace(realIP)
		}
	case IP_STRATEGY_FORWARDED:
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			// Split header and get the ip
			if parts := strings.Split(forwarded, ","); len(parts) > 0 {
				clientIP := strings.TrimSpace(parts[0])
				if clientIP != "" {
					return clientIP
				}
			}
		}
	case IP_STRATEGY_DIRECT:
		break
	default:
		L_Printf("Unknown IpStrategy: %s\n", G_Config.IpStrategy)
	}
	
	// IP_STRATEGY_DIRECT:
	
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}