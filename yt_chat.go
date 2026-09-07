package main

import (
	"fmt"
	"net/http"
	"os"
	"io"
	"encoding/json"
	"errors"
	"bytes"
	"strings"
	"regexp"
	"time"
)

const (
	YT_CHAT_DEBUG_HTML = true
	YT_CHAT_UPDATE_MS = 1500

	// Resilience tuning for long live streams. A single transient failure
	// (network blip, HTTP 429/500, rotated continuation type) must not kill
	// a multi-hour download. Pacing intentionally stays at the fixed
	// YT_CHAT_UPDATE_MS interval (following server timeoutMs loses chats).
	YT_CHAT_SEGMENT_TIMEOUT_SEC = 15
	YT_CHAT_RETRY_SLEEP_MS = 3000
	YT_CHAT_REFRESH_SLEEP_MS = 5000
	YT_CHAT_MAX_CONSECUTIVE_ERRORS = 20
	YT_CHAT_MAX_CONSECUTIVE_EMPTY = 15
	YT_CHAT_MAX_REFRESHES = 10
	YT_CHAT_LOG_SNIPPET_LEN = 1000
)

type YTChatContext struct {
	VideoId  string
	VideoUrl string
	
	ReloadContinuation string
	
	ContinuationType int
	SegmentState int
	SegmentId int
	ChatActions []interface{}
	
	INNERTUBE_CONTEXT string
	INNERTUBE_API_KEY string
	ClickTrackingParams string
	
	LastPublishUsec int64
}

func BasicWriteFile(FilePath string, FileContent string) error {
	file, err := os.Create(FilePath)
	if err != nil {
		L_Printf("Failed to create '%s': %v\n", FilePath, err)
		return err
	}
	defer file.Close()
	
	_, err = io.WriteString(file, FileContent)
	if err != nil {
		L_Printf("Failed to write file: %v\n", err)
		return err
	}
	
	return nil
}

func GetStartReloadContinuation(htmlBody string) string {
	var reloadContinuation string
	
	continuationGRegex := regexp.MustCompile(`"liveChatRenderer":\{"continuations":\[\{"reloadContinuationData":\{"continuation":"([^"]+)"`)
	
	matches := continuationGRegex.FindStringSubmatch(htmlBody)
	if len(matches) > 0 {
		reloadContinuation = matches[1]
	}
	
	return reloadContinuation
}

func YTChat_TruncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "... (truncated)"
}

func DownloadChatSegment(Continuation string, ThisChatContext *YTChatContext) (string, error) {
	url := fmt.Sprintf("https://www.youtube.com/youtubei/v1/live_chat/get_live_chat?key=%s&prettyPrint=false", ThisChatContext.INNERTUBE_API_KEY)
	
	IntertubeContextJson := map[string]interface{}{}
	
	err := json.Unmarshal([]byte(ThisChatContext.INNERTUBE_CONTEXT), &IntertubeContextJson)
	if err != nil {
		return "", err
	}
	
	if ThisChatContext.ClickTrackingParams != "" {
		IntertubeContextJson["clickTracking"] = map[string]interface{}{"clickTrackingParams": ThisChatContext.ClickTrackingParams,}
	}
	
	payload := map[string]interface{}{
		"context": IntertubeContextJson,
		"continuation": Continuation,
	}
	
	if ThisChatContext.SegmentState == 1 {
		if ThisChatContext.ContinuationType == 0 {
			payload["invalidationPayloadLastPublishAtUsec"] = fmt.Sprintf("%d", ThisChatContext.LastPublishUsec)
		}
	}
	jsonData, _ := json.Marshal(payload)
	//L_Printf("%s\n", jsonData)
	
	if ThisChatContext.ContinuationType == 0 {
		ThisChatContext.LastPublishUsec = time.Now().UnixMicro()
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: time.Second * YT_CHAT_SEGMENT_TIMEOUT_SEC}
	responsePage, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer responsePage.Body.Close()

	body, err := io.ReadAll(responsePage.Body)
	if err != nil {
		return "", err
	}

	if responsePage.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", responsePage.StatusCode, YTChat_TruncateForLog(string(body), YT_CHAT_LOG_SNIPPET_LEN))
	}

	return string(body), nil
}

