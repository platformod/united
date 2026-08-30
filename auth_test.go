// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func TestStateRoutesRequireMatchingGroupCredentials(t *testing.T) {
	_, group, handler := newHTTPTestAppWithGroup(t)

	response := request(t, handler, http.MethodGet, "/state/"+group.GetString("slug")+"/network", nil, "wrong", "password")

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, `Basic realm="Authorization Required", charset="UTF-8"`, response.Header().Get("WWW-Authenticate"))
}

func TestDeletedGroupReturnsGoneOnlyAfterValidBasicAuth(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	group = findRecord(t, app, "groups", group.Id)
	group.Set("deletedAt", "2026-08-30 00:00:00.000Z")
	require.NoError(t, app.Save(group))

	validCredentials := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusGone, validCredentials.Code)

	for _, credentials := range []struct {
		name     string
		username string
		password string
	}{
		{name: "unknown group", username: group.GetString("username"), password: testPassword(t)},
		{name: "wrong username", username: "wrong", password: testPassword(t)},
		{name: "wrong password", username: group.GetString("username"), password: "wrong"},
	} {
		t.Run(credentials.name, func(t *testing.T) {
			target := stateURL(group, "network")
			if credentials.name == "unknown group" {
				target = "/state/unknown/network"
			}

			response := request(t, handler, http.MethodGet, target, nil, credentials.username, credentials.password)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Equal(t, basicAuthChallenge, response.Header().Get("WWW-Authenticate"))
		})
	}
}

func TestCrossGroupCredentialsCannotAccessAnotherGroupURL(t *testing.T) {
	app, platform, handler := newHTTPTestAppWithGroup(t)
	operations := createGroup(t, app, findRecord(t, app, "users", platform.GetString("owner")), "operations", "operations-tf", testPassword(t))

	response := request(t, handler, http.MethodGet, stateURL(platform, "network"), nil, operations.GetString("username"), testPassword(t))

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, `Basic realm="Authorization Required", charset="UTF-8"`, response.Header().Get("WWW-Authenticate"))
}

func TestStateAccessSurvivesGroupDisplayAndUserNameChanges(t *testing.T) {
	app, group, handler := newHTTPTestAppWithGroup(t)
	body := []byte(`{"version":4,"serial":1}`)
	require.Equal(t, http.StatusOK, request(t, handler, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), testPassword(t)).Code)

	group = findRecord(t, app, "groups", group.Id)
	group.Set("displayName", "Platform Engineering")
	require.NoError(t, app.Save(group))
	owner := findRecord(t, app, "users", group.GetString("owner"))
	owner.Set("name", "Renamed Owner")
	require.NoError(t, app.Save(owner))

	response := request(t, handler, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, body, response.Body.Bytes())
}

func TestEveryStateRouteRequiresBasicAuth(t *testing.T) {
	_, group, handler := newHTTPTestAppWithGroup(t)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, "LOCK", "UNLOCK"} {
		t.Run(method, func(t *testing.T) {
			response := request(t, handler, method, "/state/"+group.GetString("slug")+"/network", nil, "wrong", "password")

			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Equal(t, `Basic realm="Authorization Required", charset="UTF-8"`, response.Header().Get("WWW-Authenticate"))
		})
	}
}

func TestGroupCredentialsCannotUsePocketBaseAuthEndpoint(t *testing.T) {
	_, group, handler := newHTTPTestAppWithGroup(t)

	response := requestJSON(t, handler, http.MethodPost, "/api/collections/groups/auth-with-password", map[string]string{
		"identity": group.GetString("username"),
		"password": testPassword(t),
	})

	require.NotEqual(t, http.StatusOK, response.Code)
}

func newHTTPTestAppWithGroup(t *testing.T) (*pocketbase.PocketBase, *core.Record, http.Handler) {
	t.Helper()

	app := newTestApp(t)
	owner := createUser(t, app)
	group := createGroup(t, app, owner, "platform", "terraform", testPassword(t))

	return app, group, newTestHandler(t, app)
}

func newTestHandler(t *testing.T, app *pocketbase.PocketBase) http.Handler {
	t.Helper()

	router, err := apis.NewRouter(app)
	require.NoError(t, err)
	serveEvent := &core.ServeEvent{App: app, Router: router}
	require.NoError(t, app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		return e.Next()
	}))

	handler, err := router.BuildMux()
	require.NoError(t, err)

	return handler
}

func request(t *testing.T, handler http.Handler, method, target string, body []byte, username, password string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	if username != "" || password != "" {
		request.SetBasicAuth(username, password)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func requestJSON(t *testing.T, handler http.Handler, method, target string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	response := request(t, handler, method, target, body, "", "")
	return response
}
