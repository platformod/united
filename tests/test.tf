terraform {
  backend "http" {
    address        = "http://localhost:8080/state/my-peoples/me"
    lock_address   = "http://localhost:8080/state/my-peoples/me"
    unlock_address = "http://localhost:8080/state/my-peoples/me"
  }
  required_providers {
    random = {
      source = "hashicorp/random"
      version = "3.6.0"
    }
  }
}

variable "changer" {
    type    = string
    default = "foo"
}

resource "random_pet" "whee" {
    keepers = {
        changer = var.changer
    }
}