func RecursiveGetJson(Json map[string]interface{}, Indexes []string) (map[string]interface{}, bool) {
	current := Json
	
	for i:=0; i < len(Indexes); i++ {
		Index := Indexes[i]
		
		newest, ok := current[Index].(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = newest
	}
	
	return current, true
}

func RetrieveLiveChatModeContinuation(SegmentJson map[string]interface{}) string {
	sortFilterSubMenuRenderer, ok := RecursiveGetJson(SegmentJson, []string{
		"continuationContents", "liveChatContinuation", "header", "liveChatHeaderRenderer", "viewSelector", "sortFilterSubMenuRenderer",
	})
	if !ok {
		L_Printf("[Live chat mode] Couldn't find 'sortFilterSubMenuRenderer'\n")
		return ""
	}
	
	subMenuItems, ok := sortFilterSubMenuRenderer["subMenuItems"].([]interface{})
	if !ok || len(subMenuItems) <= 1 {
		L_Printf("[Live chat mode] Couldn't find 'subMenuItems' or empty.\n")
		return ""
	}
	
	// Live chat mode is usually the 2nd sub item.
	liveChatSub, ok := subMenuItems[1].(map[string]interface{})
	if !ok {
		L_Printf("[Live chat mode] Couldn't find live chat item in 'subMenuItems'\n")
		return ""
	}
	
	reloadContinuationData, ok := RecursiveGetJson(liveChatSub, []string{"continuation", "reloadContinuationData"})
	if !ok {
		L_Printf("[Live chat mode] Couldn't find 'reloadContinuationData'\n")
		return ""
	}
	
	if cont, ok := reloadContinuationData["continuation"].(string); ok {
		return cont
	}
	
	return ""
}

func YTChat_ParseTimeoutMs(data map[string]interface{}) int64 {
	v, ok := data["timeoutMs"]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return t
	case int32:
		return int64(t)
	case string:
		var n int64
		_, _ = fmt.Sscanf(t, "%d", &n)
		return n
	case json.Number:
		n, _ := t.Int64()
		return n
	}
	return 0
}

// RetrieveNextContinuation scans ALL entries in continuations[] for every
// known live-chat continuation type. The old code only looked at
// continuations[0] for invalidation/timed data, so any rotation by YouTube
// (reload, playerSeek, replay, reordered array) looked like end-of-chat.
// Returns: continuation, continuationType, clickTrackingParams, timeoutMs.
// continuationType 0 = invalidation-style (send invalidationPayload...),
// everything else = 1+ (do not send it). timeoutMs is parsed for logging
// only; pacing intentionally stays at fixed YT_CHAT_UPDATE_MS.
func RetrieveNextContinuation(SegmentJson map[string]interface{}) (string, int, string, int64) {
	liveChatContinuation, ok := RecursiveGetJson(SegmentJson, []string{"continuationContents", "liveChatContinuation"})
	if !ok {
		return "", 0, "", 0
	}

	conts, ok := liveChatContinuation["continuations"].([]interface{})
	if !ok || len(conts) == 0 {
		return "", 0, "", 0
	}

	priority := []struct {
		key string
		typ int
	}{
		{"invalidationContinuationData", 0},
		{"timedContinuationData", 1},
		{"liveChatReplayContinuationData", 4},
		{"playerSeekContinuationData", 3},
		{"reloadContinuationData", 2},
	}

	for _, p := range(priority) {
		for _, c := range(conts) {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			data, ok := cm[p.key].(map[string]interface{})
			if !ok {
				continue
			}
			cont, _ := data["continuation"].(string)
			if cont == "" {
				continue
			}
			tracking, _ := data["clickTrackingParams"].(string)
			return cont, p.typ, tracking, YTChat_ParseTimeoutMs(data)
		}
	}

	return "", 0, "", 0
}

func RetrieveNextTrackingParams(SegmentJson map[string]interface{}) string {
	_, _, tracking, _ := RetrieveNextContinuation(SegmentJson)
	return tracking
}

