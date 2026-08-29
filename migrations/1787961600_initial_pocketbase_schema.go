// SPDX-License-Identifier: MPL-2.0

package migrations

import "github.com/pocketbase/pocketbase/core"

const (
	usersCollectionID      = "_pb_users_auth_"
	groupsCollectionID     = "unitedgroups001"
	statesCollectionID     = "unitedstates001"
	statefilesCollectionID = "unitedfiles0001"
)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		groups := core.NewAuthCollection("groups", groupsCollectionID)
		groups.PasswordAuth.Enabled = false
		groups.Fields.Add(
			&core.TextField{Name: "username", Required: true},
			&core.TextField{Name: "slug", Required: true},
			&core.TextField{Name: "displayName", Required: true},
			&core.TextField{Name: "wrappedStateKey", Required: true, Hidden: true},
			&core.RelationField{Name: "owner", CollectionId: usersCollectionID, Required: true, MinSelect: 1, MaxSelect: 1},
		)
		groups.AddIndex("idx_groups_username", true, "username", "")
		groups.AddIndex("idx_groups_slug", true, "slug", "")

		states := core.NewBaseCollection("states", statesCollectionID)
		states.Fields.Add(
			&core.RelationField{Name: "group", CollectionId: groups.Id, Required: true, MinSelect: 1, MaxSelect: 1},
			&core.TextField{Name: "name", Required: true},
			&core.DateField{Name: "deletedAt", Hidden: true},
			&core.TextField{Name: "lockID", Hidden: true},
			&core.JSONField{Name: "lockInfo", Hidden: true},
			&core.DateField{Name: "lockExpiresAt", Hidden: true},
		)
		states.AddIndex("idx_states_group_name", true, "group, name", "")

		statefiles := core.NewBaseCollection("statefiles", statefilesCollectionID)
		statefiles.Fields.Add(
			&core.RelationField{Name: "state", CollectionId: states.Id, Required: true, MinSelect: 1, MaxSelect: 1},
			&core.RelationField{Name: "group", CollectionId: groups.Id, Required: true, MinSelect: 1, MaxSelect: 1},
			&core.FileField{Name: "file", Required: true, Hidden: true, Protected: true, MaxSelect: 1},
			&core.NumberField{Name: "contentLength", Required: true, OnlyInt: true, Hidden: true},
			&core.TextField{Name: "contentType", Required: true, Hidden: true},
			&core.TextField{Name: "sha256", Required: true, Hidden: true},
		)

		for _, collection := range []*core.Collection{groups, states, statefiles} {
			if err := txApp.Save(collection); err != nil {
				return err
			}
		}

		states.Fields.Add(&core.RelationField{Name: "currentVersion", CollectionId: statefiles.Id, MaxSelect: 1})
		return txApp.Save(states)
	}, func(txApp core.App) error {
		for _, id := range []string{statefilesCollectionID, statesCollectionID, groupsCollectionID} {
			collection, err := txApp.FindCollectionByNameOrId(id)
			if err != nil {
				return err
			}
			if err := txApp.Delete(collection); err != nil {
				return err
			}
		}

		return nil
	})
}
