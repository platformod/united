// SPDX-License-Identifier: MPL-2.0

package main

import (
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	_ "github.com/platformod/united/migrations"
)

func NewApp(cfg Config) *pocketbase.PocketBase {
	return newApp(cfg, "")
}

func newApp(cfg Config, dataDir string) *pocketbase.PocketBase {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: dataDir})
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{})
	registerTestCommands(app)
	registerHooks(app, cfg)
	registerRoutes(app, cfg)

	return app
}

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	app := NewApp(cfg)
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
