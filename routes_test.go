// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
)

func TestLockCreatesMissingStateAndConflictsReturn423(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)

	first := lock(t, handler, group, "network", LockInfo{ID: "first"})
	require.Equal(t, http.StatusOK, first.Code)
	require.JSONEq(t, `{"ID":"first"}`, first.Body.String())
	state, err := findState(app, group.Id, "network")
	require.NoError(t, err)
	require.Equal(t, "first", state.GetString("lockID"))
	require.Equal(t, http.StatusLocked, lock(t, handler, group, "network", LockInfo{ID: "first"}).Code)
	require.Equal(t, http.StatusLocked, lock(t, handler, group, "network", LockInfo{ID: "second"}).Code)

	expired := createState(t, app, group, "expired")
	setLock(expired, LockInfo{ID: "expired"}, time.Now().UTC().Add(-lockLease))
	require.NoError(t, app.Save(expired))
	require.Equal(t, http.StatusOK, lock(t, handler, group, "expired", LockInfo{ID: "replacement"}).Code)
	expired = findRecord(t, app, "states", expired.Id)
	require.Equal(t, "replacement", expired.GetString("lockID"))

	deleted := createState(t, app, group, "deleted")
	deleted.Set("deletedAt", types.NowDateTime())
	require.NoError(t, app.Save(deleted))
	require.Equal(t, http.StatusNotFound, lock(t, handler, group, "deleted", LockInfo{ID: "first"}).Code)
}

func TestMalformedLockAndUnlockPayloadsReturn400(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	createState(t, app, group, "network")

	for _, method := range []string{"LOCK", "UNLOCK"} {
		t.Run(method, func(t *testing.T) {
			for _, body := range [][]byte{[]byte(`{`), []byte(`{"ID":""}`), []byte(`{"ID":"   "}`)} {
				response := request(t, handler, method, stateURL(group, "network"), body, group.GetString("username"), testPassword(t))
				require.Equal(t, http.StatusBadRequest, response.Code)
			}
		})
	}
}

func TestUnlockAndDeleteHonorOwnershipAndExpiry(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	state := createState(t, app, group, "network")

	missing := unlock(t, handler, group, "missing", LockInfo{ID: "first"})
	require.Equal(t, http.StatusOK, missing.Code)
	require.JSONEq(t, `{"message":"Lock Not Found. Expired. Probably."}`, missing.Body.String())
	require.Equal(t, http.StatusOK, request(t, handler, http.MethodDelete, stateURL(group, "missing"), nil, group.GetString("username"), testPassword(t)).Code)

	require.Equal(t, http.StatusOK, lock(t, handler, group, "network", LockInfo{ID: "first"}).Code)
	wrongUnlock := unlock(t, handler, group, "network", LockInfo{ID: "second"})
	require.Equal(t, http.StatusBadRequest, wrongUnlock.Code)
	require.JSONEq(t, `{"ID":"first"}`, wrongUnlock.Body.String())
	matchingUnlock := request(t, handler, "UNLOCK", stateURL(group, "network"), []byte("first\n"), group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, matchingUnlock.Code)
	require.JSONEq(t, `{"message":"ok"}`, matchingUnlock.Body.String())
	state = findRecord(t, app, "states", state.Id)
	require.Empty(t, state.GetString("lockID"))
	require.Equal(t, "null", state.GetString("lockInfo"))
	require.True(t, state.GetDateTime("lockExpiresAt").IsZero())

	require.Equal(t, http.StatusOK, lock(t, handler, group, "network", LockInfo{ID: "first"}).Code)
	require.Equal(t, http.StatusLocked, request(t, handler, http.MethodDelete, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t)).Code)
	require.Equal(t, http.StatusLocked, request(t, handler, http.MethodDelete, stateURL(group, "network")+"?ID=second", nil, group.GetString("username"), testPassword(t)).Code)
	require.Equal(t, http.StatusOK, request(t, handler, http.MethodDelete, stateURL(group, "network")+"?ID=first", nil, group.GetString("username"), testPassword(t)).Code)

	state = findRecord(t, app, "states", state.Id)
	require.False(t, state.GetDateTime("deletedAt").IsZero())
	require.Equal(t, http.StatusOK, request(t, handler, http.MethodDelete, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t)).Code)
	require.Equal(t, http.StatusGone, request(t, handler, http.MethodPost, stateURL(group, "network"), []byte(`{"version":4}`), group.GetString("username"), testPassword(t)).Code)

	expired := createState(t, app, group, "expired")
	setLock(expired, LockInfo{ID: "expired-lock"}, time.Now().UTC().Add(-lockLease))
	require.NoError(t, app.Save(expired))
	expiredUnlock := unlock(t, handler, group, "expired", LockInfo{ID: "expired-lock"})
	require.Equal(t, http.StatusOK, expiredUnlock.Code)
	require.JSONEq(t, `{"message":"Lock Not Found. Expired. Probably."}`, expiredUnlock.Body.String())
	require.Equal(t, http.StatusOK, request(t, handler, http.MethodDelete, stateURL(group, "expired"), nil, group.GetString("username"), testPassword(t)).Code)
}

