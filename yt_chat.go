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
	YT_CHAT_UPDATE_MS = 1000
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

	client := &http.Client{}
	responsePage, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer responsePage.Body.Close()
	
	body, err := io.ReadAll(responsePage.Body)
	if err != nil {
		return "", err
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

func RetrieveNextContinuation(SegmentJson map[string]interface{}) (string, int) {
	liveChatContinuation, ok := RecursiveGetJson(SegmentJson, []string{"continuationContents", "liveChatContinuation"})
	if !ok {
		return "", 0
	}
	
	conts, ok := liveChatContinuation["continuations"].([]interface{})
	if !ok || len(conts) == 0 {
		return "", 0
	}
	c0, ok := conts[0].(map[string]interface{})
	if !ok {
		return "", 0
	}
	continuationType := 0
	continuationData, ok := c0["invalidationContinuationData"].(map[string]interface{})
	if !ok {
		// Check for timedContinuationData
		continuationData, ok = c0["timedContinuationData"].(map[string]interface{})
		if ok {
			continuationType = 1
		}
	}
	if ok {
		if cont, ok := continuationData["continuation"].(string); ok {
			return cont, continuationType
		}
	}
	
	return "", 0
}

func RetrieveNextTrackingParams(SegmentJson map[string]interface{}) string {
	liveChatContinuation, ok := RecursiveGetJson(SegmentJson, []string{"continuationContents", "liveChatContinuation"})
	if !ok {
		return ""
	}
	
	conts, ok := liveChatContinuation["continuations"].([]interface{})
	if !ok || len(conts) == 0 {
		return ""
	}
	c0, ok := conts[0].(map[string]interface{})
	if !ok {
		return ""
	}
	if rc, ok := c0["reloadContinuationData"].(map[string]interface{}); ok {
		if cont, ok := rc["clickTrackingParams"].(string); ok {
			return cont
		}
	}
	
	return ""
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
		action := Actions[i].(map[string]interface{})
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
	responsePage, err := http.Get(fmt.Sprintf("https://youtube.com/watch?v=%s", VideoId))
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
	
	ChatFile, err := os.OpenFile(OutputPath, os.O_RDWR, 0644)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		ChatFile, err = os.Create(OutputPath)
		if err != nil {
			CL_Logf(Task, "Failed to create file \"%s\" error: %v\n", OutputPath, err)
			return err
		}
	} else if err != nil {
		CL_Logf(Task, "Failed to open file \"%s\" error: %v\n", OutputPath, err)
		return err
	}
	defer ChatFile.Close()
	
	for {
		if !CL_IsRunning(DownloadTask) || !CL_IsRunning(Task) {
			continue
		}
		
		// TODO: Add support for replay chats
		segmentBody, err := DownloadChatSegment(nextContinuation, ThisChatContext)
		if err != nil {
			CL_Logf(Task, "An error occured when downloading segment %d: %v\n", ThisChatContext.SegmentId, err)
			CL_FinishTask(Task, TASK_STATUS_FAILED)
			break
		}
		
		segmentJson := map[string]interface{}{}
		err = json.Unmarshal([]byte(segmentBody), &segmentJson)
		if err != nil {
			CL_Logf(Task, "Segment returned malformed json text, err: %v\n", err)
			CL_FinishTask(Task, TASK_STATUS_FAILED)
			break
		}
		
		if ThisChatContext.SegmentState == 0 {
			// Reload chat in Live chat mode
			nextContinuation = RetrieveLiveChatModeContinuation(segmentJson)
			ThisChatContext.SegmentState = 1
			if nextContinuation != "" {
				CL_Logf(Task, "Loading 'Live chat' mode!\n")
				ThisChatContext.ReloadContinuation = nextContinuation
				ThisChatContext.SegmentId += 1
				continue
			}
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
		
		continuationType := 0
		
		nextContinuation, continuationType = RetrieveNextContinuation(segmentJson)
		ThisChatContext.ClickTrackingParams = RetrieveNextTrackingParams(segmentJson)
		ThisChatContext.ContinuationType = continuationType
		
		if nextContinuation == "" {
			CL_Logf(Task, "Continuation ID is empty, ending...\n")
			CL_FinishTask(Task, TASK_STATUS_FAILED)
			break
		}
		
		ThisChatContext.SegmentId += 1
		
		time.Sleep(time.Millisecond * YT_CHAT_UPDATE_MS)
	}
	
	if CL_IsRunning(DownloadTask) && Task.Status == TASK_STATUS_RUNNING {
		CL_FinishTask(Task, TASK_STATUS_FINISHED)
	}
	
	return nil
}
