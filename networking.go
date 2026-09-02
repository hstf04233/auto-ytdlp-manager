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
	RATE_LIMIT_BUCKET_CREATEACCOUNT = 3
)

const (
	MAX_GLOBAL_REQUESTS_PER_MINUTE = 7_500
	MAX_API_REQUESTS_PER_MINUTE    = 400
	MAX_LOGIN_REQUESTS_PER_MINUTE  = 15
	MAX_ACCOUNTCREATION_REQUESTS_PER_MINUTE = 5
)

const (
	RATE_LIMIT_KBPS_LOCALIP = int(1e12)
)

type RateLimitInfo struct {
	mutex sync.Mutex
	
	Ip string
	
	GlobalRequests int
	ApiRequests    int
	LoginRequests  int
	AccountCreationRequests int
}

type DynamicThrottledWriterInfo struct {
	mutex sync.Mutex
	
	Ip string
	
	TargetKBPS int
	
	
	RequestsCountBacklog int
	ActiveCount int
	Writers []*ThrottledResponseWriter
}

// The rate limit info automatically expires after 1 minute.
var NETWORKING_RATELIMIT_INFOS = NewCache(time.Second * 60)
var NETWORKING_DYNTHROTTLED_WRITERS = make(map[string]*DynamicThrottledWriterInfo)
var N_DT_W_MUTEX = sync.RWMutex{}

