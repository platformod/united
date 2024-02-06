# United: A multitenant Terraform HTTP backend server

Designed to be run alongside [Atlantis](https://www.runatlantis.io/), United offers a simple to configure HTTP backend with locking, encryption, and flexible pass though authentication.  

You would want to use this in situations where you have many teams with many statefiles and and consistent management of them across the platform/org/agency has become a burden.

## Requirements

- S3 Bucket
- KMS Key
- Redis
- Something to run this server
- Ideally a set of creds per team that can be stored separately

## Server Config

Config is done though environment variables

```bash
######
# Required
######

# S3 Bucket to store into
export BUCKET="myorg-bucket-of-states"

# KMS Key Arn to use for encrypting
export KEY_ARN="arn:aws:kms:us-whoop-9:11111111111:key/dork-4242-9999-be3p-c0ffeec0ffee"

######
# Optional
######

# Port to listen on, defaults to 8080
export PORT="4242"

# Key prefix for S3.  This has security implications, see warning in code before changing this, defaults to "united"
export BUCKET_PREFIX="yas"

# Redis connection string for lock storage, defaults to redis://localhost:6379
export REDIS_CONN="redis://meept:woah@the_elasticache_cluster:6379"

# Enable passthough auth to AuthURL via POST, defaults to true
export VALIDATE_AUTH="true"

# URL to POST to, should return 200 for success.  Defaults to http://localhost:8090/api/collections/united/auth-with-password
export AUTH_URL="https://inside-api/api/united-states-postage"

```

## Terrafrom Config

Config is done via the [Terrafrom http backend](https://developer.hashicorp.com/terraform/language/settings/backends/http).  United uses the `/state` path and binds `/state/:group/:name` which you should provide.

An example is below:

```terraform
terraform {
  backend "http" {
    address        = "https://united.my.org/state/my-group/this-state"
    lock_address   = "https://united.my.org/state/my-group/this-state"
    unlock_address = "https://united.my.org/state/my-group/this-state"
  }
  required_providers {
    random = {
      source = "hashicorp/random"
      version = "3.6.0"
    }
  }
}
```

Then run by setting the username and password via env vars:

```bash
export TF_HTTP_USERNAME="daryl"
export TF_HTTP_PASSWORD="Ih0p3Th1$w0rKz"
terraform init
terraform plan
```

Note that Atlantis can populate env vars via script, which is the ideal tool to either fetch creds from a secret store, or decrypt on the fly from a encrypted blob in the repo.

## Runtime Notes

- Protect s3 bucket as with anything.  Data is encrypted vis [AWS S3-CSE](https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingClientSideEncryption.html) but care should still be taken to prevent leakage.  Same this with the KMS key.

- Do network isolation.  This service is meant to be interfaced by terrafrom, not the entire internet. Ideally run this next to Atlantis and only allow network traffic from it.

- Use the auth validation.  This was designed to auth against [the PocketBase API](https://pocketbase.io/docs/api-records/#auth-with-password) but the format is simple enough to write a shim in front of another source if needed.  The alternative method is basically just a hackaround for testing purposes

- Enable TlS on fronting proxies, redis, auth url, etc...

## F.A.Q.

- Why united?
  - Because it's the United States for Atlantis.


## Copyright

Copyright (C) 2024 Platform OnDemand, Inc - All Rights Reserved

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
