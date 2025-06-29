// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"log"

	"github.com/aws/amazon-s3-encryption-client-go/v3/client"
	"github.com/aws/amazon-s3-encryption-client-go/v3/materials"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/redis/go-redis/v9"
)

func newS3Client(keyArn string, isDev bool) (*client.S3EncryptionClientV3, error) {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = isDev
	})
	kmsClient := kms.NewFromConfig(cfg)
	kmsKeyArn := keyArn

	cmm, err := materials.NewCryptographicMaterialsManager(
		materials.NewKmsKeyring(
			kmsClient,
			kmsKeyArn,
			func(options *materials.KeyringOptions) {
				options.EnableLegacyWrappingAlgorithms = false
			}),
	)
	if err != nil {
		log.Fatal("error while creating new CMM")
	}

	return client.New(s3Client, cmm)
}

func newRedisClient(conn string) *redis.Client {
	opts, err := redis.ParseURL(conn)
	if err != nil {
		log.Fatal(err)
	}

	client := redis.NewClient(opts)

	return client
}
