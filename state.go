// SPDX-License-Identifier: MPL-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	lockLease              = 35 * time.Minute
	lockFieldsPerRecord    = 3
	stateMasterKeyStoreKey = "united.stateMasterKey"
)

var (
	errStateMissing       = errors.New("state missing")
	errStateDeleted       = errors.New("state deleted")
	errLockConflict       = errors.New("lock conflict")
	errInvalidWriteLockID = errors.New("invalid write lock ID")
	errUnlockOwnership    = errors.New("unlock ownership")
	errInvalidStoredLock  = errors.New("invalid stored lock")
)

func findState(app core.App, groupID, name string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(
		"states",
		"group = {:groupID} && name = {:name}",
		dbx.Params{"groupID": groupID, "name": name},
	)
}

func getState(e *core.RequestEvent, group *core.Record) error {
	state, err := findState(e.App, group.Id, e.Request.PathValue("name"))
	if errors.Is(err, sql.ErrNoRows) {
		return e.NotFoundError("State not found.", nil)
	}

	if err != nil {
		return stateUnavailable(e)
	}

	if !state.GetDateTime("deletedAt").IsZero() || state.GetString("currentVersion") == "" {
		return e.NotFoundError("State not found.", nil)
	}

	statefile, err := e.App.FindRecordById("statefiles", state.GetString("currentVersion"))
	if err != nil {
		return stateUnavailable(e)
	}

	ciphertext, err := readStatefile(e.App, statefile)
	if err != nil {
		return stateUnavailable(e)
	}

	groupKey, ok := e.App.Store().Get(stateMasterKeyStoreKey).([]byte)
	if !ok {
		return stateUnavailable(e)
	}

	key, err := UnwrapGroupKey(groupKey, group.GetString("wrappedStateKey"))
	if err != nil {
		return stateUnavailable(e)
	}

	plaintext, err := DecryptState(StateDocument{
		Ciphertext:    ciphertext,
		ContentLength: statefile.GetInt64("contentLength"),
		ContentType:   statefile.GetString("contentType"),
		SHA256:        statefile.GetString("sha256"),
	}, key)
	if err != nil {
		return stateUnavailable(e)
	}

	e.Response.Header().Set("Content-Length", statefile.GetString("contentLength"))

	return e.Blob(http.StatusOK, statefile.GetString("contentType"), plaintext)
}

func postState(e *core.RequestEvent, group *core.Record, masterKey []byte) error {
	plaintext, err := io.ReadAll(e.Request.Body)
	if err != nil {
		return stateUnavailable(e)
	}

	groupKey, err := UnwrapGroupKey(masterKey, group.GetString("wrappedStateKey"))
	if err != nil {
		return stateUnavailable(e)
	}

	document, err := EncryptState(plaintext, groupKey, e.Request.Header.Get("Content-Type"))
	if err != nil {
		return stateUnavailable(e)
	}

	lockID := e.Request.URL.Query().Get("ID")

	err = e.App.RunInTransaction(func(txApp core.App) error {
		return writeStateVersion(txApp, group, e.Request.PathValue("name"), lockID, document)
	})
	if err == nil {
		return e.NoContent(http.StatusOK)
	}

	if errors.Is(err, errStateDeleted) {
		return e.Error(http.StatusGone, "State deleted.", nil)
	}

	if errors.Is(err, errInvalidWriteLockID) {
		return e.BadRequestError("Invalid lock ID.", nil)
	}

	return stateUnavailable(e)
}

func writeStateVersion(txApp core.App, group *core.Record, name, lockID string, document StateDocument) error {
	state, newState, err := stateForPost(txApp, group, name, lockID)
	if err != nil {
		return err
	}

	if err := authorizePostLock(txApp, state, lockID); err != nil {
		return err
	}

	if newState {
		if err := txApp.Save(state); err != nil {
			return err
		}

		state, err = txApp.FindRecordById("states", state.Id)
		if err != nil {
			return err
		}
	}

	statefile, err := saveStatefile(txApp, state, group, document)
	if err != nil {
		return err
	}

	// If this state save fails, the database transaction retains the prior version;
	// an uploaded, unreachable encrypted file is an accepted first-phase cleanup risk.
	state.Set("currentVersion", statefile.Id)

	return txApp.Save(state)
}

func stateForPost(txApp core.App, group *core.Record, name, lockID string) (*core.Record, bool, error) {
	state, err := findState(txApp, group.Id, name)
	if !errors.Is(err, sql.ErrNoRows) {
		if err != nil {
			return nil, false, err
		}

		if !state.GetDateTime("deletedAt").IsZero() {
			return nil, false, errStateDeleted
		}

		return state, false, nil
	}

	if lockID != "" {
		return nil, false, errInvalidWriteLockID
	}

	states, err := txApp.FindCollectionByNameOrId("states")
	if err != nil {
		return nil, false, err
	}

	state = core.NewRecord(states)
	state.Set("group", group.Id)
	state.Set("name", name)

	return state, true, nil
}

