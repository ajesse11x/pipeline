// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"github.com/spf13/viper"
	"github.com/uber-common/bark"
	"github.com/uber-common/bark/zbark"
	"go.uber.org/cadence/client"

	"github.com/banzaicloud/pipeline/internal/platform/cadence"
)

func newCadenceConfig() cadence.Config {
	return cadence.Config{
		Host:   viper.GetString("cadence.host"),
		Port:   viper.GetInt("cadence.port"),
		Domain: viper.GetString("cadence.domain"),
	}
}

// CadenceClient returns a new cadence client.
func CadenceClient() (client.Client, error) {
	return cadence.NewClient(newCadenceConfig(), zbark.Zapify(bark.NewLoggerFromLogrus(Logger()).WithField("component", "cadence-client")))
}
