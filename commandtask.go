package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	MAX_TASK_OUTPUT_LOG = (1 << 16)
)

const (
	TASK_TYPE_GENERIC  = 0
	TASK_TYPE_LISTING  = 1
	TASK_TYPE_DOWNLOAD = 2
)

const (
	TASK_STATUS_RUNNING  = 0
	TASK_STATUS_FAILED   = 1
	TASK_STATUS_FINISHED = 2
	TASK_STATUS_CANCELED = 3
)

type TaskChannelInfo struct {
	Name string `json:"name"`
	Url  string `json:"url"`
	Id   string `json:"id"`
}

type TaskVideoInfo struct {
	Title string `json:"title"`
	Url   string `json:"url"`
	Id    string `json:"id"`
}

type CommandTask struct {
	Lock   *sync.RWMutex `json:"-"`
	
	Id     string `json:"id"`
	Title  string `json:"title"`
	Type   int    `json:"type"`
	Status int    `json:"status"`
	Cmd    *exec.Cmd `json:"-"`
	
	FromChannelId string `json:"from_channel"`
	ChannelInfo   *TaskChannelInfo `json:"basic_channel_info"`
	FromVideoId   string `json:"from_video"`
	VideoInfo     *TaskVideoInfo `json:"basic_video_info"`
	
	RunArgs string `json:"run_args"`
	
	// TODO: These are a string for now because I just want something working... this could be slow
	Output         string `json:"output"`
	RealtimeOutput string `json:"-"`
	
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	UpdatedAt time.Time `json:"updated_at"`
}

var ARCT_Lock sync.RWMutex
var AllRunningCommandTasks map[string]*CommandTask

func TruncateOutput(Output string) string {
	if len(Output) > MAX_TASK_OUTPUT_LOG-30 {
		Output = fmt.Sprintf("[%d CHARACTERS TRUNCATED]\n", len(Output) - (MAX_TASK_OUTPUT_LOG-30)) +
				 Output[len(Output)-(MAX_TASK_OUTPUT_LOG-30):]
		
		return Output
	}
	
	return Output
}

func watchStd(std io.ReadCloser, buf *[]byte, nout *int, Mutex *sync.RWMutex) {
	var n int
	var err error
	ErrorCount := 0
	for {
		Mutex.Lock()
		nout_val := *nout
		Mutex.Unlock()
		if nout_val == -1 {
			n, err = std.Read(*buf)
		} else {
			// Wait for the other thread to read the output before continuing!
			time.Sleep(16 * time.Millisecond)
			continue
		}
		if n > 0 {
			Mutex.Lock()
			*nout = n
			Mutex.Unlock()
		}
		if err == io.EOF || errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		} else if err != nil {
			ErrorCount++
			if ErrorCount > 1000 {
				// This pipe must be broken then?
				L_Printf("Pipe error wat | err: %v\n", err)
				break
			}
			time.Sleep(16 * time.Millisecond)
			continue
		} else {
			ErrorCount = 0
		}
	}
	//L_Printf("This pipe closed!\n")
}

