// SPDX-License-Identifier: MPL-2.0

package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"
)

func registerHooks(app core.App, cfg Config) {
	registerGroupHooks(app, cfg)
	registerStateHooks(app)
	registerStatefileHooks(app)
}

func registerGroupHooks(app core.App, cfg Config) {
	app.OnRecordCreate("groups").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			wrappedKey, err := GenerateWrappedGroupKey(cfg.StateMasterKey)
			if err != nil {
				return err
			}

			e.Record.Set("wrappedStateKey", wrappedKey)

			return e.Next()
		},
	})

	app.OnRecordUpdate("groups").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			for _, field := range []string{"slug", "username", "owner", "wrappedStateKey"} {
				if e.Record.GetString(field) != e.Record.Original().GetString(field) {
					return errors.New("immutable group identity field changed")
				}
			}
			if !e.Record.Original().GetDateTime("deletedAt").IsZero() {
				return errors.New("deleted group cannot be modified")
			}

			return e.Next()
		},
	})

	registerGroupRequestHooks(app)
}

func registerGroupRequestHooks(app core.App) {
	app.OnRecordCreateRequest("groups").BindFunc(createGroupRequest)
	app.OnRecordUpdateRequest("groups").BindFunc(updateGroupRequest)
	app.OnRecordDeleteRequest("groups").BindFunc(deleteGroupRequest)
}

func createGroupRequest(e *core.RecordRequestEvent) error {
	if err := requireGroupUser(e); err != nil {
		return err
	}

	e.Record.Set("owner", e.Auth.Id)

	return e.Next()
}

func updateGroupRequest(e *core.RecordRequestEvent) error {
	if err := requireGroupOwner(e); err != nil {
		return err
	}

	if !e.Record.GetDateTime("deletedAt").IsZero() {
		return e.BadRequestError("Deleted group cannot be modified.", nil)
	}

	return e.Next()
}

func deleteGroupRequest(e *core.RecordRequestEvent) error {
	if err := requireGroupOwner(e); err != nil {
		return err
	}

	if !e.Record.GetDateTime("deletedAt").IsZero() {
		return e.NoContent(http.StatusNoContent)
	}

	e.Record.Set("deletedAt", types.NowDateTime())

	if err := e.App.Save(e.Record); err != nil {
		return e.BadRequestError("Failed to delete group.", err)
	}

	return e.NoContent(http.StatusNoContent)
}

func requireGroupOwner(e *core.RecordRequestEvent) error {
	if err := requireGroupUser(e); err != nil || e.Record.GetString("owner") != e.Auth.Id {
		return e.ForbiddenError("Group ownership is required.", nil)
	}

	return nil
}

func requireGroupUser(e *core.RecordRequestEvent) error {
	if e.Auth == nil || e.Auth.Collection().Name != "users" {
		return e.ForbiddenError("User authentication is required.", nil)
	}

	return nil
}

func registerStateHooks(app core.App) {
	app.OnRecordCreate("states").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			if err := validateStateRecord(e); err != nil {
				return err
			}

			return e.Next()
		},
	})

	app.OnRecordUpdate("states").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			for _, field := range []string{"group", "name"} {
				if e.Record.GetString(field) != e.Record.Original().GetString(field) {
					return errors.New("immutable state identity field changed")
				}
			}
			if e.Record.Original().GetString("deletedAt") != "" {
				return errors.New("deleted state cannot be modified")
			}

			if err := validateStateRecord(e); err != nil {
				return err
			}

			return e.Next()
		},
	})
}

func registerStatefileHooks(app core.App) {
	app.OnRecordCreate("statefiles").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			state, err := e.App.FindRecordById("states", e.Record.GetString("state"))
			if err != nil {
				return err
			}

			if state.GetString("group") != e.Record.GetString("group") {
				return errors.New("statefile group must match state group")
			}

			return e.Next()
		},
	})

	app.OnRecordUpdate("statefiles").Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			return errors.New("statefiles cannot be modified")
		},
	})
}

func validateStateRecord(e *core.RecordEvent) error {
	currentVersion := e.Record.GetString("currentVersion")
	if currentVersion != "" {
		statefile, err := e.App.FindRecordById("statefiles", currentVersion)
		if err != nil {
			return err
		}

		if statefile.GetString("state") != e.Record.Id {
			return errors.New("current version must belong to state")
		}
	}

	lockFieldsPresent := 0
	if e.Record.GetString("lockID") != "" {
		lockFieldsPresent++
	}

	lockInfo := strings.TrimSpace(e.Record.GetString("lockInfo"))
	if lockInfo != "" && lockInfo != "null" {
		lockFieldsPresent++
	}

	if !e.Record.GetDateTime("lockExpiresAt").IsZero() {
		lockFieldsPresent++
	}

	if lockFieldsPresent != 0 && lockFieldsPresent != 3 {
		return errors.New("state lock fields must be all empty or all present")
	}

	return nil
}