func GetActions(SegmentJson map[string]interface{}) ([]interface{}, bool) {
	liveChatContinuation, ok := RecursiveGetJson(SegmentJson, []string{"continuationContents", "liveChatContinuation"})
	if !ok {
		return nil, false
	}
	
	actions, ok := liveChatContinuation["actions"].([]interface{})
	if !ok {
		return nil, false
	}
	
	return actions, true
}

func WriteActions(Actions []interface{}, ChatFile *os.File) error {
	var actionsBuf strings.Builder
	
	for i:=0; i < len(Actions); i++ {
		action, ok := Actions[i].(map[string]interface{})
		if !ok {
			continue
		}
		delete(action, "clickTrackingParams")
		actionStr, err := json.Marshal(action)
		if err != nil {
			return err
		}
		
		actionsBuf.Write(actionStr)
		actionsBuf.WriteString("\n")
	}
	
	_, err := ChatFile.WriteString(actionsBuf.String())
	if err != nil {
		return err
	}
	
	return nil
}

func YTC_DownloadYTWebpage(VideoUrl string, VideoId string) (*YTChatContext, error) {
	client := &http.Client{Timeout: time.Second * YT_CHAT_SEGMENT_TIMEOUT_SEC}
	responsePage, err := client.Get(fmt.Sprintf("https://youtube.com/watch?v=%s", VideoId))
	if err != nil {
		//L_Printf("Failed to download page: %v\n", err)
		return nil, err
	}
	defer responsePage.Body.Close()
	
	body, err := io.ReadAll(responsePage.Body)
	if err != nil {
		//L_Printf("An error occured when reading page: %v\n", err)
		return nil, err
	}
	
	bodyStr := string(body)
	
	if YT_CHAT_DEBUG_HTML == true {
		dfile, err := os.Create("output.html")
		if err != nil {
			//L_Printf("Failed to create output.html: %v\n", err)
			return nil, err
		}
		defer dfile.Close()
		
		_, err = io.WriteString(dfile, bodyStr)
		if err != nil {
			//L_Printf("Failed to write file: %v\n", err)
			return nil, err
		}
	}
	
	reloadContinuation := GetStartReloadContinuation(bodyStr)
	if len(reloadContinuation) <= 0 {
		//L_Printf("reload continuation id not found...\n")
		return nil, errors.New("Reload continuation id not found")
	}
	
	itcReg := regexp.MustCompile(`"INNERTUBE_CONTEXT":(.*?),"INNERTUBE_CONTEXT_CLIENT_NAME"`)
	contextMatches := itcReg.FindStringSubmatch(bodyStr)
	if len(contextMatches) <= 0 {
		//L_Printf("Could not find INNERTUBE_CONTEXT data...\n")
		return nil, errors.New("Could not find INNERTUBE_CONTEXT data...")
	}
	INNERTUBE_CONTEXT := contextMatches[1]
	
	itkReg := regexp.MustCompile(`"INNERTUBE_API_KEY":"([^"]+)"`)
	apiKeyMatches := itkReg.FindStringSubmatch(bodyStr)
	if len(apiKeyMatches) <= 0 {
		L_Printf("Could not find INNERTUBE_API_KEY...\n")
		return nil, errors.New("Could not find INNERTUBE_API_KEY...")
	}
	INNERTUBE_API_KEY := apiKeyMatches[1]
	
	ThisChatContext := &YTChatContext{
		VideoId:  VideoId,
		VideoUrl: VideoUrl,
		
		ReloadContinuation: reloadContinuation,
		
		SegmentState: 0,
		SegmentId:    0,
		
		INNERTUBE_CONTEXT: INNERTUBE_CONTEXT,
		INNERTUBE_API_KEY: INNERTUBE_API_KEY,
		
		ClickTrackingParams: "",
	}
	
	return ThisChatContext, nil
}

