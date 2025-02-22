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

package clusterfeature

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/banzaicloud/pipeline/internal/common/commonadapter"
)

func TestFeatureService_List(t *testing.T) {
	clusterID := uint(1)
	repository := NewInMemoryFeatureRepository(map[uint][]Feature{
		clusterID: {
			{
				Name: "myActiveFeature",
				Spec: FeatureSpec{
					"someSpecKey": "someSpecValue",
				},
				Status: FeatureStatusActive,
			},
			{
				Name: "myPendingFeature",
				Spec: FeatureSpec{
					"mySpecKey": "mySpecValue",
				},
				Status: FeatureStatusPending,
			},
			{
				Name: "myErrorFeature",
				Spec: FeatureSpec{
					"mySpecKey": "mySpecValue",
				},
				Status: FeatureStatusError,
			},
		},
	})
	registry := MakeFeatureManagerRegistry([]FeatureManager{
		&dummyFeatureManager{
			TheName: "myInactiveFeature",
			Output: FeatureOutput{
				"someOutputKey": "someOutputValue",
			},
		},
		&dummyFeatureManager{
			TheName: "myPendingFeature",
			Output: FeatureOutput{
				"someOutputKey": "someOutputValue",
			},
		},
		&dummyFeatureManager{
			TheName: "myActiveFeature",
			Output: FeatureOutput{
				"someOutputKey": "someOutputValue",
			},
		},
		&dummyFeatureManager{
			TheName: "myErrorFeature",
			Output: FeatureOutput{
				"someOutputKey": "someOutputValue",
			},
		},
	})
	expected := []Feature{
		{
			Name:   "myActiveFeature",
			Status: FeatureStatusActive,
		},
		{
			Name:   "myPendingFeature",
			Status: FeatureStatusPending,
		},
		{
			Name:   "myErrorFeature",
			Status: FeatureStatusError,
		},
	}
	logger := commonadapter.NewNoopLogger()
	service := MakeFeatureService(nil, registry, repository, logger)

	features, err := service.List(context.Background(), clusterID)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, features)
}

func TestFeatureService_Details(t *testing.T) {
	clusterID := uint(1)
	registry := MakeFeatureManagerRegistry([]FeatureManager{
		&dummyFeatureManager{
			TheName: "myActiveFeature",
			Output: FeatureOutput{
				"myOutputKey": "myOutputValue",
			},
		},
		&dummyFeatureManager{
			TheName: "myInactiveFeature",
			Output: FeatureOutput{
				"myOutputKey": "myOutputValue",
			},
		},
	})
	repository := NewInMemoryFeatureRepository(map[uint][]Feature{
		clusterID: {
			{
				Name: "myActiveFeature",
				Spec: FeatureSpec{
					"mySpecKey": "mySpecValue",
				},
				Status: FeatureStatusActive,
			},
		},
	})
	logger := commonadapter.NewNoopLogger()
	service := MakeFeatureService(nil, registry, repository, logger)

	cases := map[string]struct {
		FeatureName string
		Result      Feature
		Error       error
	}{
		"active feature": {
			FeatureName: "myActiveFeature",
			Result: Feature{
				Name: "myActiveFeature",
				Spec: FeatureSpec{
					"mySpecKey": "mySpecValue",
				},
				Output: FeatureOutput{
					"myOutputKey": "myOutputValue",
				},
				Status: FeatureStatusActive,
			},
		},
		"inactive feature": {
			FeatureName: "myInactiveFeature",
			Result: Feature{
				Name:   "myInactiveFeature",
				Status: FeatureStatusInactive,
			},
		},
		"unknown feature": {
			FeatureName: "myUnknownFeature",
			Error: UnknownFeatureError{
				FeatureName: "myUnknownFeature",
			},
		},
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			feature, err := service.Details(context.Background(), clusterID, tc.FeatureName)
			switch tc.Error {
			case nil:
				require.NoError(t, err)
				assert.Equal(t, tc.Result, feature)
			default:
				assert.Error(t, err)
				assert.Equal(t, tc.Error, errors.Cause(err))
			}
		})
	}
}

func TestFeatureService_Activate(t *testing.T) {
	clusterID := uint(1)
	featureName := "myFeature"
	dispatcher := &dummyFeatureOperationDispatcher{}
	featureManager := &dummyFeatureManager{
		TheName: featureName,
		Output: FeatureOutput{
			"someKey": "someValue",
		},
	}
	registry := MakeFeatureManagerRegistry([]FeatureManager{featureManager})
	repository := NewInMemoryFeatureRepository(nil)
	logger := commonadapter.NewNoopLogger()
	service := MakeFeatureService(dispatcher, registry, repository, logger)

	cases := map[string]struct {
		FeatureName     string
		ValidationError error
		ApplyError      error
		Error           interface{}
		FeatureSaved    bool
	}{
		"success": {
			FeatureName:  featureName,
			FeatureSaved: true,
		},
		"unknown feature": {
			FeatureName: "notMyFeature",
			Error: UnknownFeatureError{
				FeatureName: "notMyFeature",
			},
		},
		"invalid spec": {
			FeatureName:     featureName,
			ValidationError: errors.New("validation error"),
			Error:           true,
		},
		"begin apply fails": {
			FeatureName: featureName,
			ApplyError:  errors.New("failed to begin apply"),
			Error:       true,
		},
	}
	spec := FeatureSpec{
		"mySpecKey": "mySpecValue",
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			repository.Clear()
			dispatcher.ApplyError = tc.ApplyError
			featureManager.ValidationError = tc.ValidationError

			err := service.Activate(context.Background(), clusterID, tc.FeatureName, spec)
			switch tc.Error {
			case true:
				assert.Error(t, err)
			case nil, false:
				assert.NoError(t, err)
			default:
				assert.Equal(t, tc.Error, errors.Cause(err))
			}

			if tc.FeatureSaved {
				assert.NotEmpty(t, repository.features[clusterID])
			} else {
				assert.Empty(t, repository.features[clusterID])
			}
		})
	}
}