func TestConcurrentLockAttempts(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	createState(t, app, group, "network")

	responses := make(chan *httptest.ResponseRecorder, 2)
	var ready sync.WaitGroup
	var start sync.WaitGroup
	ready.Add(2)
	start.Add(1)

	for _, id := range []string{"first", "second"} {
		go func() {
			ready.Done()
			start.Wait()
			responses <- lock(t, handler, group, "network", LockInfo{ID: id})
		}()
	}

	ready.Wait()
	start.Done()
	first, second := <-responses, <-responses
	require.ElementsMatch(t, []int{http.StatusOK, http.StatusLocked}, []int{first.Code, second.Code})
}

func lock(t *testing.T, handler http.Handler, group *core.Record, name string, info LockInfo) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(info)
	require.NoError(t, err)

	return request(t, handler, "LOCK", stateURL(group, name), body, group.GetString("username"), testPassword(t))
}

func unlock(t *testing.T, handler http.Handler, group *core.Record, name string, info LockInfo) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(info)
	require.NoError(t, err)

	return request(t, handler, "UNLOCK", stateURL(group, name), body, group.GetString("username"), testPassword(t))
}

func TestFirstPostCreatesEncryptedVersionAndGetReturnsOriginalBody(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	body := []byte(`{"version":4,"serial":1}`)

	post := request(t, handler, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, post.Code)

	state, err := findState(app, group.Id, "network")
	require.NoError(t, err)
	statefile, err := app.FindRecordById("statefiles", state.GetString("currentVersion"))
	require.NoError(t, err)
	require.NotEqual(t, body, storedStatefileBytes(t, app, statefile))

	get := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, get.Code)
	require.Equal(t, body, get.Body.Bytes())
	require.Equal(t, strconv.Itoa(len(body)), get.Header().Get("Content-Length"))
}

func TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	first := []byte(`{"version":4,"serial":1}`)
	second := []byte(`{"version":4,"serial":2}`)

	require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), first, group.GetString("username"), testPassword(t)).Code)
	state, err := findState(app, group.Id, "network")
	require.NoError(t, err)
	firstVersion := state.GetString("currentVersion")
	setLock(state, LockInfo{ID: "lock-1"}, time.Now())
	require.NoError(t, app.Save(state))

	wrong := request(t, handler, http.MethodPost, stateURL(group, "network")+"?ID=wrong", second, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusBadRequest, wrong.Code)

	matching := request(t, handler, http.MethodPost, stateURL(group, "network")+"?ID=lock-1", second, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, matching.Code)

	state, err = findState(app, group.Id, "network")
	require.NoError(t, err)
	require.NotEqual(t, firstVersion, state.GetString("currentVersion"))
	versions, err := app.FindRecordsByFilter("statefiles", "state = {:stateID}", "", 0, 0, map[string]any{"stateID": state.Id})
	require.NoError(t, err)
	require.Len(t, versions, 2)
}

