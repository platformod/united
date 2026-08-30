// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
)

func TestGroupOwnerAPIRestrictsGroupManagementToItsOwner(t *testing.T) {
	app := newTestApp(t)
	handler := newTestHandler(t, app)
	owner := createUser(t, app)
	otherUser := createUserWithEmail(t, app, "other@example.test")
	ownerToken := authenticateUser(t, handler, owner.GetString("email"))
	otherToken := authenticateUser(t, handler, otherUser.GetString("email"))

	created := requestJSONWithAuth(t, handler, http.MethodPost, "/api/collections/groups/records", map[string]string{
		"email":           "platform-tf@terraform.invalid",
		"username":        "platform-tf",
		"password":        testPassword(t),
		"passwordConfirm": testPassword(t),
		"slug":            "platform",
		"displayName":     "Platform",
		"owner":           otherUser.Id,
	}, ownerToken)
	require.Equalf(t, http.StatusOK, created.Code, "response: %s", created.Body.String())

	var group struct {
		ID    string `json:"id"`
		Owner string `json:"owner"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &group))
	require.Equal(t, owner.Id, group.Owner)

	list := requestWithAuth(t, handler, http.MethodGet, "/api/collections/groups/records", nil, ownerToken)
	require.Equal(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), group.ID)

	view := requestWithAuth(t, handler, http.MethodGet, "/api/collections/groups/records/"+group.ID, nil, ownerToken)
	require.Equal(t, http.StatusOK, view.Code)

	updated := requestJSONWithAuth(t, handler, http.MethodPatch, "/api/collections/groups/records/"+group.ID, map[string]string{"displayName": "Platform Engineering"}, ownerToken)
	require.Equal(t, http.StatusOK, updated.Code)

	otherList := requestWithAuth(t, handler, http.MethodGet, "/api/collections/groups/records", nil, otherToken)
	require.Equal(t, http.StatusOK, otherList.Code)
	require.NotContains(t, otherList.Body.String(), group.ID)

	otherView := requestWithAuth(t, handler, http.MethodGet, "/api/collections/groups/records/"+group.ID, nil, otherToken)
	require.Equal(t, http.StatusNotFound, otherView.Code)
}

func TestGroupSoftDeleteRetainsTheRecord(t *testing.T) {
	app := newTestApp(t)
	handler := newTestHandler(t, app)
	owner := createUser(t, app)
	group := createGroup(t, app, owner, "platform", "platform-tf", testPassword(t))
	token := authenticateUser(t, handler, owner.GetString("email"))

	deleted := requestWithAuth(t, handler, http.MethodDelete, "/api/collections/groups/records/"+group.Id, nil, token)
	require.Equal(t, http.StatusNoContent, deleted.Code)

	tombstoned := findRecord(t, app, "groups", group.Id)
	require.False(t, tombstoned.GetDateTime("deletedAt").IsZero())
}

func TestDeletedGroupCannotBeUpdatedOrRevivedThroughTheAPI(t *testing.T) {
	app := newTestApp(t)
	handler := newTestHandler(t, app)
	owner := createUser(t, app)
	group := createGroup(t, app, owner, "platform", "platform-tf", testPassword(t))
	token := authenticateUser(t, handler, owner.GetString("email"))

	require.Equal(t, http.StatusNoContent, requestWithAuth(t, handler, http.MethodDelete, "/api/collections/groups/records/"+group.Id, nil, token).Code)

	update := requestJSONWithAuth(t, handler, http.MethodPatch, "/api/collections/groups/records/"+group.Id, map[string]string{"displayName": "Revived"}, token)
	require.NotEqual(t, http.StatusOK, update.Code)

	revive := requestJSONWithAuth(t, handler, http.MethodPatch, "/api/collections/groups/records/"+group.Id, map[string]string{"deletedAt": ""}, token)
	require.NotEqual(t, http.StatusOK, revive.Code)

	tombstoned := findRecord(t, app, "groups", group.Id)
	require.False(t, tombstoned.GetDateTime("deletedAt").IsZero())
}

func TestGroupSlugMustBeOnePathSafeSegment(t *testing.T) {
	app := newTestApp(t)
	owner := createUser(t, app)
	collection, err := app.FindCollectionByNameOrId("groups")
	require.NoError(t, err)

	for _, slug := range []string{"platform", "platform-prod", "p2"} {
		t.Run("accepts "+slug, func(t *testing.T) {
			group := core.NewRecord(collection)
			group.Set("email", slug+"@terraform.invalid")
			group.Set("username", slug+"-tf")
			group.SetPassword(testPassword(t))
			group.Set("slug", slug)
			group.Set("displayName", slug)
			group.Set("owner", owner.Id)
			require.NoError(t, app.Save(group))
		})
	}

	for _, slug := range []string{"platform/production", "platform production", ".", "..", "platform%2Fproduction", "platform?debug=true"} {
		t.Run("rejects "+slug, func(t *testing.T) {
			group := core.NewRecord(collection)
			group.Set("email", "invalid-"+slug+"@terraform.invalid")
			group.Set("username", "invalid-"+slug)
			group.SetPassword(testPassword(t))
			group.Set("slug", slug)
			group.Set("displayName", slug)
			group.Set("owner", owner.Id)
			require.Error(t, app.Save(group))
		})
	}
}

func TestGroupCreationGeneratesKeyAndRejectsIdentityChanges(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	require.NotEmpty(t, group.GetString("wrappedStateKey"))

	group.Set("slug", "renamed")
	require.Error(t, app.Save(group))
}

func TestStateIdentityCannotChange(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")

	state.Set("name", "renamed")
	require.Error(t, app.Save(state))
}

func TestStatefileMustBelongToStatesGroup(t *testing.T) {
	app := newTestApp(t)
	owner := createUser(t, app)
	stateGroup := createGroup(t, app, owner, "platform", "platform-tf", testPassword(t))
	otherGroup := createGroup(t, app, owner, "operations", "operations-tf", testPassword(t))
	state := createState(t, app, stateGroup, "network")

	statefile := newStatefile(t, app, state, otherGroup)
	require.Error(t, app.Save(statefile))
}

func TestStatefileCannotChange(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	statefile := createStatefile(t, app, createState(t, app, group, "network"), group)

	statefile.Set("contentType", "application/octet-stream")
	require.Error(t, app.Save(statefile))
}

func TestStateCurrentVersionMustBelongToState(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")
	otherState := createState(t, app, group, "database")
	otherVersion := createStatefile(t, app, otherState, group)

	state.Set("currentVersion", otherVersion.Id)
	require.Error(t, app.Save(state))
}

func TestStateLockInfoMustBeJSONWithMatchingID(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))

	for _, test := range []struct {
		name     string
		lockID   string
		lockInfo string
	}{
		{name: "malformed JSON", lockID: "lock-id", lockInfo: `{"ID":`},
		{name: "JSON string", lockID: "lock-id", lockInfo: `"lock-id"`},
		{name: "missing ID", lockID: "lock-id", lockInfo: `{}`},
		{name: "empty ID", lockID: "lock-id", lockInfo: `{"ID":""}`},
		{name: "mismatched ID", lockID: "lock-id", lockInfo: `{"ID":"other-lock"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := createState(t, app, group, test.name)
			state.Set("lockID", test.lockID)
			state.Set("lockInfo", test.lockInfo)
			state.Set("lockExpiresAt", types.NowDateTime().Add(time.Minute))
			require.Error(t, app.Save(state))
		})
	}
}