func monitorTaskCmdOutput(Task *CommandTask, stdout io.ReadCloser, stderr io.ReadCloser) {
	bufout := make([]byte, 4096)
	c_out := -1
	buferr := make([]byte, 4096)
	c_err := -1
	State := 1
	Mutex := &sync.RWMutex{}
	
	go watchStd(stdout, &bufout, &c_out, Mutex)
	go watchStd(stderr, &buferr, &c_err, Mutex)
	
	defer stdout.Close()
	defer stderr.Close()
	
	var buf *[]byte
	
	// TODO: Rewrite this so it doesn't use strings. Currently this creates a ton of garbage strings.
	
	CarriageReturn := false
	EndWait := time.Now().UnixMilli()
	
	for {
		n := 0
		Mutex.Lock()
		_c_out := c_out
		_c_err := c_err
		if State == 0 {
			// Read stdout
			buf = &bufout
			n = c_out
			c_out = -1
			if c_err > 0 {
				State = 1
			}
		} else {
			// Read stderr
			buf = &buferr
			n = c_err
			c_err = -1
			if c_out > 0 {
				State = 0
			}
		}
		
		if n > 0 {
			Task.Lock.Lock()
			
			chunk := (*buf)[:n]
			inEscapeSequence := 0
			var writeLine bytes.Buffer
			for _, b := range(chunk) {
				if inEscapeSequence != 0 {
					inEscapeSequence += 1
					if b == byte('K') {
						// Clear the line!
						VisibleOutput_Lines := strings.Split(Task.RealtimeOutput, "\n")
						if len(VisibleOutput_Lines) >= 2 {
							Task.RealtimeOutput = strings.Join(VisibleOutput_Lines[0:len(VisibleOutput_Lines)-1], "\n") + "\n" + writeLine.String()
						} else {
							Task.RealtimeOutput = writeLine.String()
						}
						writeLine.Reset()
					}
					if inEscapeSequence >= 3 {
						inEscapeSequence = 0
					}
					continue
				}
				if b == byte('\x1b') {
					inEscapeSequence = 1
					continue
				}
				
				writeLine.WriteByte(b)
				
				if b == byte('\n') {
					CarriageReturn = false
					Task.RealtimeOutput += writeLine.String()
					writeLine.Reset()
				} else {
					if CarriageReturn {
						CarriageReturn = false
						VisibleOutput_Lines := strings.Split(Task.RealtimeOutput, "\n")
						if len(VisibleOutput_Lines) >= 2 {
							Task.RealtimeOutput = strings.Join(VisibleOutput_Lines[0:len(VisibleOutput_Lines)-1], "\n") + "\n" + string(b)
						} else {
							Task.RealtimeOutput = "\n" + string(b)
						}
						writeLine.Reset()
					}
				}
				if b == byte('\r') {
					CarriageReturn = true
				}
			}
			Task.RealtimeOutput += writeLine.String()
			
			Task.Lock.Unlock()
			
			_c_out = c_out
			_c_err = c_err
			Mutex.Unlock()
		} else {
			Mutex.Unlock()
			time.Sleep(16 * time.Millisecond)
			_c_out = c_out
			_c_err = c_err
		}
		if Task.Status != TASK_STATUS_RUNNING && _c_err <= 0 && _c_out <= 0 {
			// Wait a bit for the output to fully flush out.
			if time.Now().UnixMilli() > EndWait {
				break
			}
		} else {
			EndWait = time.Now().UnixMilli() + 500
		}
	}
	
	Task.Lock.Lock()
	Task.Output = TruncateOutput(Task.RealtimeOutput)
	Task.RealtimeOutput = ""
	Task.Lock.Unlock()
	
	DB_UpdateCommandTaskInfo(Task)
}

type Task_StdOut struct {
	Lock sync.RWMutex
	RawOutput string
}

func CL_BasicWatchStdPipe(StdPipe io.ReadCloser) *Task_StdOut {
	StdOut := &Task_StdOut{}
	
	go func(){
		buf := make([]byte, 4096)
		for {
			count, err := StdPipe.Read(buf)
			if count > 0 {
				StdOut.Lock.Lock()
				StdOut.RawOutput += string(buf[0:count])
				StdOut.Lock.Unlock()
			}
			
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			} else if err != nil {
				L_Printf("Unknown pipe error? : %v\n", err)
				break
			}
		}
	}()
	
	return StdOut
}

// Create a blank command task with a random id
func CL_NewTask() *CommandTask {
	return &CommandTask{
		Lock: &sync.RWMutex{},
		
		Id: uuid.New().String(),
		
		StartTime: time.Now().UTC(),
		EndTime:   time.Now().UTC(),
	}
}

func CL_FinishTask(Task *CommandTask, Status int) {
	Task.Lock.Lock()
	
	Task.Status = Status
	Task.EndTime = time.Now().UTC()
	Task.UpdatedAt = time.Now().UTC()
	
	Task.Lock.Unlock()
	
	DB_UpdateCommandTaskInfo(Task)
	
	ARCT_Lock.Lock()
	if AllRunningCommandTasks[Task.Id] != nil {
		delete(AllRunningCommandTasks, Task.Id)
	}
	ARCT_Lock.Unlock()
}

func CL_CommandTaskRun(Task *CommandTask, stdout io.ReadCloser, stderr io.ReadCloser) {
	ARCT_Lock.Lock()
	AllRunningCommandTasks[Task.Id] = Task
	ARCT_Lock.Unlock()
	
	if stdout != nil && stderr != nil {
		go monitorTaskCmdOutput(Task, stdout, stderr)
	}
	
	NextUpdate := time.Now().UnixMilli() + (1000 * 10)
	
	for {
		exitCode := Task.Cmd.ProcessState.ExitCode()
		if exitCode != -1 {
			if exitCode > 0 {
				// Command failed?
				Task.Lock.Lock()
				if Task.Status == TASK_STATUS_RUNNING {
					Task.Lock.Unlock()
					CL_FinishTask(Task, TASK_STATUS_FAILED)
				} else {
					Task.Lock.Unlock()
				}
			}
			break
		}
		Task.Lock.Lock()
		if Task.Status != TASK_STATUS_RUNNING {
			Task.Lock.Unlock()
			L_Printf("This task isn't running? Id: %s\n", Task.Id)
			break
		}
		Task.Lock.Unlock()
		
		if time.Now().UnixMilli() > NextUpdate {
			Task.Lock.Lock()
			if Task.RealtimeOutput != "" {
				Task.Output = TruncateOutput(Task.RealtimeOutput)
			}
			Task.UpdatedAt = time.Now().UTC()
			
			Task.Lock.Unlock()
			DB_UpdateCommandTaskInfo(Task)
			NextUpdate = time.Now().UnixMilli() + (1000 * 10)
		}
		time.Sleep(1000 * time.Millisecond)
	}
	
	Task.Lock.Lock()
	if Task.Status == TASK_STATUS_RUNNING {
		Task.Lock.Unlock()
		CL_FinishTask(Task, TASK_STATUS_FINISHED)
	} else {
		Task.Lock.Unlock()
	}
}

