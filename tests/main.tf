terraform {
  backend "http" {}
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