func TestStateLockFieldsMustBeAllEmptyOrAllPresent(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")

	state.Set("lockID", "lock-id")
	require.Error(t, app.Save(state))

	state = findRecord(t, app, "states", state.Id)
	state.Set("lockID", "lock-id")
	state.Set("lockInfo", map[string]any{"ID": "lock-id"})
	state.Set("lockExpiresAt", types.NowDateTime().Add(time.Minute))
	require.NoError(t, app.Save(state))

	state = findRecord(t, app, "states", state.Id)
	state.Set("lockID", "")
	require.Error(t, app.Save(state))
}

func authenticateUser(t *testing.T, handler http.Handler, email string) string {
	t.Helper()

	response := requestJSONWithAuth(t, handler, http.MethodPost, "/api/collections/users/auth-with-password", map[string]string{
		"identity": email,
		"password": testPassword(t),
	}, "")
	require.Equal(t, http.StatusOK, response.Code)

	var result struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.NotEmpty(t, result.Token)
	return result.Token
}

func requestJSONWithAuth(t *testing.T, handler http.Handler, method, target string, payload any, token string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return requestWithAuth(t, handler, method, target, body, token)
}

func requestWithAuth(t *testing.T, handler http.Handler, method, target string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func createState(t *testing.T, app core.App, group *core.Record, name string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("states")
	require.NoError(t, err)

	state := core.NewRecord(collection)
	state.Set("group", group.Id)
	state.Set("name", name)
	require.NoError(t, app.Save(state))

	return findRecord(t, app, "states", state.Id)
}

func createStatefile(t *testing.T, app core.App, state, group *core.Record) *core.Record {
	t.Helper()
	statefile := newStatefile(t, app, state, group)
	require.NoError(t, app.Save(statefile))

	return findRecord(t, app, "statefiles", statefile.Id)
}

func findRecord(t *testing.T, app core.App, collection, id string) *core.Record {
	t.Helper()

	record, err := app.FindRecordById(collection, id)
	require.NoError(t, err)

	return record
}

func newStatefile(t *testing.T, app core.App, state, group *core.Record) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("statefiles")
	require.NoError(t, err)

	file, err := filesystem.NewFileFromBytes([]byte("ciphertext"), "state.enc")
	require.NoError(t, err)

	statefile := core.NewRecord(collection)
	statefile.Set("state", state.Id)
	statefile.Set("group", group.Id)
	statefile.Set("file", file)
	statefile.Set("contentLength", 9)
	statefile.Set("contentType", "application/json")
	statefile.Set("sha256", "digest")

	return statefile
}
