// SPDX-License-Identifier: MPL-2.0

package main

var cfg Config

// Gin also needs PORT, defaults to 8080
type Config struct {
	// S3 Bucket to store into
	Bucket string `env:"BUCKET,required"`
	// Key prefix for bucket.  When ValidateAuth is false, we use this value as a salt
	BucketPrefix string `env:"BUCKET_PREFIX,default=united"`
	// KMS Key Arn to use for S3-CSE
	KeyArn string `env:"KEY_ARN,required"`
	// Redis connection string for locking
	RedisConn string `env:"REDIS_CONN,default=redis://localhost:6379"`
	// When true, will POST creds to AuthUrl to try to auth
	ValidateAuth bool `env:"VALIDATE_AUTH,default=true"`
	// URL to POST to, should return 200 for success.  Body is JSON: '{"identity": "USER", "password": "PASS"}'
	AuthURL string `env:"AUTH_URL,default=http://localhost:8090/api/collections/united/auth-with-password"`
	//Dev flag to alter some config for localstack
	Dev bool `env:"DEV,default=false"`
}

// Shape of the Lock info TF gives us in LOCK and UNLOCK
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
