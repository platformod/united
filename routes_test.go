// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
)

func TestFirstPostCreatesEncryptedVersionAndGetReturnsOriginalBody(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	body := []byte(`{"version":4,"serial":1}`)

	post := request(t, handler, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), "correct horse")
	require.Equal(t, http.StatusOK, post.Code)

	state, err := findState(app, group.Id, "network")
	require.NoError(t, err)
	statefile, err := app.FindRecordById("statefiles", state.GetString("currentVersion"))
	require.NoError(t, err)
	require.NotEqual(t, body, storedStatefileBytes(t, app, statefile))

	get := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), "correct horse")
	require.Equal(t, http.StatusOK, get.Code)
	require.Equal(t, body, get.Body.Bytes())
	require.Equal(t, strconv.Itoa(len(body)), get.Header().Get("Content-Length"))
}

func TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	first := []byte(`{"version":4,"serial":1}`)
	second := []byte(`{"version":4,"serial":2}`)

	require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), first, group.GetString("username"), "correct horse").Code)
	state, err := findState(app, group.Id, "network")
	require.NoError(t, err)
	firstVersion := state.GetString("currentVersion")
	setLock(state, LockInfo{ID: "lock-1"}, time.Now())
	require.NoError(t, app.Save(state))

	wrong := request(t, handler, http.MethodPost, stateURL(group, "network")+"?ID=wrong", second, group.GetString("username"), "correct horse")
	require.Equal(t, http.StatusBadRequest, wrong.Code)

	matching := request(t, handler, http.MethodPost, stateURL(group, "network")+"?ID=lock-1", second, group.GetString("username"), "correct horse")
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
		response := request(t, handler, http.MethodGet, stateURL(group, "missing"), nil, group.GetString("username"), "correct horse")
		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("deleted", func(t *testing.T) {
		state := createState(t, app, group, "deleted")
		state.Set("deletedAt", types.NowDateTime())
		require.NoError(t, app.Save(state))

		response := request(t, handler, http.MethodGet, stateURL(group, "deleted"), nil, group.GetString("username"), "correct horse")
		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("versionless", func(t *testing.T) {
		createState(t, app, group, "versionless")

		response := request(t, handler, http.MethodGet, stateURL(group, "versionless"), nil, group.GetString("username"), "correct horse")
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

		response := request(t, handler, http.MethodPost, stateURL(group, "deleted"), body, group.GetString("username"), "correct horse")
		require.Equal(t, http.StatusGone, response.Code)
	})

	t.Run("ID without active lock", func(t *testing.T) {
		createState(t, app, group, "unlocked")

		response := request(t, handler, http.MethodPost, stateURL(group, "unlocked")+"?ID=lock-1", body, group.GetString("username"), "correct horse")
		require.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("active lock without ID", func(t *testing.T) {
		state := createState(t, app, group, "locked")
		setLock(state, LockInfo{ID: "lock-1"}, time.Now())
		require.NoError(t, app.Save(state))

		response := request(t, handler, http.MethodPost, stateURL(group, "locked"), body, group.GetString("username"), "correct horse")
		require.Equal(t, http.StatusBadRequest, response.Code)
	})
}

func TestPostClearsExpiredLockBeforeWriting(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	state := createState(t, app, group, "network")
	setLock(state, LockInfo{ID: "expired-lock"}, time.Now().Add(-lockLease))
	require.NoError(t, app.Save(state))

	response := request(t, handler, http.MethodPost, stateURL(group, "network"), []byte(`{"version":4}`), group.GetString("username"), "correct horse")
	require.Equal(t, http.StatusOK, response.Code)

	state = findRecord(t, app, "states", state.Id)
	require.Empty(t, state.GetString("lockID"))
	require.Equal(t, "null", state.GetString("lockInfo"))
	require.True(t, state.GetDateTime("lockExpiresAt").IsZero())
}

func TestGetReturnsGenericServiceUnavailableForUnreadableStateVersions(t *testing.T) {
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
			require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), "correct horse").Code)
			state, err := findState(app, group.Id, "network")
			require.NoError(t, err)
			statefile, err := app.FindRecordById("statefiles", state.GetString("currentVersion"))
			require.NoError(t, err)

			corrupt(t, app, group, statefile)

			response := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), "correct horse")
			requireServiceUnavailable(t, response)
		})
	}
}

func TestFailedPostPreservesCurrentVersion(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	first := []byte(`{"version":4,"serial":1}`)
	second := []byte(`{"version":4,"serial":2}`)

	require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), first, group.GetString("username"), "correct horse").Code)

	failStateUpdate := true
	app.OnRecordUpdate("states").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			if failStateUpdate && e.Record.GetString("currentVersion") != e.Record.Original().GetString("currentVersion") {
				return errInjectedSaveFailure
			}
			return e.Next()
		},
	})
	failed := request(t, handler, http.MethodPost, stateURL(group, "network"), second, group.GetString("username"), "correct horse")
	failStateUpdate = false
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)

	get := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), "correct horse")
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
	require.Contains(t, response.Body.String(), "State unavailable.")
	require.NotContains(t, response.Body.String(), "invalid")
	require.NotContains(t, response.Body.String(), "integrity")
	require.NotContains(t, response.Body.String(), "encrypted")
}

func stateURL(group *core.Record, name string) string {
	return "/state/" + group.GetString("slug") + "/" + name
}
