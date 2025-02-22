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

package workflow

import (
	"context"

	"github.com/banzaicloud/pipeline/internal/clusterfeature"
)

const ClusterFeatureSetSpecActivityName = "cluster-feature-set-spec"

type ClusterFeatureSetSpecActivityInput struct {
	ClusterID   uint
	FeatureName string
	Spec        map[string]interface{}
}

type ClusterFeatureSetSpecActivity struct {
	features clusterfeature.FeatureRepository
}

func MakeClusterFeatureSetSpecActivity(features clusterfeature.FeatureRepository) ClusterFeatureSetSpecActivity {
	return ClusterFeatureSetSpecActivity{
		features: features,
	}
}

func (a ClusterFeatureSetSpecActivity) Execute(ctx context.Context, input ClusterFeatureSetSpecActivityInput) error {
	return a.features.UpdateFeatureSpec(ctx, input.ClusterID, input.FeatureName, input.Spec)
}
