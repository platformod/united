// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/amazon-s3-encryption-client-go/v3/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/bsm/redislock"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func getHandler(c *gin.Context) {
	group := c.Param("group")
	name := c.Param("name")

	prefix := c.MustGet("prefix")
	s3c := c.MustGet("s3c").(*client.S3EncryptionClientV3)

	// nrh: As far as I can tell, there's no locking for GET requests?
	o, err := s3c.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(fmt.Sprintf("%s/%s/%s", prefix, group, name)),
	})

	// We care about which error here, since no state should 404,
	// which TF sees as success for new states
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.(type) {
		case *s3types.NoSuchKey, *s3types.NotFound:
			c.JSON(http.StatusNotFound, gin.H{"message": "Not Found"})
			return
		default:
			//nolint:errcheck
			c.Error(err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Could not retrieve from storage"})
			return
		}
	} else if err != nil {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Could not retrieve from storage"})
		return
	}

	defer o.Body.Close()
	len, _ := strconv.Atoi(o.Metadata["x-amz-unencrypted-content-length"])
	c.DataFromReader(http.StatusOK, int64(len), *o.ContentType, o.Body, nil)
}

func postHandler(c *gin.Context) {
	group := c.Param("group")
	name := c.Param("name")
	id := c.Param("ID")

	var storedLock LockInfo

	prefix := c.MustGet("prefix")
	s3c := c.MustGet("s3c").(*client.S3EncryptionClientV3)
	rc := c.MustGet("rc").(*redis.Client)
	lc := redislock.New(rc)

	lockBase := fmt.Sprintf("%s/%s/%s", prefix, group, name)

	// lock if someone passes us an id,
	// otherwise assume that someone used -lock=false and go nuts
	if id != "" {
		lock, err := lc.Obtain(c, lockBase, 5*time.Second, nil)
		if err == redislock.ErrNotObtained {
			//nolint:errcheck
			c.Error(err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Lock failed at inital step"})
			return
		}
		//nolint:errcheck
		defer lock.Release(c)

		stored, err := rc.Get(context.Background(), lockBase+"-xreq").Result()
		if err != nil {
			//nolint:errcheck
			c.Error(err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Failed to retrieve lock info"})
			return
		}
		_ = json.Unmarshal([]byte(stored), &storedLock)

		if id != storedLock.ID {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Locked by different ID", "ID": storedLock.ID})
			return
		}
	}

	body, _ := io.ReadAll(c.Request.Body)
	_, err := s3c.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.Bucket),
		Key:         aws.String(fmt.Sprintf("%s/%s/%s", prefix, group, name)),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})

	if err != nil {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Failed to store state"})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}

// Unsure where TF calls this...
func deleteHandler(c *gin.Context) {
	group := c.Param("group")
	name := c.Param("name")

	prefix := c.MustGet("prefix")
	s3c := c.MustGet("s3c").(*client.S3EncryptionClientV3)

	_, err := s3c.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(fmt.Sprintf("%s/%s/%s", prefix, group, name)),
	})

	if err != nil {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Failed to delete state"})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
}

func lockHandler(c *gin.Context) {
	group := c.Param("group")
	name := c.Param("name")

	var storedLock LockInfo
	var reqLock LockInfo
	_ = c.BindJSON(&reqLock)

	prefix := c.MustGet("prefix")
	rc := c.MustGet("rc").(*redis.Client)
	lc := redislock.New(rc)

	lockBase := fmt.Sprintf("%s/%s/%s", prefix, group, name)

	// Get outer mutex for operations on the cross request lock
	lock, err := lc.Obtain(c, lockBase, 5*time.Second, nil)
	if err == redislock.ErrNotObtained {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Lock failed at inital step"})
		return
	}
	//nolint:errcheck
	defer lock.Release(c)

	stored, err := rc.Get(context.Background(), lockBase+"-xreq").Result()
	if err != redis.Nil {
		err = json.Unmarshal([]byte(stored), &storedLock)
		if err != nil {
			c.JSON(http.StatusLocked, gin.H{"message": "Already Locked"})
			return
		} else {
			c.JSON(http.StatusLocked, gin.H{"message": "Already Locked", "ID": storedLock.ID})
			return
		}
	}

	reqLockStr, _ := json.Marshal(reqLock)
	err = rc.Set(context.Background(), lockBase+"-xreq", reqLockStr, 35*time.Minute).Err()
	if err != nil {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Lock failed"})
		return
	} else {
		c.JSON(http.StatusOK, reqLock.ID)
		return
	}
}

func unlockHandler(c *gin.Context) {
	group := c.Param("group")
	name := c.Param("name")

	var storedLock LockInfo
	var reqLock LockInfo
	_ = c.BindJSON(&reqLock)

	prefix := c.MustGet("prefix")
	rc := c.MustGet("rc").(*redis.Client)
	lc := redislock.New(rc)

	lockBase := fmt.Sprintf("%s/%s/%s", prefix, group, name)

	lock, err := lc.Obtain(context.Background(), fmt.Sprintf("%s/%s/%s", prefix, group, name), 5*time.Second, nil)
	if err == redislock.ErrNotObtained {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Unlock failed at inital step"})
		return
	}
	//nolint:errcheck
	defer lock.Release(c)

	stored, err := rc.Get(context.Background(), lockBase+"-xreq").Result()
	if err == redis.Nil {
		// Since locks expire in Redis, it's possible that this condition will happen, and that's probably ok
		c.JSON(http.StatusOK, gin.H{"message": "Lock Not Found. Expired. Probably."})
		return
	}
	_ = json.Unmarshal([]byte(stored), &storedLock)

	if reqLock.ID != storedLock.ID {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Locked by different ID", "ID": storedLock.ID})
		return
	}

	err = rc.Del(context.Background(), lockBase+"-xreq").Err()
	if err != nil {
		//nolint:errcheck
		c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Unlock failed"})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}