func TestFeatureService_Deactivate(t *testing.T) {
	clusterID := uint(1)
	featureName := "myFeature"
	dispatcher := &dummyFeatureOperationDispatcher{}
	registry := MakeFeatureManagerRegistry([]FeatureManager{
		dummyFeatureManager{
			TheName: featureName,
			Output: FeatureOutput{
				"someKey": "someValue",
			},
		},
	})
	repository := NewInMemoryFeatureRepository(map[uint][]Feature{
		clusterID: {
			{
				Name: featureName,
				Spec: FeatureSpec{
					"mySpecKey": "mySpecValue",
				},
				Status: FeatureStatusActive,
			},
		},
	})
	snapshot := repository.Snapshot()
	logger := commonadapter.NewNoopLogger()
	service := MakeFeatureService(dispatcher, registry, repository, logger)

	cases := map[string]struct {
		FeatureName     string
		DeactivateError error
		Error           interface{}
		StatusAfter     FeatureStatus
	}{
		"success": {
			FeatureName: featureName,
			StatusAfter: FeatureStatusPending,
		},
		"unknown feature": {
			FeatureName: "notMyFeature",
			Error: UnknownFeatureError{
				FeatureName: "notMyFeature",
			},
			StatusAfter: FeatureStatusActive,
		},
		"begin deactivate fails": {
			FeatureName:     featureName,
			DeactivateError: errors.New("failed to begin deactivate"),
			Error:           true,
			StatusAfter:     FeatureStatusActive,
		},
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			repository.Restore(snapshot)
			dispatcher.DeactivateError = tc.DeactivateError

			err := service.Deactivate(context.Background(), clusterID, tc.FeatureName)
			switch tc.Error {
			case true:
				assert.Error(t, err)
			case nil, false:
				assert.NoError(t, err)
			default:
				assert.Equal(t, tc.Error, errors.Cause(err))
			}

			assert.Equal(t, tc.StatusAfter, repository.features[clusterID][featureName].Status)
		})
	}
}

func TestFeatureService_Update(t *testing.T) {
	clusterID := uint(1)
	featureName := "myFeature"
	dispatcher := &dummyFeatureOperationDispatcher{}
	featureManager := &dummyFeatureManager{
		TheName: featureName,
		Output: FeatureOutput{
			"someKey": "someValue",
		},
	}
	registry := MakeFeatureManagerRegistry([]FeatureManager{featureManager})
	repository := NewInMemoryFeatureRepository(map[uint][]Feature{
		clusterID: {
			{
				Name: featureName,
				Spec: FeatureSpec{
					"mySpecKey": "mySpecValue",
				},
				Status: FeatureStatusActive,
			},
		},
	})
	snapshot := repository.Snapshot()
	logger := commonadapter.NewNoopLogger()
	service := MakeFeatureService(dispatcher, registry, repository, logger)

	cases := map[string]struct {
		FeatureName     string
		ValidationError error
		ApplyError      error
		Error           interface{}
		StatusAfter     FeatureStatus
	}{
		"success": {
			FeatureName: featureName,
			StatusAfter: FeatureStatusPending,
		},
		"unknown feature": {
			FeatureName: "notMyFeature",
			Error: UnknownFeatureError{
				FeatureName: "notMyFeature",
			},
			StatusAfter: FeatureStatusActive,
		},
		"invalid spec": {
			FeatureName:     featureName,
			ValidationError: errors.New("validation error"),
			Error:           true,
			StatusAfter:     FeatureStatusActive,
		},
		"begin apply fails": {
			FeatureName: featureName,
			ApplyError:  errors.New("failed to begin apply"),
			Error:       true,
			StatusAfter: FeatureStatusActive,
		},
	}
	spec := FeatureSpec{
		"someSpecKey": "someSpecValue",
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			repository.Restore(snapshot)
			dispatcher.ApplyError = tc.ApplyError
			featureManager.ValidationError = tc.ValidationError

			err := service.Update(context.Background(), clusterID, tc.FeatureName, spec)
			switch tc.Error {
			case true:
				assert.Error(t, err)
			case nil, false:
				assert.NoError(t, err)
			default:
				assert.Equal(t, tc.Error, errors.Cause(err))
			}

			assert.Equal(t, tc.StatusAfter, repository.features[clusterID][featureName].Status)
		})
	}
}

type dummyFeatureOperationDispatcher struct {
	ApplyError      error
	DeactivateError error
}

func (d dummyFeatureOperationDispatcher) DispatchApply(ctx context.Context, clusterID uint, featureName string, spec FeatureSpec) error {
	return d.ApplyError
}

func (d dummyFeatureOperationDispatcher) DispatchDeactivate(ctx context.Context, clusterID uint, featureName string, spec FeatureSpec) error {
	return d.DeactivateError
}
