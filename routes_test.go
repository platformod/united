// SPDX-License-Identifier: MPL-2.0

package main

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
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
	require.NotEqual(t, string(body), statefile.GetString("file"))

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

func stateURL(group *core.Record, name string) string {
	return "/state/" + group.GetString("slug") + "/" + name
}
