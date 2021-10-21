// Copyright (c) 2021 Doc.ai and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package config contains config and helper functions for configuration cmd-nsc-init
package config

import (
	"net/url"
	"time"
)

// Config - configuration for cmd-nsc-init
type Config struct {
	Name             string        `default:"cmd-nsc-init" desc:"Name of the client" split_words:"true"`
	DialTimeout      time.Duration `default:"5s" desc:"timeout to dial NSMgr" split_words:"true"`
	RequestTimeout   time.Duration `default:"15s" desc:"timeout to request NSE" split_words:"true"`
	RetryTimeout     time.Duration `default:"0s" desc:"retry timeout" split_words:"true"`
	RetryInterval    time.Duration `default:"100ms" desc:"retry interval" split_words:"true"`
	ConnectTo        url.URL       `default:"unix:///var/lib/networkservicemesh/nsm.io.sock" desc:"url to connect to" split_words:"true"`
	MaxTokenLifetime time.Duration `default:"10m" desc:"maximum lifetime of tokens" split_words:"true"`
	NetworkServices  []url.URL     `default:"" desc:"A list of Network Service Requests" split_words:"true"`
	LogLevel         string        `default:"INFO" desc:"Log level" split_words:"true"`
}