func yt_chat_Run(VideoUrl string, OutputPath string, DownloadTask *CommandTask) error {
	if VideoUrl == "" {
		L_Printf("No url inputed, exiting.")
		return nil
	}
	
	Task := CL_NewGenericTask()
	Task.Type = TASK_TYPE_GENERIC
	Task.FromVideoId = DownloadTask.FromVideoId
	Task.FromChannelId = DownloadTask.FromChannelId
	Task.Title = fmt.Sprintf("Live chat download: \"%s\"", VideoUrl)
	DB_UpdateCommandTaskInfo(Task)
	
	defer func() {
		if Task.Status == TASK_STATUS_RUNNING {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
	}()
	
	CL_Logf(Task, "Downloading yt live chat: %s\n", VideoUrl)
	
	ytIdRegex := regexp.MustCompile(`(?:v=|\/watch\/|\/live\/|\/shorts\/|youtu\.be\/)([a-zA-Z0-9_-]{11})`)
	
	matches := ytIdRegex.FindStringSubmatch(VideoUrl)
	if len(matches) <= 0 {
		CL_Logf(Task, "Invalid YouTube url: %s\n", VideoUrl)
		return nil
	}
	videoId := matches[1]
	CL_Logf(Task, "VideoId: %s\n\n", videoId)
	
	ThisChatContext, err := YTC_DownloadYTWebpage(VideoUrl, videoId)
	if err != nil {
		CL_Logf(Task, "Failed to retrieve webpage, error: %v\n", err)
		return err
	}
	
	nextContinuation := ThisChatContext.ReloadContinuation

	ChatFile, err := os.OpenFile(OutputPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		CL_Logf(Task, "Failed to open file \"%s\" error: %v\n", OutputPath, err)
		return err
	}
	defer ChatFile.Close()
	// Ensure appends go at the end even if the file already existed.
	_, _ = ChatFile.Seek(0, io.SeekEnd)

	consecutiveErrors := 0
	consecutiveEmpty := 0
	refreshAttempts := 0

	for {
		if !CL_IsRunning(DownloadTask) || !CL_IsRunning(Task) {
			break
		}

		// TODO: Add support for replay chats
		segmentBody, err := DownloadChatSegment(nextContinuation, ThisChatContext)
		if err != nil {
			consecutiveErrors += 1
			CL_Logf(Task, "Error downloading chat segment %d (attempt %d/%d): %v\n", ThisChatContext.SegmentId, consecutiveErrors, YT_CHAT_MAX_CONSECUTIVE_ERRORS, err)
			if consecutiveErrors > YT_CHAT_MAX_CONSECUTIVE_ERRORS {
				CL_Logf(Task, "Too many consecutive segment errors, ending...\n")
				if CL_IsRunning(DownloadTask) {
					CL_FinishTask(Task, TASK_STATUS_FAILED)
				} else {
					CL_FinishTask(Task, TASK_STATUS_FINISHED)
				}
				break
			}
			time.Sleep(time.Millisecond * YT_CHAT_RETRY_SLEEP_MS)
			continue
		}

		segmentJson := map[string]interface{}{}
		err = json.Unmarshal([]byte(segmentBody), &segmentJson)
		if err != nil {
			consecutiveErrors += 1
			CL_Logf(Task, "Segment %d returned malformed json (attempt %d/%d), err: %v. Snippet: %s\n", ThisChatContext.SegmentId, consecutiveErrors, YT_CHAT_MAX_CONSECUTIVE_ERRORS, err, YTChat_TruncateForLog(segmentBody, YT_CHAT_LOG_SNIPPET_LEN))
			if consecutiveErrors > YT_CHAT_MAX_CONSECUTIVE_ERRORS {
				CL_Logf(Task, "Too many consecutive malformed segments, ending...\n")
				if CL_IsRunning(DownloadTask) {
					CL_FinishTask(Task, TASK_STATUS_FAILED)
				} else {
					CL_FinishTask(Task, TASK_STATUS_FINISHED)
				}
				break
			}
			time.Sleep(time.Millisecond * YT_CHAT_RETRY_SLEEP_MS)
			continue
		}
		consecutiveErrors = 0

		if ThisChatContext.SegmentState == 0 {
			// Reload chat in Live chat mode
			liveModeContinuation := RetrieveLiveChatModeContinuation(segmentJson)
			ThisChatContext.SegmentState = 1
			if liveModeContinuation != "" {
				CL_Logf(Task, "Switching to 'Live chat' mode\n")
				ThisChatContext.ReloadContinuation = liveModeContinuation
				nextContinuation = liveModeContinuation
				consecutiveEmpty = 0
				ThisChatContext.SegmentId += 1
				continue
			}
			// Fall through and try normal continuation parsing below.
		}

		actions, actionsExist := GetActions(segmentJson)
		if actionsExist {
			err := WriteActions(actions, ChatFile)
			if err != nil {
				CL_Logf(Task, "Cannot write to file err: %v, ending...\n", err)
				CL_FinishTask(Task, TASK_STATUS_FAILED)
				break
			}
		}

		nextCont, continuationType, trackingParams, _ := RetrieveNextContinuation(segmentJson)
		// Only overwrite tracking params when the server actually sent new
		// ones; the old code wiped them to "" on almost every segment.
		if trackingParams != "" {
			ThisChatContext.ClickTrackingParams = trackingParams
		}
		ThisChatContext.ContinuationType = continuationType

		if nextCont == "" {
			consecutiveEmpty += 1
			CL_Logf(Task, "Empty continuation on segment %d (attempt %d/%d). Snippet: %s\n", ThisChatContext.SegmentId, consecutiveEmpty, YT_CHAT_MAX_CONSECUTIVE_EMPTY, YTChat_TruncateForLog(segmentBody, YT_CHAT_LOG_SNIPPET_LEN))
			if consecutiveEmpty >= YT_CHAT_MAX_CONSECUTIVE_EMPTY {
				if CL_IsRunning(DownloadTask) && refreshAttempts < YT_CHAT_MAX_REFRESHES {
					refreshAttempts += 1
					CL_Logf(Task, "Attempting to refresh continuation via webpage (%d/%d)...\n", refreshAttempts, YT_CHAT_MAX_REFRESHES)
					fresh, ferr := YTC_DownloadYTWebpage(VideoUrl, videoId)
					if ferr == nil && fresh.ReloadContinuation != "" {
						ThisChatContext.INNERTUBE_CONTEXT = fresh.INNERTUBE_CONTEXT
						ThisChatContext.INNERTUBE_API_KEY = fresh.INNERTUBE_API_KEY
						ThisChatContext.ReloadContinuation = fresh.ReloadContinuation
						nextContinuation = fresh.ReloadContinuation
						ThisChatContext.SegmentState = 0
						consecutiveEmpty = 0
						time.Sleep(time.Millisecond * YT_CHAT_REFRESH_SLEEP_MS)
						continue
					}
					CL_Logf(Task, "Continuation refresh failed: %v\n", ferr)
				}
				if consecutiveEmpty >= YT_CHAT_MAX_CONSECUTIVE_EMPTY && !(CL_IsRunning(DownloadTask) && refreshAttempts < YT_CHAT_MAX_REFRESHES) {
					CL_Logf(Task, "Continuation ID is empty, ending...\n")
					if CL_IsRunning(DownloadTask) {
						CL_FinishTask(Task, TASK_STATUS_FAILED)
					} else {
						// Stream ended; chat ending here is normal.
						CL_FinishTask(Task, TASK_STATUS_FINISHED)
					}
					break
				}
			}
			// Transient: retry the same continuation.
			time.Sleep(time.Millisecond * YT_CHAT_RETRY_SLEEP_MS)
			continue
		}

		nextContinuation = nextCont
		consecutiveEmpty = 0
		refreshAttempts = 0

		ThisChatContext.SegmentId += 1
		if ThisChatContext.SegmentId%10 == 0 {
			_ = ChatFile.Sync()
		}

		// NOTE: fixed interval on purpose; server timeoutMs is ignored
		// because following it has been observed to miss chats.
		time.Sleep(time.Millisecond * YT_CHAT_UPDATE_MS)
	}
	
	if Task.Status == TASK_STATUS_RUNNING {
		// Clean loop exit (live download ended/cancelled) with no explicit
		// failure above means the chat file is complete as far as we got.
		CL_FinishTask(Task, TASK_STATUS_FINISHED)
	}
	
	return nil
}
