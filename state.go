// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	lockLease           = 35 * time.Minute
	lockFieldsPerRecord = 3
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
