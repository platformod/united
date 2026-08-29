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
		"password": "correct horse",
	})

	require.NotEqual(t, http.StatusOK, response.Code)
}

func newHTTPTestAppWithGroup(t *testing.T) (*pocketbase.PocketBase, *core.Record, http.Handler) {
	t.Helper()

	app := newTestApp(t)
	owner := createUser(t, app)
	group := createGroup(t, app, owner, "platform", "terraform", "correct horse")

	router, err := apis.NewRouter(app)
	require.NoError(t, err)
	serveEvent := &core.ServeEvent{App: app, Router: router}
	require.NoError(t, app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		return e.Next()
	}))

	handler, err := router.BuildMux()
	require.NoError(t, err)

	return app, group, handler
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
