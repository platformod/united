// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/base64"
	"errors"
	"os"
)

type Config struct {
	StateMasterKey []byte
}

func LoadConfig() (Config, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("UNITED_STATE_MASTER_KEY"))
	if err != nil || len(key) != 32 {
		return Config{}, errors.New("UNITED_STATE_MASTER_KEY must be a base64-encoded 32-byte key")
	}

	return Config{StateMasterKey: key}, nil
}

// LockInfo: Shape of the Lock info TF gives us in LOCK and UNLOCK.
type LockInfo struct {
	// "Created": "2024-02-05T20:04:43.120857Z",
	Created string `json:"Created"`
	// "ID": "5b64957f-e4d3-8820-77a2-913e4a8a10bd",
	ID string `json:"ID"`
	// "Info": "",
	Info string `json:"Info"`
	// "Operation": "OperationTypePlan",
	Operation string `json:"Operation"`
	// "Path": "",
	Path string `json:"Path"`
	// "Version": "1.7.2",
	Version string `json:"Version"`
	// "Who": "nhruby@newhope.local"
	Who string `json:"Who"`
}
