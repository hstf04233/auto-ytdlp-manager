package main

// Channel groups.
// Groups are purely visual, they don't change how channels are checked or downloaded.
// A group can hold channels and other groups, but only one level deep:
//   Group "ALL"
//     Group "ExampleChannel"   <- sub group (cannot hold more groups)
//     Group "ExampleChannel2"
// Viewing a group includes the channels of its sub groups.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const GROUP_MAX_NAME_LENGTH = 128

type ChannelGroup struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	ParentId string `json:"parent_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Serializes group modifications, so validation (nesting rules) and the write happen together.
var GroupsLock sync.Mutex

func DB_ListChannelGroups() ([]*ChannelGroup, error) {
	Rows, err := GDB.Query(`SELECT Id, Name, ParentId, CreatedAt, UpdatedAt FROM ChannelGroups ORDER BY Name COLLATE NOCASE ASC, CreatedAt ASC`)
	if err != nil {
		return nil, err
	}
	defer Rows.Close()

	Groups := []*ChannelGroup{}
	for Rows.Next() {
		Group := &ChannelGroup{}
		var rawCreatedAt, rawUpdatedAt any
		if err := Rows.Scan(&Group.Id, &Group.Name, &Group.ParentId, &rawCreatedAt, &rawUpdatedAt); err != nil {
			return nil, err
		}
		Group.CreatedAt = parseDBTime(rawCreatedAt)
		Group.UpdatedAt = parseDBTime(rawUpdatedAt)
		Groups = append(Groups, Group)
	}

	return Groups, Rows.Err()
}

func DB_GetChannelGroup(GroupId string) (*ChannelGroup, error) {
	Group := &ChannelGroup{}
	var rawCreatedAt, rawUpdatedAt any
	err := GDB.QueryRow(`SELECT Id, Name, ParentId, CreatedAt, UpdatedAt FROM ChannelGroups WHERE Id = ?`, GroupId).Scan(
		&Group.Id, &Group.Name, &Group.ParentId, &rawCreatedAt, &rawUpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	Group.CreatedAt = parseDBTime(rawCreatedAt)
	Group.UpdatedAt = parseDBTime(rawUpdatedAt)

	return Group, nil
}

func DB_UpdateChannelGroup(Group *ChannelGroup) error {
	TimeNow := time.Now().UTC()
	_, err := GDB.Exec(`
	INSERT INTO ChannelGroups(Id, Name, ParentId, CreatedAt, UpdatedAt)
	VALUES (?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Name=excluded.Name,
	ParentId=excluded.ParentId,
	UpdatedAt=excluded.UpdatedAt
	`, Group.Id, Group.Name, Group.ParentId, TimeNow, TimeNow)
	if err != nil {
		L_Printf("DB_UpdateChannelGroup ERR: %v\n", err)
	}
	return err
}

// Deletes the group. Its sub groups become top level groups and its channels become ungrouped.
func DB_DeleteChannelGroup(GroupId string) error {
	Tx, err := GDB.Begin()
	if err != nil {
		return err
	}
	defer Tx.Rollback()

	if _, err := Tx.Exec(`UPDATE ChannelGroups SET ParentId = '' WHERE ParentId = ?`, GroupId); err != nil {
		return err
	}
	if _, err := Tx.Exec(`UPDATE ArchiveChannels SET GroupId = '' WHERE GroupId = ?`, GroupId); err != nil {
		return err
	}
	if _, err := Tx.Exec(`DELETE FROM ChannelGroups WHERE Id = ?`, GroupId); err != nil {
		return err
	}

	return Tx.Commit()
}

// Returns the ids of every channel inside the group, including the channels inside its sub groups.
// Found is false if the group doesn't exist.
func GetChannelIdsInGroup(GroupId string) (ChannelIds []string, Found bool, err error) {
	Groups, err := DB_ListChannelGroups()
	if err != nil {
		return nil, false, err
	}

	GroupIds := map[string]bool{}
	for _, Group := range(Groups) {
		if Group.Id == GroupId {
			Found = true
			GroupIds[Group.Id] = true
		}
	}
	if !Found {
		return nil, false, nil
	}
	for _, Group := range(Groups) {
		if Group.ParentId == GroupId {
			GroupIds[Group.Id] = true
		}
	}

	ChannelIds = []string{}
	G_ArchiveChannels.ChannelsLock.RLock()
	for _, AChannel := range(G_ArchiveChannels.Channels) {
		AChannel.Lock.RLock()
		if AChannel.GroupId != "" && GroupIds[AChannel.GroupId] {
			ChannelIds = append(ChannelIds, AChannel.Id)
		}
		AChannel.Lock.RUnlock()
	}
	G_ArchiveChannels.ChannelsLock.RUnlock()

	return ChannelIds, true, nil
}

// Sets the channel's group (in memory and in the database).
func SetArchiveChannelGroup(AChannel *ArchiveChannel, GroupId string) error {
	AChannel.Lock.Lock()
	if AChannel.GroupId == GroupId {
		AChannel.Lock.Unlock()
		return nil
	}
	AChannel.GroupId = GroupId
	AChannel.Lock.Unlock()

	return DB_UpdateArchiveChannel(AChannel)
}

// Checks if the group id can be used for a channel. '' (ungrouped) is always valid.
func IsValidChannelGroupId(GroupId string) (bool, error) {
	if GroupId == "" {
		return true, nil
	}
	if len(GroupId) > API_MAX_REQUEST_ID {
		return false, nil
	}
	Group, err := DB_GetChannelGroup(GroupId)
	if err != nil {
		return false, err
	}
	return Group != nil, nil
}

// ========== API ==========

type API_RequestGroupBody struct {
	Name     *string `json:"name"`
	ParentId *string `json:"parent_id"`

	// When set, these channels become exactly the channels directly inside this group.
	// Channels that were inside this group but aren't in this list become ungrouped.
	ChannelIds *[]string `json:"channel_ids"`
}

// Must be called with GroupsLock held.
// Checks the nesting rules for putting group 'GroupId' ('' for a new group) under 'ParentId'.
func api_ValidateGroupParent(GroupId string, ParentId string) (bool, string, error) {
	if ParentId == "" {
		return true, "", nil
	}
	if len(ParentId) > API_MAX_REQUEST_ID {
		return false, "Invalid parent group id.", nil
	}
	if ParentId == GroupId {
		return false, "A group cannot be inside itself.", nil
	}

	Groups, err := DB_ListChannelGroups()
	if err != nil {
		return false, "", err
	}

	var Parent *ChannelGroup
	for _, Group := range(Groups) {
		if Group.Id == ParentId {
			Parent = Group
		}
		if GroupId != "" && Group.ParentId == GroupId {
			return false, "This group has sub groups, so it cannot be put inside another group (groups can only be nested one level deep).", nil
		}
	}
	if Parent == nil {
		return false, "Parent group not found.", nil
	}
	if Parent.ParentId != "" {
		return false, "Cannot put a group inside a sub group (groups can only be nested one level deep).", nil
	}

	return true, "", nil
}

// Must be called with GroupsLock held.
func api_ApplyGroupChannels(GroupId string, ChannelIds []string) error {
	Wanted := map[string]bool{}
	for _, Id := range(ChannelIds) {
		if Id == "" || len(Id) > API_MAX_REQUEST_ID || Id == MANUAL_CHANNEL_ID {
			continue
		}
		Wanted[Id] = true
	}

	G_ArchiveChannels.ChannelsLock.RLock()
	Channels := make([]*ArchiveChannel, len(G_ArchiveChannels.Channels))
	copy(Channels, G_ArchiveChannels.Channels)
	G_ArchiveChannels.ChannelsLock.RUnlock()

	for _, AChannel := range(Channels) {
		AChannel.Lock.RLock()
		CurrentGroup := AChannel.GroupId
		AChannel.Lock.RUnlock()

		var err error
		if Wanted[AChannel.Id] {
			err = SetArchiveChannelGroup(AChannel, GroupId)
		} else if CurrentGroup == GroupId {
			err = SetArchiveChannelGroup(AChannel, "")
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func api_VerifyGroupBody(Body API_RequestGroupBody) (bool, string) {
	if Body.Name != nil {
		Name := strings.TrimSpace(*Body.Name)
		if Name == "" {
			return false, "Group name cannot be empty."
		}
		if len(Name) > GROUP_MAX_NAME_LENGTH {
			return false, fmt.Sprintf("Group name must be shorter than %d characters.", GROUP_MAX_NAME_LENGTH)
		}
	}
	if Body.ChannelIds != nil && len(*Body.ChannelIds) > 10000 {
		return false, "Too many channel ids."
	}
	return true, ""
}

func API_GetGroups(w http.ResponseWriter, r *http.Request) {
	Groups, err := DB_ListChannelGroups()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list groups: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(Groups),
		"groups": Groups,
	})
}

func API_NewGroup(w http.ResponseWriter, r *http.Request) {
	var Body API_RequestGroupBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if Body.Name == nil {
		http.Error(w, "Group name required.", http.StatusBadRequest)
		return
	}
	if ok, Reason := api_VerifyGroupBody(Body); !ok {
		http.Error(w, Reason, http.StatusBadRequest)
		return
	}

	GroupsLock.Lock()
	defer GroupsLock.Unlock()

	Group := &ChannelGroup{
		Id:   uuid.New().String(),
		Name: strings.TrimSpace(*Body.Name),
	}
	if Body.ParentId != nil {
		ok, Reason, err := api_ValidateGroupParent("", *Body.ParentId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, Reason, http.StatusBadRequest)
			return
		}
		Group.ParentId = *Body.ParentId
	}

	if err := DB_UpdateChannelGroup(Group); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if Body.ChannelIds != nil {
		if err := api_ApplyGroupChannels(Group.Id, *Body.ChannelIds); err != nil {
			http.Error(w, fmt.Sprintf("Group created, but failed to add channels: %v", err), http.StatusInternalServerError)
			return
		}
	}

	NewGroup, err := DB_GetChannelGroup(Group.Id)
	if err != nil || NewGroup == nil {
		NewGroup = Group
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(NewGroup)
}

func API_UpdateGroup(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if RequestId == "" || len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid group id.", http.StatusBadRequest)
		return
	}

	var Body API_RequestGroupBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if ok, Reason := api_VerifyGroupBody(Body); !ok {
		http.Error(w, Reason, http.StatusBadRequest)
		return
	}

	GroupsLock.Lock()
	defer GroupsLock.Unlock()

	Group, err := DB_GetChannelGroup(RequestId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if Group == nil {
		http.Error(w, "Group not found.", http.StatusNotFound)
		return
	}

	if Body.Name != nil {
		Group.Name = strings.TrimSpace(*Body.Name)
	}
	if Body.ParentId != nil && *Body.ParentId != Group.ParentId {
		ok, Reason, err := api_ValidateGroupParent(Group.Id, *Body.ParentId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, Reason, http.StatusBadRequest)
			return
		}
		Group.ParentId = *Body.ParentId
	}

	if err := DB_UpdateChannelGroup(Group); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if Body.ChannelIds != nil {
		if err := api_ApplyGroupChannels(Group.Id, *Body.ChannelIds); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update group channels: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Group)
}

func API_DeleteGroup(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if RequestId == "" || len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid group id.", http.StatusBadRequest)
		return
	}

	GroupsLock.Lock()
	defer GroupsLock.Unlock()

	Group, err := DB_GetChannelGroup(RequestId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if Group == nil {
		http.Error(w, "Group not found.", http.StatusNotFound)
		return
	}

	if err := DB_DeleteChannelGroup(RequestId); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete group: %v", err), http.StatusInternalServerError)
		return
	}

	// Keep the in memory channels in sync with the database.
	G_ArchiveChannels.ChannelsLock.RLock()
	for _, AChannel := range(G_ArchiveChannels.Channels) {
		AChannel.Lock.Lock()
		if AChannel.GroupId == RequestId {
			AChannel.GroupId = ""
		}
		AChannel.Lock.Unlock()
	}
	G_ArchiveChannels.ChannelsLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
}
