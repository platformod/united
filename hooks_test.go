// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
)

func TestGroupCreationGeneratesKeyAndRejectsIdentityChanges(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", "correct horse")
	require.NotEmpty(t, group.GetString("wrappedStateKey"))

	group.Set("slug", "renamed")
	require.Error(t, app.Save(group))
}

func TestStateIdentityCannotChange(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", "correct horse")
	state := createState(t, app, group, "network")

	state.Set("name", "renamed")
	require.Error(t, app.Save(state))
}

func TestStatefileMustBelongToStatesGroup(t *testing.T) {
	app := newTestApp(t)
	owner := createUser(t, app)
	stateGroup := createGroup(t, app, owner, "platform", "platform-tf", "correct horse")
	otherGroup := createGroup(t, app, owner, "operations", "operations-tf", "correct horse")
	state := createState(t, app, stateGroup, "network")

	statefile := newStatefile(t, app, state, otherGroup)
	require.Error(t, app.Save(statefile))
}

func TestStatefileCannotChange(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", "correct horse")
	statefile := createStatefile(t, app, createState(t, app, group, "network"), group)

	statefile.Set("contentType", "application/octet-stream")
	require.Error(t, app.Save(statefile))
}

func TestStateCurrentVersionMustBelongToState(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", "correct horse")
	state := createState(t, app, group, "network")
	otherState := createState(t, app, group, "database")
	otherVersion := createStatefile(t, app, otherState, group)

	state.Set("currentVersion", otherVersion.Id)
	require.Error(t, app.Save(state))
}

func TestStateLockFieldsMustBeAllEmptyOrAllPresent(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", "correct horse")
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
