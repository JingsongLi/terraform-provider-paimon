// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

terraform {
  required_providers {
    paimon = {
      source = "apache/paimon"
    }
  }
}

variable "dlf_access_key_id" {
  type      = string
  sensitive = true
}

variable "dlf_access_key_secret" {
  type      = string
  sensitive = true
}

variable "dlf_security_token" {
  type      = string
  sensitive = true
  default   = null
}

provider "paimon" {
  uri            = "https://dlfnext.cn-hangzhou.aliyuncs.com"
  warehouse      = "my_catalog"
  token_provider = "dlf"

  dlf_access_key_id     = var.dlf_access_key_id
  dlf_access_key_secret = var.dlf_access_key_secret
  dlf_security_token    = var.dlf_security_token
}
