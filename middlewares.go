// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func UnitedSetup() gin.HandlerFunc {
	s3c, _ := newS3Client(cfg.KeyArn, cfg.Dev)
	rc := newRedisClient(cfg.RedisConn)

	return func(c *gin.Context) {
		c.Set("s3c", s3c)
		c.Set("rc", rc)
		c.Set("prefix", cfg.BucketPrefix)
	}
}

func UnitedBasicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		hdr := c.GetHeader("Authorization")
		if hdr == "" {
			c.Header("WWW-Authenticate", "Authorization Required")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		decodedAuth, _ := base64.StdEncoding.DecodeString(strings.Split(hdr, " ")[1])
		auth := strings.Split(string(decodedAuth), ":")

		// ============= WARNING: TRICKERY AHEAD =============
		// If ValidateAuth is true, we'll try to pass though auth via HTTP with the given creds
		//   This lets us validate creds separately and have a single home for them.  Do this.
		// If ValidateAuth is false, We'll update the S3 prefix with the SHA of BucketPrefix-password-group.
		//   This gives some uniqueness to the path and means that it should be hard to guess / collide from the outside
		//   but it is not secure and should only be used in trustworthy or non prod environments
		//   NOTE THAT IF YOU CHANGE THE PASSWORD OR BUCKET PREFIX YOUR STATE GOES TO A NEW PLACE AND YOU LOOSE THE OLD ONE
		if cfg.ValidateAuth {
			type AuthBody struct {
				Identity string `json:"identity"`
				Password string `json:"password"`
			}
			body, err := json.Marshal(AuthBody{Identity: auth[0], Password: auth[1]})
			if err != nil {
				c.Header("WWW-Authenticate", "Authorization Required")
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}

			res, err := http.Post(cfg.AuthURL, "application/json", bytes.NewReader(body))
			if err != nil || res.StatusCode != 200 {
				c.Header("WWW-Authenticate", "Authorization Required")
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Set("prefix", fmt.Sprintf("%s-%s", cfg.BucketPrefix, auth[0]))
		} else {
			group := c.Param("group")
			key := fmt.Sprintf("%s-%s-%s", cfg.BucketPrefix, auth[1], group)
			c.Set("prefix", fmt.Sprintf("%x", sha256.Sum256([]byte(key))))
		}
		c.Set("user", auth[0])
	}
}
