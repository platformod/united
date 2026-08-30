// SPDX-License-Identifier: MPL-2.0

package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const groupOwnerRule = "owner.id = @request.auth.id"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		groups, err := txApp.FindCollectionByNameOrId(groupsCollectionID)
		if err != nil {
			return err
		}

		groups.Fields.Add(&core.DateField{Name: "deletedAt"})
		groups.ListRule = types.Pointer(groupOwnerRule)
		groups.ViewRule = types.Pointer(groupOwnerRule)
		groups.CreateRule = types.Pointer("@request.auth.id != ''")
		groups.UpdateRule = types.Pointer(groupOwnerRule)
		groups.DeleteRule = types.Pointer(groupOwnerRule)

		return txApp.Save(groups)
	}, func(txApp core.App) error {
		groups, err := txApp.FindCollectionByNameOrId(groupsCollectionID)
		if err != nil {
			return err
		}

		groups.Fields.RemoveByName("deletedAt")
		groups.ListRule = nil
		groups.ViewRule = nil
		groups.CreateRule = nil
		groups.UpdateRule = nil
		groups.DeleteRule = nil

		return txApp.Save(groups)
	})
}