func TestGetReturnsNotFoundForMissingDeletedAndVersionlessStates(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)

	t.Run("missing", func(t *testing.T) {
		response := request(t, handler, http.MethodGet, stateURL(group, "missing"), nil, group.GetString("username"), testPassword(t))
		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("deleted", func(t *testing.T) {
		state := createState(t, app, group, "deleted")
		state.Set("deletedAt", types.NowDateTime())
		require.NoError(t, app.Save(state))

		response := request(t, handler, http.MethodGet, stateURL(group, "deleted"), nil, group.GetString("username"), testPassword(t))
		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("versionless", func(t *testing.T) {
		createState(t, app, group, "versionless")

		response := request(t, handler, http.MethodGet, stateURL(group, "versionless"), nil, group.GetString("username"), testPassword(t))
		require.Equal(t, http.StatusNotFound, response.Code)
	})
}

func TestPostRejectsDeletedStateAndInvalidLockIDUsage(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	body := []byte(`{"version":4}`)

	t.Run("deleted state", func(t *testing.T) {
		state := createState(t, app, group, "deleted")
		state.Set("deletedAt", types.NowDateTime())
		require.NoError(t, app.Save(state))

		response := request(t, handler, http.MethodPost, stateURL(group, "deleted"), body, group.GetString("username"), testPassword(t))
		require.Equal(t, http.StatusGone, response.Code)
	})

	t.Run("ID without active lock", func(t *testing.T) {
		createState(t, app, group, "unlocked")

		response := request(t, handler, http.MethodPost, stateURL(group, "unlocked")+"?ID=lock-1", body, group.GetString("username"), testPassword(t))
		require.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("active lock without ID", func(t *testing.T) {
		state := createState(t, app, group, "locked")
		setLock(state, LockInfo{ID: "lock-1"}, time.Now())
		require.NoError(t, app.Save(state))

		response := request(t, handler, http.MethodPost, stateURL(group, "locked"), body, group.GetString("username"), testPassword(t))
		require.Equal(t, http.StatusBadRequest, response.Code)
	})
}

func TestExpiredLockRejectsStaleIDAndAllowsNoIDPost(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	state := createState(t, app, group, "network")
	setLock(state, LockInfo{ID: "expired-lock"}, time.Now().Add(-lockLease))
	require.NoError(t, app.Save(state))

	stale := request(t, handler, http.MethodPost, stateURL(group, "network")+"?ID=expired-lock", []byte(`{"version":4}`), group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusBadRequest, stale.Code)

	response := request(t, handler, http.MethodPost, stateURL(group, "network"), []byte(`{"version":4}`), group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, response.Code)

	state = findRecord(t, app, "states", state.Id)
	require.Empty(t, state.GetString("lockID"))
	require.Equal(t, "null", state.GetString("lockInfo"))
	require.True(t, state.GetDateTime("lockExpiresAt").IsZero())
}

func TestTamperedStateVersionsReturn503(t *testing.T) {
	tests := map[string]func(t *testing.T, app core.App, group, statefile *core.Record){
		"missing file": func(t *testing.T, app core.App, _ *core.Record, statefile *core.Record) {
			fsys, err := app.NewFilesystem()
			require.NoError(t, err)
			defer fsys.Close()
			require.NoError(t, fsys.Delete(statefile.BaseFilesPath()+"/"+statefile.GetString("file")))
		},
		"corrupt ciphertext": func(t *testing.T, app core.App, _ *core.Record, statefile *core.Record) {
			fsys, err := app.NewFilesystem()
			require.NoError(t, err)
			defer fsys.Close()
			require.NoError(t, fsys.Upload([]byte("not encrypted"), statefile.BaseFilesPath()+"/"+statefile.GetString("file")))
		},
		"decryption failure": func(t *testing.T, app core.App, _ *core.Record, statefile *core.Record) {
			document, err := EncryptState([]byte(`{"version":4,"serial":2}`), bytes.Repeat([]byte{2}, 32), "application/json")
			require.NoError(t, err)
			fsys, err := app.NewFilesystem()
			require.NoError(t, err)
			defer fsys.Close()
			require.NoError(t, fsys.Upload(document.Ciphertext, statefile.BaseFilesPath()+"/"+statefile.GetString("file")))
		},
		"integrity failure": func(t *testing.T, app core.App, _ *core.Record, statefile *core.Record) {
			statefile.Set("sha256", "0000000000000000000000000000000000000000000000000000000000000000")
			require.NoError(t, app.UnsafeWithoutHooks().Save(statefile))
		},
		"key failure": func(t *testing.T, app core.App, group, _ *core.Record) {
			group.Set("wrappedStateKey", "invalid wrapped key")
			require.NoError(t, app.UnsafeWithoutHooks().Save(group))
		},
	}

	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			app, group, handler := newHTTPTestAppWithGroup(t)
			body := []byte(`{"version":4,"serial":1}`)
			require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), testPassword(t)).Code)
			state, err := findState(app, group.Id, "network")
			require.NoError(t, err)
			statefile, err := app.FindRecordById("statefiles", state.GetString("currentVersion"))
			require.NoError(t, err)

			corrupt(t, app, group, statefile)

			response := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
			requireServiceUnavailable(t, response)
		})
	}
}