func GetRealArgs(Args []string) string {
	SB := []string{}
	for i := 0; i < len(Args); i++ {
		Arg := Args[i]
		if strings.Contains(Arg, " ") {
			Arg = "\"" + Arg + "\""
		}
		
		SB = append(SB, Arg)
	}
	
	return strings.Join(SB, " ")
}

func CL_CancelTask(Task *CommandTask) error {
	ARCT_Lock.Lock()
	ActiveTask := AllRunningCommandTasks[Task.Id]
	ARCT_Lock.Unlock()
	if Task != ActiveTask {
		Task = ActiveTask
	}
	
	CanCancel := false
	if Task.Status == TASK_STATUS_RUNNING {
		CanCancel = true
	}
	
	if Task.Cmd != nil {
		err := Task.Cmd.Process.Kill()
		if err != nil {
			L_Printf("Cannot kill process | err: %v\n", err)
			return err
		}
	}
	if CanCancel {
		CL_FinishTask(Task, TASK_STATUS_CANCELED)
		
		CL_Logf(Task, "\nThis task was canceled manually.\n")
		DB_UpdateCommandTaskInfo(Task)
	}
	
	return nil
}

func CL_RunDownloadTask(Cmd *exec.Cmd, Video *VideoInfo, ChannelId string) (*CommandTask, error) {
	Task := CL_NewTask()
	
	Task.Title = fmt.Sprintf("Downloading video: \"%s\"", Video.Title)
	
	Task.FromVideoId = Video.Id
	Task.FromChannelId = ChannelId
	Task.Type = TASK_TYPE_DOWNLOAD
	Task.Cmd = Cmd
	
	Task.RunArgs = GetRealArgs(Cmd.Args)
	
	stdout, err := Task.Cmd.StdoutPipe()
	if err != nil {
		ErrMsg := fmt.Sprintf("Error creating StdoutPipe: %s\n", err)
		Task.Output = ErrMsg
		L_Printf("%s", ErrMsg)
		
		CL_FinishTask(Task, TASK_STATUS_FAILED)
		return nil, err
	}
	stderr, err := Task.Cmd.StderrPipe()
	if err != nil {
		ErrMsg := fmt.Sprintf("Error creating StderrPipe: %s\n", err)
		Task.Output = ErrMsg
		L_Printf("%s", ErrMsg)
		
		CL_FinishTask(Task, TASK_STATUS_FAILED)
		return nil, err
	}
	
	go CL_CommandTaskRun(Task, stdout, stderr)
	
	Task.UpdatedAt = time.Now().UTC()
	DB_UpdateCommandTaskInfo(Task)
	return Task, nil
}

func CL_RunListTask(Cmd *exec.Cmd, AChannel *ArchiveChannel) (*CommandTask, error) {
	Task := CL_NewTask()
	Task.Title = fmt.Sprintf("Listing videos for %s", AChannel.Name)
	
	Task.FromChannelId = AChannel.Id
	Task.Type = TASK_TYPE_LISTING
	Task.Cmd = Cmd
	
	Task.RunArgs = GetRealArgs(Cmd.Args)
	
	go CL_CommandTaskRun(Task, nil, nil)
	
	Task.UpdatedAt = time.Now().UTC()
	DB_UpdateCommandTaskInfo(Task)
	return Task, nil
}

func CL_NewGenericTask() *CommandTask {
	Task := CL_NewTask()
	Task.Title = "Generic Task."
	
	Task.Type = TASK_TYPE_GENERIC
	
	ARCT_Lock.Lock()
	AllRunningCommandTasks[Task.Id] = Task
	ARCT_Lock.Unlock()
	
	Task.UpdatedAt = time.Now().UTC()
	//DB_UpdateCommandTaskInfo(Task)
	return Task
}

