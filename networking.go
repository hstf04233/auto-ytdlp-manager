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

const (
	RATE_LIMIT_KBPS_PER_IP  = 10_000
	RATE_LIMIT_KBPS_LOCALIP = 1e12
)

type RateLimitInfo struct {
	mutex sync.Mutex
	
	Ip string
	
	TotalRequests int
	ApiRequests   int
	LoginRequests int
}

type DynamicThrottledWriterInfo struct {
	mutex sync.Mutex
	
	Ip string
	
	TargetKBPS int
	
	RequestsCountBacklog int
	ActiveCount int
	Writers []*ThrottledResponseWriter
}

var NETWORKING_RATELIMIT_INFOS = NewCache(time.Second * 60)
var NETWORKING_DYNTHROTTLED_WRITERS = make(map[string]*DynamicThrottledWriterInfo)
var N_DT_W_MUTEX = sync.RWMutex{}

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
	mu      sync.Mutex
	kbps_mu sync.Mutex
	
	LastWriteTime time.Time
	
	DTWInfo *DynamicThrottledWriterInfo
	
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
		
		TargetKBPS: RATE_LIMIT_KBPS_PER_IP,
	}
	NETWORKING_DYNTHROTTLED_WRITERS[Ip] = NewInfo
	
	return NewInfo
}
func NewDynamicThrottledResponseWriter(w http.ResponseWriter, r *http.Request) *ThrottledResponseWriter {
	NewThrottleWriter := &ThrottledResponseWriter{
		ResponseWriter: w,
		kbps: 1000,
		
		ticker: time.NewTicker(50 * time.Millisecond),
	}
	
	DTWInfo := GetDynThrottledWriterInfo(r)
	DTWInfo.mutex.Lock()
	DTWInfo.ActiveCount += 1
	DTWInfo.RequestsCountBacklog += 1
	DTWInfo.Writers = append(DTWInfo.Writers, NewThrottleWriter)
	
	NewThrottleWriter.DTWInfo = DTWInfo
	
	//KBPS := RATE_LIMIT_KBPS_PER_IP
	NewThrottleWriter.kbps = int64(DTWInfo.TargetKBPS)
	
	DTWInfo.mutex.Unlock()
	
	return NewThrottleWriter
}

// Write applies bandwidth throttling
func (t *ThrottledResponseWriter) Write(ToWrite []byte) (int, error) {
	totalWritten := 0
	
	for len(ToWrite) > 0 {
		// bytes allowed per 50ms
		t.mu.Lock()
		
		t.kbps_mu.Lock()
		bytesPerTick := (t.kbps * 1024 * 50) / 1000
		t.kbps_mu.Unlock()
		
		AllowedCount := bytesPerTick
		if int64(len(ToWrite)) < AllowedCount {
			AllowedCount = int64(len(ToWrite))
		}
		
		t.LastWriteTime = time.Now()
		
		t.mu.Unlock()
		n, err := t.ResponseWriter.Write(ToWrite[:AllowedCount])
		totalWritten += n
		ToWrite = ToWrite[n:]
		
		if err != nil {
			return totalWritten, err
		}
		t.mu.Lock()
		
		if t.bytesUntilSleep > bytesPerTick {  // kbps could have changed!
			t.bytesUntilSleep = bytesPerTick
		}
		
		t.bytesUntilSleep -= int64(n)
		
		if t.bytesUntilSleep <= 0 {
			t.mu.Unlock()
			t.bytesUntilSleep = bytesPerTick
			
			<-t.ticker.C  // Wait for 50ms
		} else {
			t.mu.Unlock()
		}
	}
	
	return totalWritten, nil
}

func (t *ThrottledResponseWriter) Close() {
	t.ticker.Stop()
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
		DeleteTick := 0
		for {
			time.Sleep(time.Millisecond * 1000)
			DeleteTick -= 1
			
			N_DT_W_MUTEX.Lock()
			
			for Ip, DTWInfo := range(NETWORKING_DYNTHROTTLED_WRITERS) {
				DTWInfo.mutex.Lock()
				if DTWInfo.ActiveCount <= 0 && DTWInfo.RequestsCountBacklog <= 0 && DeleteTick <= 0 {
					DTWInfo.mutex.Unlock()
					// Delete this info.
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
				
				L_Printf("Set kbps: %d, Active: %d\n", KBPS, RealActiveCount)
				
				for _, DynWriter := range(DTWInfo.Writers) {
					DynWriter.kbps_mu.Lock()
					DynWriter.kbps = int64(KBPS)
					DynWriter.kbps_mu.Unlock()
				}
				
				if DTWInfo.RequestsCountBacklog > 0 {
					DTWInfo.RequestsCountBacklog = max(DTWInfo.RequestsCountBacklog - 40, 0)
					
				}
				
				DTWInfo.mutex.Unlock()
			}
			N_DT_W_MUTEX.Unlock()
			
			if DeleteTick < 0 {
				DeleteTick = 60
			}
		}
	}()
}