func TestPasswordRotationRejectsOldPasswordAndDecryptsOldVersion(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	body := []byte(`{"version":4,"serial":1}`)
	require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), testPassword(t)).Code)

	group = findRecord(t, app, "groups", group.Id)
	group.SetPassword(testPassword(t) + "-rotated")
	require.NoError(t, app.Save(group))

	oldPassword := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusUnauthorized, oldPassword.Code)

	newPassword := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t)+"-rotated")
	require.Equal(t, http.StatusOK, newPassword.Code)
	require.Equal(t, body, newPassword.Body.Bytes())
}

func TestRestartReadsCurrentVersionFromExistingDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	cfg := Config{StateMasterKey: make([]byte, 32)}
	app := newApp(cfg, dataDir)
	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())
	firstRunning := true
	t.Cleanup(func() {
		if firstRunning {
			require.NoError(t, app.ResetBootstrapState())
		}
	})

	group := createGroup(t, app, createUser(t, app), "platform", "terraform", testPassword(t))
	body := []byte(`{"version":4,"serial":1}`)
	require.Equal(t, http.StatusOK, request(t, newTestHandler(t, app), http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), testPassword(t)).Code)
	require.NoError(t, app.ResetBootstrapState())
	firstRunning = false

	restarted := newApp(cfg, dataDir)
	require.NoError(t, restarted.Bootstrap())
	t.Cleanup(func() {
		require.NoError(t, restarted.ResetBootstrapState())
	})

	response := request(t, newTestHandler(t, restarted), http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, body, response.Body.Bytes())
}

func TestFailedPostPreservesCurrentVersion(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	first := []byte(`{"version":4,"serial":1}`)
	second := []byte(`{"version":4,"serial":2}`)

	require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), first, group.GetString("username"), testPassword(t)).Code)

	failStateUpdate := true
	app.OnRecordUpdate("states").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			if failStateUpdate && e.Record.GetString("currentVersion") != e.Record.Original().GetString("currentVersion") {
				return errInjectedSaveFailure
			}
			return e.Next()
		},
	})
	failed := request(t, handler, http.MethodPost, stateURL(group, "network"), second, group.GetString("username"), testPassword(t))
	failStateUpdate = false
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)

	get := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, get.Code)
	require.Equal(t, first, get.Body.Bytes())
}

var errInjectedSaveFailure = errors.New("injected state save failure")

func storedStatefileBytes(t *testing.T, app core.App, statefile *core.Record) []byte {
	t.Helper()

	fsys, err := app.NewFilesystem()
	require.NoError(t, err)
	defer fsys.Close()

	file, err := fsys.GetReader(statefile.BaseFilesPath() + "/" + statefile.GetString("file"))
	require.NoError(t, err)
	defer file.Close()

	contents, err := io.ReadAll(file)
	require.NoError(t, err)

	return contents
}

func requireServiceUnavailable(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusServiceUnavailable, response.Code)

	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &responseBody))
	require.Equal(t, map[string]any{
		"status":  float64(http.StatusServiceUnavailable),
		"message": "State unavailable.",
		"data":    map[string]any{},
	}, responseBody)
}

func stateURL(group *core.Record, name string) string {
	return "/state/" + group.GetString("slug") + "/" + name
}