func CL_ListCommandTasks(Limit int, Offset int, Query ListCommandTasksQuery) ([]*CommandTask, error) {
	TasksList, err := DB_ListCommandTasks(Limit, Offset, Query)
	if err != nil {
		return nil, err
	}
	
	ARCT_Lock.RLock()
	defer ARCT_Lock.RUnlock()
	for _, Task := range(TasksList) {
		ActiveTask := AllRunningCommandTasks[Task.Id]
		if ActiveTask != nil {
			ActiveTask.Lock.RLock()
			
			if ActiveTask.RealtimeOutput != "" {
				Task.Output = TruncateOutput(ActiveTask.RealtimeOutput)
			} else {
				Task.Output = TruncateOutput(ActiveTask.Output)
			}
			Task.Status  = ActiveTask.Status
			Task.EndTime = ActiveTask.EndTime
			
			ActiveTask.Lock.RUnlock()
		} else {
			if Limit > 0 && Limit <= 30 {	// Don't send out the output if the request size is too big...
				Task.Output, err = DB_GetCommandTaskOutput(Task.Id)
				if len(Task.Output) > MAX_TASK_OUTPUT_LOG+100 {
					Task.Output = TruncateOutput(Task.Output)
				}
			}
		}
	}
	
	return TasksList, nil
}
func CL_GetCommandTask(TaskId string) (*CommandTask, error) {
	ARCT_Lock.RLock()
	Task := AllRunningCommandTasks[TaskId]
	ARCT_Lock.RUnlock()
	
	if Task == nil {
		var err error
		Task, err = DB_GetCommandTask(TaskId)
		if err != nil {
			L_Printf("DB_GetCommandTask error: %v !\n", err)
			return nil, err
		}
	}
	
	return Task, nil
}

func CL_IsRunning(Task *CommandTask) bool {
	if Task == nil {
		return false
	}
	Task.Lock.Lock()
	defer Task.Lock.Unlock()
	
	if Task.Status == TASK_STATUS_RUNNING {
		return true
	}
	
	return false
}

func CL_CancelTasksForVideo(VideoId string, TaskType int) {
	Tasks, err := CL_ListCommandTasks(-1, 0, ListCommandTasksQuery{
		Status: 0,   // Search for running tasks.
		Type: TaskType,
		FromVideoId: VideoId,
	})
	if err == nil {
		for _, Task := range(Tasks) {
			CL_CancelTask(Task)
		}
	}
}

func CL_Logf(Task *CommandTask, format string, a ... any) {
	if Task == nil {
		L_Printf(format, a ...)
		return
	}
	
	Msg := fmt.Sprintf(format, a ...)
	
	Task.Lock.Lock()
	Task.Output += Msg
	Task.RealtimeOutput += Msg
	Task.UpdatedAt = time.Now().UTC()
	Task.Lock.Unlock()
	
	//DB_UpdateCommandTaskInfo(Task)
}

func CleanUpTasksInDatabase() {
	if !G_Config.TaskLog_AutoDelete_Enabled {
		// Auto deleting for task logs isn't enabled.
		return
	}
	
	TasksList, err := DB_ListCommandTasks(-1, 0, ListCommandTasksQuery{
		Type: -1,
		Status: -1,
	})
	if err != nil {
		L_Printf("CleanUpListingTasksInDatabase error: %v\n", err)
		return
	}
	
	TaskLog_AutoDelete_Seconds      := G_Config.TaskLog_AutoDelete_Seconds
	TaskLog_List_AutoDelete_Seconds := G_Config.TaskLog_List_AutoDelete_Seconds
	
	DeleteTime         := time.Now().UTC().Add(time.Second * -time.Duration(TaskLog_AutoDelete_Seconds))
	Listing_DeleteTime := time.Now().UTC().Add(time.Second * -time.Duration(TaskLog_List_AutoDelete_Seconds))
	
	DeleteCount := 0
	
	ARCT_Lock.RLock()
	defer ARCT_Lock.RUnlock()
	
	for _, Task := range(TasksList) {
		DeleteThis := false
		
		if Task.Type == TASK_TYPE_LISTING {
			if TaskLog_List_AutoDelete_Seconds > 0 && Listing_DeleteTime.Unix() > Task.EndTime.Unix() {
				DeleteThis = true
			}
		} else {
			if TaskLog_AutoDelete_Seconds > 0 && DeleteTime.Unix() > Task.EndTime.Unix() {
				DeleteThis = true
			}
		}
		if DeleteThis {
			err := DB_DeleteCommandTask(Task.Id)
			if err != nil {
				L_Printf("Could not delete task '%s', error: %v\n", err)
				continue
			}
			DeleteCount += 1
		}
	}
}

func init() {
	AllRunningCommandTasks = make(map[string]*CommandTask)
}