func authorizePostLock(txApp core.App, state *core.Record, lockID string) error {
	active, expired, lock, err := activeLock(state, time.Now())
	if err != nil {
		return err
	}

	if expired {
		if _, err := clearExpiredLock(txApp, state, time.Now()); err != nil {
			return err
		}

		active = false
	}

	if (active && lockID != lock.ID) || (!active && lockID != "") {
		return errInvalidWriteLockID
	}

	return nil
}

func saveStatefile(txApp core.App, state, group *core.Record, document StateDocument) (*core.Record, error) {
	statefiles, err := txApp.FindCollectionByNameOrId("statefiles")
	if err != nil {
		return nil, err
	}

	file, err := filesystem.NewFileFromBytes(document.Ciphertext, "state.enc")
	if err != nil {
		return nil, err
	}

	statefile := core.NewRecord(statefiles)
	statefile.Set("state", state.Id)
	statefile.Set("group", group.Id)
	statefile.Set("file", file)
	statefile.Set("contentLength", document.ContentLength)
	statefile.Set("contentType", document.ContentType)
	statefile.Set("sha256", document.SHA256)

	if err := txApp.Save(statefile); err != nil {
		return nil, err
	}

	return statefile, nil
}

func readStatefile(app core.App, statefile *core.Record) ([]byte, error) {
	fsys, err := app.NewFilesystem()
	if err != nil {
		return nil, err
	}
	defer fsys.Close()

	file, err := fsys.GetFile(statefile.BaseFilesPath() + "/" + statefile.GetString("file"))
	if err != nil {
		return nil, err
	}

	defer file.Close()

	return io.ReadAll(file)
}

func stateUnavailable(e *core.RequestEvent) error {
	return e.Error(http.StatusServiceUnavailable, "State unavailable.", nil)
}

func activeLock(state *core.Record, now time.Time) (active bool, expired bool, info LockInfo, err error) {
	lockID := state.GetString("lockID")
	lockInfo := strings.TrimSpace(state.GetString("lockInfo"))
	expiresAt := state.GetDateTime("lockExpiresAt")

	fieldsPresent := countPresentLockFields(lockID, lockInfo, expiresAt)
	if fieldsPresent == 0 {
		return false, false, LockInfo{}, nil
	}

	if fieldsPresent != lockFieldsPerRecord {
		return false, false, LockInfo{}, errInvalidStoredLock
	}

	info, err = decodeLockInfo(lockID, lockInfo)
	if err != nil {
		return false, false, LockInfo{}, errInvalidStoredLock
	}

	if now.UTC().Before(expiresAt.Time()) {
		return true, false, info, nil
	}

	return false, true, info, nil
}

func countPresentLockFields(lockID, lockInfo string, expiresAt types.DateTime) int {
	fields := []bool{
		lockID != "",
		lockInfo != "" && lockInfo != "null",
		!expiresAt.IsZero(),
	}
	count := 0

	for _, present := range fields {
		if present {
			count++
		}
	}

	return count
}

func decodeLockInfo(lockID, payload string) (LockInfo, error) {
	var info LockInfo
	if err := json.Unmarshal([]byte(payload), &info); err != nil {
		return LockInfo{}, errInvalidStoredLock
	}

	if info.ID == "" || info.ID != lockID {
		return LockInfo{}, errInvalidStoredLock
	}

	return info, nil
}

func clearExpiredLock(app core.App, state *core.Record, now time.Time) (bool, error) {
	_, expired, _, err := activeLock(state, now)
	if err != nil || !expired {
		return false, err
	}

	log.Printf(
		"expired state lock: group_id=%q state_name=%q expires_at=%s",
		state.GetString("group"),
		state.GetString("name"),
		state.GetDateTime("lockExpiresAt").String(),
	)
	clearLock(state)

	return true, app.Save(state)
}

func clearLock(state *core.Record) {
	state.Set("lockID", "")
	state.Set("lockInfo", "null")
	state.Set("lockExpiresAt", types.DateTime{})
}

func setLock(state *core.Record, info LockInfo, now time.Time) {
	lockInfo, err := json.Marshal(info)
	if err != nil {
		panic(err)
	}

	expiresAt, err := types.ParseDateTime(now.UTC().Add(lockLease))
	if err != nil {
		panic(err)
	}

	state.Set("lockID", info.ID)
	state.Set("lockInfo", string(lockInfo))
	state.Set("lockExpiresAt", expiresAt)
}
