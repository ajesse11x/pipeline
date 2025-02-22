// Copyright © 2018 Banzai Cloud
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

package verify

import (
	"github.com/sirupsen/logrus"

	"github.com/banzaicloud/pipeline/config"
)

// Note: this should be FieldLogger instead.
// Debug mode should be split to a separate config.
// nolint: gochecknoglobals
var log *logrus.Logger

func init() {
	log = config.Logger()
}