func GetRateLimitInfoFromRequest(r *http.Request) *RateLimitInfo {
	IpAddress := GetIpAddressFromRequest(r)
	
	NETWORKING_RATELIMIT_INFOS.mutex.Lock()
	defer NETWORKING_RATELIMIT_INFOS.mutex.Unlock()
	
	var RInfo *RateLimitInfo
	
	RateLimitCache, Exists := NETWORKING_RATELIMIT_INFOS.GetNoMutex(IpAddress)
	if Exists {
		RInfo = RateLimitCache.(*RateLimitInfo)
	} else {
		RInfo = &RateLimitInfo{
			Ip: IpAddress,
			
			GlobalRequests: 0,
			ApiRequests:   0,
			LoginRequests: 0,
			AccountCreationRequests:   0,
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
	
	if RInfo.GlobalRequests > MAX_GLOBAL_REQUESTS_PER_MINUTE {
		return true
	}
	
	switch Bucket {
	case RATE_LIMIT_BUCKET_GLOBAL:
		RInfo.GlobalRequests++
	case RATE_LIMIT_BUCKET_API:
		if RInfo.ApiRequests > MAX_API_REQUESTS_PER_MINUTE {
			return true
		}
		RInfo.ApiRequests++
	case RATE_LIMIT_BUCKET_LOGIN:
		if RInfo.LoginRequests > MAX_LOGIN_REQUESTS_PER_MINUTE {
			return true
		}
		RInfo.LoginRequests++
	case RATE_LIMIT_BUCKET_CREATEACCOUNT:
		if RInfo.AccountCreationRequests > MAX_ACCOUNTCREATION_REQUESTS_PER_MINUTE {
			return true
		}
		RInfo.AccountCreationRequests++
	default:
		break
	}
	
	return false
}

// Returns true if rate limited.
func RateLimitRequest(w http.ResponseWriter, r *http.Request, Bucket int) bool {
	// Handle rate limit errors automatically.
	if TestRateLimitForRequest(w, r, Bucket) {
		L_Printf("Request from \"%s\" Path: \"%s\" was rate limited\n", GetIpAddressFromRequest(r), r.URL.Path)
		http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
		return true
	}
	
	return false
}

// ThrottledResponseWriter wraps http.ResponseWriter with bandwidth limiting
type ThrottledResponseWriter struct {
	http.ResponseWriter
	kbps   int64
	BytesUntilSleep int64
	mu      sync.Mutex
	kbps_mu sync.Mutex
	
	LastWriteTime time.Time
	
	DTWInfo *DynamicThrottledWriterInfo
}

func NewThrottledResponseWriter(w http.ResponseWriter, kbps int) *ThrottledResponseWriter {
	if kbps <= 0 {
		kbps = 1000
	}
	return &ThrottledResponseWriter{
		ResponseWriter: w,
		kbps: int64(kbps),
	}
}

func GetDynThrottledWriterInfo(r *http.Request) *DynamicThrottledWriterInfo {
	N_DT_W_MUTEX.Lock()
	defer N_DT_W_MUTEX.Unlock()
	
	Ip := GetIpAddressFromRequest(r)
	
	Info, Exists := NETWORKING_DYNTHROTTLED_WRITERS[Ip]
	if Exists {
		return Info
	}
	
	NewInfo := &DynamicThrottledWriterInfo{
		Ip: Ip,
		
		TargetKBPS: Get_RateLimitKBPSPerPublicIp(G_Config),
	}
	NETWORKING_DYNTHROTTLED_WRITERS[Ip] = NewInfo
	
	return NewInfo
}
func NewDynamicThrottledResponseWriter(w http.ResponseWriter, r *http.Request) *ThrottledResponseWriter {
	NewThrottleWriter := &ThrottledResponseWriter{
		ResponseWriter: w,
		kbps: 1000,
	}
	
	DTWInfo := GetDynThrottledWriterInfo(r)
	DTWInfo.mutex.Lock()
	defer DTWInfo.mutex.Unlock()
	
	DTWInfo.ActiveCount += 1
	DTWInfo.RequestsCountBacklog += 1
	DTWInfo.Writers = append(DTWInfo.Writers, NewThrottleWriter)
	
	NewThrottleWriter.DTWInfo = DTWInfo
	
	//KBPS := RATE_LIMIT_KBPS_PER_IP
	NewThrottleWriter.kbps = int64(DTWInfo.TargetKBPS)
	
	// Set bytesUntilSleep immediately so there isn't a 50ms delay when something writes to it for the first time.
	NewThrottleWriter.BytesUntilSleep = (NewThrottleWriter.kbps * 1024 * 50) / 1000
	
	return NewThrottleWriter
}

// Write applies bandwidth throttling
func (t *ThrottledResponseWriter) Write(ToWrite []byte) (int, error) {
	totalWritten := 0
	
	for len(ToWrite) > 0 {
		t.mu.Lock()
		
		t.kbps_mu.Lock()
		bytesPerTick := (t.kbps * 1024 * 50) / 1000
		t.kbps_mu.Unlock()
		
		AllowedCount := bytesPerTick
		if int64(len(ToWrite)) < AllowedCount {
			AllowedCount = int64(len(ToWrite))
		}
		
		t.LastWriteTime = time.Now()
		
		n, err := t.ResponseWriter.Write(ToWrite[:AllowedCount])
		totalWritten += n
		ToWrite = ToWrite[n:]
		
		if err != nil {
			t.mu.Unlock()
			return totalWritten, err
		}
		
		if t.BytesUntilSleep > bytesPerTick {  // kbps could have changed!
			t.BytesUntilSleep = bytesPerTick
		}
		
		t.BytesUntilSleep -= int64(n)
		
		if t.BytesUntilSleep <= 0 {
			t.BytesUntilSleep += bytesPerTick
			t.mu.Unlock()
			
			time.Sleep(50 * time.Millisecond)
		} else {
			t.mu.Unlock()
		}
	}
	
	return totalWritten, nil
}

func (t *ThrottledResponseWriter) Close() {
	if t.DTWInfo != nil {
		t.DTWInfo.mutex.Lock()
		t.DTWInfo.ActiveCount -= 1
		for index, Writer := range(t.DTWInfo.Writers) {
			if Writer == t {
				t.DTWInfo.Writers = append(t.DTWInfo.Writers[:index], t.DTWInfo.Writers[index+1:]...)
				break
			}
		}
		t.DTWInfo.mutex.Unlock()
	}
}

func (t *ThrottledResponseWriter) Header() http.Header {
	return t.ResponseWriter.Header()
}
func (t *ThrottledResponseWriter) WriteHeader(statusCode int) {
	t.ResponseWriter.WriteHeader(statusCode)
}

func GetIpAddressFromRequest(r *http.Request) string {
	IpStrategy := G_Config.IpStrategy
	switch IpStrategy {
	case IP_STRATEGY_CLOUDFLARE:
		if CloudflareIp := r.Header.Get("CF-Connecting-IP"); CloudflareIp != "" {
			return strings.TrimSpace(CloudflareIp)
		}
	case IP_STRATEGY_REALIP:
		if RealIP := r.Header.Get("X-Real-IP"); RealIP != "" {
			return strings.TrimSpace(RealIP)
		}
	case IP_STRATEGY_FORWARDED:
		if Forwarded := r.Header.Get("X-Forwarded-For"); Forwarded != "" {
			// Split header and get the first ip
			if parts := strings.Split(Forwarded, ","); len(parts) > 0 {
				clientIP := strings.TrimSpace(parts[0])
				if clientIP != "" {
					return clientIP
				}
			}
		}
	case IP_STRATEGY_DIRECT:
		break
	default:
		if strings.HasPrefix(IpStrategy, "HEADER:") {
			CustomHeaderName := strings.TrimPrefix(IpStrategy, "HEADER:")
			if CustomHeaderIp := r.Header.Get(CustomHeaderName); CustomHeaderIp != "" {
				return strings.TrimSpace(CustomHeaderIp)
			}
		} else {
			L_Printf("Unknown IpStrategy: %s\n", IpStrategy)
		}
	}
	
	// Localhost or IP_STRATEGY_DIRECT:
	
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	
	if host == "::1" {
		host = "127.0.0.1"
	}
	
	return host
}

func IsIpAddressLocal(Ip string) bool {
	// Chat gpt vibed ts up... Hopefully it's correct 😭
	ParsedIp := net.ParseIP(Ip)
	if ParsedIp == nil {
		return false
	}
	
	// Loopback addresses (127.0.0.0/8, ::1)
	if ParsedIp.IsLoopback() {
		return true
	}
	
	// RFC1918 private IPv4 ranges and IPv6 Unique Local Addresses (fc00::/7)
	if ParsedIp.IsPrivate() {
		return true
	}
	
	// Link-local unicast/multicast
	if ParsedIp.IsLinkLocalUnicast() || ParsedIp.IsLinkLocalMulticast() {
		return true
	}
	
	return false
}

func init() {
	go func() {
		CleanUpTick := 0
		for {
			time.Sleep(time.Millisecond * 1000)
			CleanUpTick -= 1
			if CleanUpTick <= 0 {
				NETWORKING_RATELIMIT_INFOS.CleanUp()
			}
			
			N_DT_W_MUTEX.Lock()
			
			RATE_LIMIT_KBPS_PER_IP := Get_RateLimitKBPSPerPublicIp(G_Config)
			
			for Ip, DTWInfo := range(NETWORKING_DYNTHROTTLED_WRITERS) {
				DTWInfo.mutex.Lock()
				if DTWInfo.ActiveCount <= 0 && DTWInfo.RequestsCountBacklog <= 0 && CleanUpTick <= 0 {
					DTWInfo.mutex.Unlock()
					// This info has expired! Delete it.
					delete(NETWORKING_DYNTHROTTLED_WRITERS, Ip)
					continue
				}
				
				KBPS := RATE_LIMIT_KBPS_PER_IP
				if IsIpAddressLocal(DTWInfo.Ip) {
					KBPS = RATE_LIMIT_KBPS_LOCALIP
				}
				
				RealActiveCount := 0 + (DTWInfo.RequestsCountBacklog / 2)
				if DTWInfo.ActiveCount >= 1 || RealActiveCount > 0 {
					TimeNowMS := time.Now().UnixMilli()
					for _, DynWriter := range(DTWInfo.Writers) {
						DynWriter.mu.Lock()
						if DynWriter.LastWriteTime.UnixMilli()+990 > TimeNowMS {
							RealActiveCount += 1
						}
						DynWriter.mu.Unlock()
					}
					KBPS = KBPS / max(RealActiveCount, 1)
					if KBPS <= 100 {
						KBPS = 100
					}
				}
				
				DTWInfo.TargetKBPS = KBPS
				
				//L_Printf("Set kbps: %d, Active: %d\n", KBPS, RealActiveCount)
				
				for _, DynWriter := range(DTWInfo.Writers) {
					DynWriter.kbps_mu.Lock()
					DynWriter.kbps = int64(KBPS)
					DynWriter.kbps_mu.Unlock()
				}
				
				if DTWInfo.RequestsCountBacklog > 0 {
					DTWInfo.RequestsCountBacklog = max(DTWInfo.RequestsCountBacklog - 20, 0)
					
				}
				
				DTWInfo.mutex.Unlock()
			}
			N_DT_W_MUTEX.Unlock()
			
			if CleanUpTick < 0 {
				CleanUpTick = 60
			}
		}
	}()
}
