// SPDX-License-Identifier: MPL-2.0

package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestFixtureTestAppUsesIsolatedCopy(t *testing.T) {
	first := newFixtureTestApp(t)
	second := newFixtureTestApp(t)

	firstFixtureUser, err := first.FindAuthRecordByEmail("users", "user@example.com")
	require.NoError(t, err)
	secondFixtureUser, err := second.FindAuthRecordByEmail("users", "user@example.com")
	require.NoError(t, err)
	require.Equal(t, firstFixtureUser.Id, secondFixtureUser.Id)

	created := createUserWithEmail(t, first, "isolated@example.test")
	firstUser, err := first.FindAuthRecordByEmail("users", created.GetString("email"))
	require.NoError(t, err)
	require.Equal(t, created.Id, firstUser.Id)

	_, err = second.FindAuthRecordByEmail("users", created.GetString("email"))
	require.Error(t, err)
}

const testDataDir = "test_pb_data"

func newFixtureTestApp(t testing.TB) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp(testDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}

func fixtureAuthToken(t testing.TB, app core.App, collection, email string) string {
	t.Helper()

	record, err := app.FindAuthRecordByEmail(collection, email)
	require.NoError(t, err)
	token, err := record.NewAuthToken()
	require.NoError(t, err)

	return token
}

func bindTestRoutes(t testing.TB, app core.App) http.Handler {
	t.Helper()

	cfg := Config{StateMasterKey: make([]byte, 32)}
	registerHooks(app, cfg)
	registerRoutes(app, cfg)

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

func newTestApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()

	app := newApp(Config{StateMasterKey: make([]byte, 32)}, t.TempDir())
	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())
	t.Cleanup(func() {
		require.NoError(t, app.ResetBootstrapState())
	})

	return app
}

var (
	testPasswordOnce  sync.Once
	testPasswordValue string
)

func testPassword(t *testing.T) string {
	t.Helper()

	testPasswordOnce.Do(func() {
		value := make([]byte, 24)
		_, err := rand.Read(value)
		require.NoError(t, err)
		testPasswordValue = base64.RawStdEncoding.EncodeToString(value)
	})

	return testPasswordValue
}

func invalidTestPassword(t *testing.T) string {
	t.Helper()

	return testPassword(t) + "-invalid"
}

func invalidTestUsername(t *testing.T) string {
	t.Helper()

	return "invalid-" + testPassword(t)
}

func createUser(t *testing.T, app core.App) *core.Record {
	t.Helper()

	return createUserWithEmail(t, app, "owner@example.test")
}

func createUserWithEmail(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(collection)
	user.Set("email", email)
	user.SetPassword(testPassword(t))
	require.NoError(t, app.Save(user))
	return user
}

func createGroup(t *testing.T, app core.App, owner *core.Record, slug, username, password string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("groups")
	require.NoError(t, err)
	group := core.NewRecord(collection)
	group.Set("email", username+"@terraform.invalid")
	group.Set("username", username)
	group.Set("slug", slug)
	group.Set("displayName", slug)
	group.Set("owner", owner.Id)
	group.SetPassword(password)
	require.NoError(t, app.Save(group))
	return group
}
