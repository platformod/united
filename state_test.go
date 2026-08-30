// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFindStateScopesLookupToGroupAndName(t *testing.T) {
	app := newTestApp(t)
	owner := createUser(t, app)
	platform := createGroup(t, app, owner, "platform", "platform-tf", testPassword(t))
	operations := createGroup(t, app, owner, "operations", "operations-tf", testPassword(t))
	want := createState(t, app, platform, "network")
	createState(t, app, operations, "network")

	got, err := findState(app, platform.Id, "network")

	require.NoError(t, err)
	require.Equal(t, want.Id, got.Id)
}

func TestActiveLockUsesServerUTCAndExpiresAfter35Minutes(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")
	info := LockInfo{ID: "lock-1", Operation: "OperationTypeApply", Who: "terraform"}
	acquiredAt := time.Date(2026, 8, 29, 8, 0, 0, 0, time.FixedZone("PDT", -4*60*60))

	setLock(state, info, acquiredAt)

	require.Equal(t, "2026-08-29 12:35:00.000Z", state.GetDateTime("lockExpiresAt").String())

	active, expired, gotInfo, err := activeLock(state, time.Date(2026, 8, 29, 12, 34, 59, 0, time.UTC))
	require.NoError(t, err)
	require.True(t, active)
	require.False(t, expired)
	require.Equal(t, info, gotInfo)

	active, expired, _, err = activeLock(state, time.Date(2026, 8, 29, 12, 35, 0, 0, time.UTC))
	require.NoError(t, err)
	require.False(t, active)
	require.True(t, expired)
}

func TestActiveLockRejectsPartialOrInvalidStoredPayload(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")

	state.Set("lockID", "lock-1")
	active, expired, _, err := activeLock(state, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	require.Error(t, err)
	require.False(t, active)
	require.False(t, expired)

	state.Set("lockInfo", "{")
	state.Set("lockExpiresAt", "2026-08-29 12:35:00.000Z")
	active, expired, _, err = activeLock(state, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	require.Error(t, err)
	require.False(t, active)
	require.False(t, expired)
}

func TestClearExpiredLockPersistsTheClearedFields(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")
	setLock(state, LockInfo{ID: "lock-1"}, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))

	cleared, err := clearExpiredLock(app, state, time.Date(2026, 8, 29, 12, 35, 0, 0, time.UTC))

	require.NoError(t, err)
	require.True(t, cleared)
	persisted := findRecord(t, app, "states", state.Id)
	require.Empty(t, persisted.GetString("lockID"))
	require.True(t, persisted.GetDateTime("lockExpiresAt").IsZero())
	active, expired, _, err := activeLock(persisted, time.Date(2026, 8, 29, 12, 35, 0, 0, time.UTC))
	require.NoError(t, err)
	require.False(t, active)
	require.False(t, expired)
}

func TestClearLockClearsAllLockFields(t *testing.T) {
	app := newTestApp(t)
	group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
	state := createState(t, app, group, "network")
	setLock(state, LockInfo{ID: "lock-1"}, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))

	clearLock(state)

	require.Empty(t, state.GetString("lockID"))
	require.True(t, state.GetDateTime("lockExpiresAt").IsZero())
	active, expired, _, err := activeLock(state, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.False(t, active)
	require.False(t, expired)
}
