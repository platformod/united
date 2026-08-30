// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestFixtureUserCanAuthenticateWithPassword(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:            "fixture user authenticates with password",
		Method:          http.MethodPost,
		URL:             "/api/collections/users/auth-with-password",
		Body:            strings.NewReader(`{"identity":"user@example.com","password":"foofoofoo"}`),
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"token":`, `"email":"user@example.com"`},
		TestAppFactory:  fixtureAppWithUnitedRoutes,
	}
	scenario.Test(t)
}

func TestFixtureUserCanAuthenticateAndCreateOwnedGroup(t *testing.T) {
	userToken := fixtureAuthToken(t, newFixtureTestApp(t), "users", "user@example.com")
	scenario := tests.ApiScenario{
		Name:            "authenticated user creates a group owned by themselves",
		Method:          http.MethodPost,
		URL:             "/api/collections/groups/records",
		Headers:         map[string]string{"Authorization": userToken},
		Body:            strings.NewReader(`{"email":"fixture-tf@terraform.invalid","username":"fixture-tf","password":"fixture-password","passwordConfirm":"fixture-password","slug":"fixture","displayName":"Fixture","owner":"another-owner"}`),
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"slug":"fixture"`},
		TestAppFactory:  fixtureAppWithUnitedRoutes,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			t.Helper()

			created, err := app.FindFirstRecordByFilter("groups", "slug = {:slug}", map[string]any{"slug": "fixture"})
			require.NoError(t, err)
			owner, err := app.FindAuthRecordByEmail("users", "user@example.com")
			require.NoError(t, err)
			require.Equal(t, owner.Id, created.GetString("owner"))
		},
	}
	scenario.Test(t)
}

func TestStateCollectionRejectsUnauthenticatedRequests(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:           "states collection rejects unauthenticated requests",
		Method:         http.MethodGet,
		URL:            "/api/collections/states/records",
		ExpectedStatus: http.StatusForbidden,
		ExpectedContent: []string{
			`"message":`,
		},
		TestAppFactory: fixtureAppWithUnitedRoutes,
	}
	scenario.Test(t)
}

func TestStatefileCollectionRejectsUnauthenticatedRequests(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:           "statefiles collection rejects unauthenticated requests",
		Method:         http.MethodGet,
		URL:            "/api/collections/statefiles/records",
		ExpectedStatus: http.StatusForbidden,
		ExpectedContent: []string{
			`"message":`,
		},
		TestAppFactory: fixtureAppWithUnitedRoutes,
	}
	scenario.Test(t)
}

func TestSuperuserCanInspectRetainedStateVersions(t *testing.T) {
	superuserToken := fixtureAuthToken(t, newFixtureTestApp(t), "_superusers", "test@example.com")
	scenario := tests.ApiScenario{
		Name:           "superuser can inspect retained state versions",
		Method:         http.MethodGet,
		URL:            "/api/collections/states/records",
		Headers:        map[string]string{"Authorization": superuserToken},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"items":`,
		},
		TestAppFactory: fixtureAppWithUnitedRoutes,
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			t.Helper()

			owner, err := app.FindAuthRecordByEmail("users", "user@example.com")
			require.NoError(t, err)
			groups, err := app.FindCollectionByNameOrId("groups")
			require.NoError(t, err)
			group := core.NewRecord(groups)
			group.Set("email", "state-api-tf@terraform.invalid")
			group.Set("username", "state-api-tf")
			group.SetPassword("state-api-password")
			group.Set("slug", "state-api")
			group.Set("displayName", "State API")
			group.Set("owner", owner.Id)
			require.NoError(t, app.Save(group))

			handler, err := e.Router.BuildMux()
			require.NoError(t, err)
			postFixtureState(t, handler, group, []byte(`{"version":4,"serial":1}`))
			postFixtureState(t, handler, group, []byte(`{"version":4,"serial":2}`))

			request := httptest.NewRequest(http.MethodDelete, stateURL(group, "network"), nil)
			request.SetBasicAuth(group.GetString("username"), "state-api-password")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, "response: %s", response.Body.String())
		},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			t.Helper()

			group, err := app.FindFirstRecordByFilter("groups", "slug = {:slug}", map[string]any{"slug": "state-api"})
			require.NoError(t, err)
			state, err := findState(app, group.Id, "network")
			require.NoError(t, err)
			require.False(t, state.GetDateTime("deletedAt").IsZero())
			require.NotEmpty(t, state.GetString("currentVersion"))

			currentVersion, err := app.FindRecordById("statefiles", state.GetString("currentVersion"))
			require.NoError(t, err)
			require.Equal(t, state.Id, currentVersion.GetString("state"))

			versions, err := app.FindRecordsByFilter("statefiles", "state = {:stateID}", "", 0, 0, map[string]any{"stateID": state.Id})
			require.NoError(t, err)
			require.Len(t, versions, 2)
		},
	}
	scenario.Test(t)
}

func fixtureAppWithUnitedRoutes(t testing.TB) *tests.TestApp {
	app := newFixtureTestApp(t)
	bindTestRoutes(t, app)
	return app
}

func postFixtureState(t testing.TB, handler http.Handler, group *core.Record, body []byte) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, stateURL(group, "network"), bytes.NewReader(body))
	request.SetBasicAuth(group.GetString("username"), "state-api-password")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, "response: %s", response.Body.String())
}
