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

package anchore

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"emperror.dev/errors"
	"github.com/antihax/optional"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/resty.v1"

	"github.com/banzaicloud/pipeline/.gen/anchore"
	"github.com/banzaicloud/pipeline/.gen/pipeline/pipeline"
	"github.com/banzaicloud/pipeline/internal/common"
)

type UserManagementClient interface {
	CreateAccount(ctx context.Context, accountName string, email string) error
	DeleteAccount(ctx context.Context, accountName string) error
	GetAccount(ctx context.Context, accountName string) (string, error)
	CreateUser(ctx context.Context, accountName string, userName string, password string) error
	DeleteUser(ctx context.Context, accountName string, userName string) error
	GetUser(ctx context.Context, userName string) (interface{}, error)
	GetUserCredentials(ctx context.Context, userName string) (string, error)
}

type ImagesClient interface {
	// GetImageVulnerabilities gets the vulnerabilities for the given image digest
	ScanImage(ctx context.Context, image pipeline.ClusterImage) (interface{}, error)
	// GetImageVulnerabilities gets the vulnerabilities for the given image digest
	GetImageVulnerabilities(ctx context.Context, imageDigest string) (interface{}, error)
	// CheckImage cheks rthe image for anchore metadata
	CheckImage(ctx context.Context, imageDigest string) (interface{}, error)
}

type PolicyClient interface {
	GetPolicy(ctx context.Context, policyID string) (interface{}, error)
	ListPolicies(ctx context.Context) (interface{}, error)
	CreatePolicy(ctx context.Context, policy pipeline.PolicyBundleRecord) (interface{}, error)
	DeletePolicy(ctx context.Context, policyID string) error
	UpdatePolicy(ctx context.Context, policyID string, activate bool) error
}

// AnchoreClient "facade" for supported Anchore operations, decouples anchore specifics from the application
type AnchoreClient interface {
	UserManagementClient
	ImagesClient
	PolicyClient
}

type anchoreClient struct {
	userName string
	password string
	endpoint string
	logger   common.Logger
}

func NewAnchoreClient(userName string, password string, endpoint string, logger common.Logger) AnchoreClient {
	return anchoreClient{
		userName: userName,
		password: password,
		endpoint: endpoint,
		logger:   logger.WithFields(map[string]interface{}{"anchore-client": ""}),
	}
}

func (a anchoreClient) UpdatePolicy(ctx context.Context, policyID string, activate bool) error {
	fnCtx := map[string]interface{}{"policyID": policyID}
	a.logger.Info("updating policy", fnCtx)

	//temporary workaround for the policy update to work
	// at the time of writing the generated model lacks some field(s) that cause de update to fail

	var updatePolicyEndpoint = strings.Join([]string{a.endpoint, "policies", "{policyId}"}, "/")

	// get the policy to be updated
	r, err := a.authenticatedResty().SetPathParams(map[string]string{"policyId": policyID}).Get(updatePolicyEndpoint)
	if err != nil || r.StatusCode() != http.StatusOK {
		a.logger.Debug("failed to retrieve policy for update", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to retrieve policy for update", fnCtx)
	}

	// bind the response in order to be able to assemble the update request
	jsonContent := make([]map[string]interface{}, 0)
	err = json.Unmarshal(r.Body(), &jsonContent)
	if err != nil {
		a.logger.Debug("failed to unmarshal the policy response", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to unmarshal the policy response", fnCtx)
	}

	r, err = a.authenticatedResty().
		SetPathParams(map[string]string{"policyId": policyID}).
		SetQueryParam("active", strconv.FormatBool(activate)).
		SetBody(jsonContent[0]). // the first element is the retrieved policy!
		Put(updatePolicyEndpoint)

	if err != nil || r.StatusCode() != http.StatusOK {
		a.logger.Debug("failed to update policy", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to update for update", fnCtx)
	}

	a.logger.Info("policy successfully updated", fnCtx)
	return nil
}

func (a anchoreClient) GetPolicy(ctx context.Context, policyID string) (interface{}, error) {
	fnCtx := map[string]interface{}{"policyID": policyID}
	a.logger.Info("retrieving policy", fnCtx)

	policyBundles, r, err := a.getRestClient().PoliciesApi.GetPolicy(a.authorizedContext(ctx), policyID, &anchore.GetPolicyOpts{
		Detail: optional.NewBool(true),
	})
	if err != nil || r.StatusCode != http.StatusOK {
		a.logger.Debug("failed to retrieve policy", fnCtx)

		return nil, errors.WrapIfWithDetails(err, "failed to retrieve policy", fnCtx)
	}

	return &policyBundles, nil
}

func (a anchoreClient) ListPolicies(ctx context.Context) (interface{}, error) {
	a.logger.Info("retrieving policies ...")

	var listPoliciesEndpoint = strings.Join([]string{a.endpoint, "policies"}, "/")

	// authenticate the request
	r, err := a.authenticatedResty().Get(listPoliciesEndpoint)

	if err != nil {
		a.logger.Debug("failed to retrieve policies")

		return nil, errors.WrapIf(err, "failed to retrieve policies")
	}

	if r.StatusCode() != http.StatusOK {
		a.logger.Debug("failed to retrieve policies")

		return nil, errors.NewWithDetails("failed to retrieve policies", "httpStatusCode", r.StatusCode())
	}

	a.logger.Info("policies successfully retrieved")
	respJson := make([]map[string]interface{}, 0)
	if err = json.Unmarshal(r.Body(), &respJson); err != nil {
		a.logger.Debug("failed to unmarshal policy list")

		return nil, errors.WrapIfWithDetails(err, "failed to unmarshal policy list")
	}

	return respJson, nil
}

func (a anchoreClient) CreatePolicy(ctx context.Context, policy pipeline.PolicyBundleRecord) (interface{}, error) {
	fnCtx := map[string]interface{}{"policyID": policy}
	a.logger.Info("creating policy ...", fnCtx)

	var bundle anchore.PolicyBundle
	if err := a.transform(policy, &bundle); err != nil {
		a.logger.Debug("failed to transform policy", fnCtx)

		return nil, errors.WrapIfWithDetails(err, "failed to transform policy")
	}

	policyBundleRecord, r, err := a.getRestClient().PoliciesApi.AddPolicy(a.authorizedContext(ctx), bundle, &anchore.AddPolicyOpts{})
	if err != nil || r.StatusCode != http.StatusOK {
		a.logger.Debug("failed to create policy", fnCtx)

		return nil, errors.WrapIfWithDetails(err, "failed to create policy")
	}

	a.logger.Info("policy successfully created", fnCtx)

	return &policyBundleRecord, nil
}

func (a anchoreClient) DeletePolicy(ctx context.Context, policyID string) error {
	fnCtx := map[string]interface{}{"policyID": policyID}
	a.logger.Info("deleting policy ...", fnCtx)

	r, err := a.getRestClient().PoliciesApi.DeletePolicy(a.authorizedContext(ctx), policyID, &anchore.DeletePolicyOpts{})
	if err != nil || r.StatusCode != http.StatusOK {
		a.logger.Debug("failed to delete policy", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to delete policy")
	}

	a.logger.Info("policy successfully deleted")
	return nil
}

func (a anchoreClient) CreateAccount(ctx context.Context, accountName string, email string) error {
	fnCtx := map[string]interface{}{"accountName": accountName, "email": email}
	a.logger.Info("creating anchore account", fnCtx)

	_, resp, err := a.getRestClient().UserManagementApi.CreateAccount(a.authorizedContext(ctx),
		anchore.AccountCreationRequest{
			Name:  accountName,
			Email: email,
		})

	if err != nil || (resp.StatusCode != http.StatusOK) {
		a.logger.Debug("failed to create anchore account", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to create anchore account", fnCtx)
	}

	a.logger.Info("anchore account created", fnCtx)
	return nil
}

func (a anchoreClient) CreateUser(ctx context.Context, accountName string, userName string, password string) error {
	fnCtx := map[string]interface{}{"accountName": accountName, "userName": userName}
	a.logger.Info("creating anchore user", fnCtx)

	_, resp, err := a.getRestClient().UserManagementApi.CreateUser(a.authorizedContext(ctx),
		accountName, anchore.UserCreationRequest{
			Username: userName,
			Password: password,
		})

	if err != nil || (resp.StatusCode != http.StatusOK) {
		a.logger.Debug("failed to create anchore user", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to create anchore account", fnCtx)
	}

	a.logger.Info("anchore user created", fnCtx)
	return nil
}

func (a anchoreClient) GetUser(ctx context.Context, userName string) (interface{}, error) {
	fnCtx := map[string]interface{}{"userName": userName}
	a.logger.Info("retrieving anchore user", fnCtx)

	usr, resp, err := a.getRestClient().UserManagementApi.GetAccountUser(a.authorizedContext(ctx), userName, userName)
	if resp.StatusCode == http.StatusNotFound {
		// user not found
		return nil, nil
	}

	if err != nil {
		a.logger.Debug("failed to retrieve user from anchore", fnCtx)

		return nil, errors.WrapIfWithDetails(err, "failed to retrieve user from anchore", fnCtx)
	}

	return usr, nil
}

func (a anchoreClient) GetUserCredentials(ctx context.Context, userName string) (string, error) {
	fnCtx := map[string]interface{}{"userName": userName}
	a.logger.Info("retrieving anchore credentials", fnCtx)

	credentials, resp, err := a.getRestClient().UserManagementApi.ListUserCredentials(a.authorizedContext(ctx), userName, userName)
	if err != nil || (resp.StatusCode != http.StatusOK) {
		a.logger.Debug("failed to retrieve user credentials from anchore", fnCtx)

		return "", errors.WrapIfWithDetails(err, "failed to retrieve user credentials from anchore", fnCtx)
	}

	for _, credential := range credentials {
		if credential.Value != "" {
			return credential.Value, nil
		}
	}

	return "", errors.NewWithDetails("no credentials found", "userName", userName)
}

func (a anchoreClient) DeleteAccount(ctx context.Context, accountName string) error {
	fnCtx := map[string]interface{}{"accountName": accountName}
	a.logger.Info("deleting anchore account", fnCtx)

	// update the status of the account before delete
	s, ur, err := a.getRestClient().UserManagementApi.UpdateAccountState(a.authorizedContext(ctx), accountName, anchore.AccountStatus{State: "disabled"})

	if err != nil || ur.StatusCode != http.StatusOK || s.State != "disabled" {
		a.logger.Debug("failed to deactivate anchore account", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to deactivate anchore account", fnCtx)
	}

	// delete the account upon successful disable
	dr, err := a.getRestClient().UserManagementApi.DeleteAccount(a.authorizedContext(ctx), accountName)
	if err != nil || (dr.StatusCode != http.StatusOK && dr.StatusCode != http.StatusNoContent) {
		a.logger.Debug("failed to delete anchore account", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to delete anchore account", fnCtx)
	}

	a.logger.Info("deleted anchore account", fnCtx)
	return nil
}

func (a anchoreClient) GetAccount(ctx context.Context, accountName string) (string, error) {
	fnCtx := map[string]interface{}{"accountName": accountName}
	a.logger.Info("retrieving anchore account", fnCtx)

	acc, r, err := a.getRestClient().UserManagementApi.GetAccount(a.authorizedContext(ctx), accountName)
	if err != nil || r.StatusCode != http.StatusOK {
		a.logger.Debug("failed to get anchore account", fnCtx)

		return "", errors.WrapIfWithDetails(err, "failed to get anchore account", fnCtx)
	}

	a.logger.Info("retrieved anchore account", fnCtx)
	return acc.Name, nil
}

func (a anchoreClient) DeleteUser(ctx context.Context, accountName string, userName string) error {
	fnCtx := map[string]interface{}{"accountName": accountName, "userName": userName}
	a.logger.Info("deleting anchore user", fnCtx)

	r, err := a.getRestClient().UserManagementApi.DeleteUser(a.authorizedContext(ctx), accountName, userName)
	if err != nil || r.StatusCode != http.StatusNoContent {
		a.logger.Debug("failed to delete anchore user", fnCtx)

		return errors.WrapIfWithDetails(err, "failed to delete anchore user", fnCtx)
	}

	a.logger.Info("deleted anchore user", fnCtx)
	return nil
}

// ScanImage registers an image for security scanning
func (a anchoreClient) ScanImage(ctx context.Context, image pipeline.ClusterImage) (interface{}, error) {
	fnCtx := map[string]interface{}{"imageName": image.ImageName, "tag": image.ImageTag}

	body := struct {
		Tag    string `json:"tag,omitempty"`
		Digest string `json:"digest,omitempty"`
	}{
		Tag: strings.Join([]string{image.ImageName, image.ImageTag}, ":"),
	}

	var imagesEndpoint = strings.Join([]string{a.endpoint, "images"}, "/")

	resp, err := a.authenticatedResty().
		SetBody(body).
		Post(imagesEndpoint)

	if err != nil {
		a.logger.Debug("failed to add image to scan", fnCtx)

		return nil, errors.WrapIfWithDetails(err, "failed to add image to scan", image.ImageDigest)
	}

	if resp.StatusCode() != http.StatusOK {
		a.logger.Debug("image scan call failed", fnCtx)

		return nil, errors.NewWithDetails("image scan call failed", "httpStatus", resp.StatusCode())
	}

	a.logger.Debug("image added for security scan", map[string]interface{}{"image": image.ImageName})
	return resp.Body(), nil
}

func (a anchoreClient) GetImageVulnerabilities(ctx context.Context, imageDigest string) (interface{}, error) {
	a.logger.Debug("retrieving image vulnerabilities")

	vulnerabilities, resp, err := a.getRestClient().ImagesApi.GetImageVulnerabilitiesByType(a.authorizedContext(ctx),
		imageDigest, "all", &anchore.GetImageVulnerabilitiesByTypeOpts{})

	if err != nil || resp.StatusCode != http.StatusOK {
		a.logger.Debug("failed to retrieve image vulnerabilities")

		return nil, errors.WrapIf(err, "failed to retrieve vulnerabilities")
	}

	a.logger.Debug("successfully retrieved image vulnerabilities")
	return vulnerabilities, nil
}

func (a anchoreClient) CheckImage(ctx context.Context, imageDigest string) (interface{}, error) {
	a.logger.Debug("retrieving image metadata", map[string]interface{}{"imageDigest": imageDigest})

	imageMeta, resp, err := a.getRestClient().ImagesApi.GetImage(a.authorizedContext(ctx), imageDigest, &anchore.GetImageOpts{})

	if err != nil || resp.StatusCode != http.StatusOK {
		a.logger.Debug("failure while retrieving image metadata", map[string]interface{}{"imageDigest": imageDigest})

		return nil, errors.WrapIf(err, "failure while retrieving image metadata")
	}

	a.logger.Debug("successfully retrieved image metadata", map[string]interface{}{"imageDigest": imageDigest})
	return imageMeta, nil
}

func (a anchoreClient) authorizedContext(ctx context.Context) context.Context {

	basicAuth := anchore.BasicAuth{
		UserName: a.userName,
		Password: a.password,
	}

	return context.WithValue(ctx, anchore.ContextBasicAuth, basicAuth)
}

func (a anchoreClient) getRestClient() *anchore.APIClient {

	return anchore.NewAPIClient(&anchore.Configuration{
		BasePath:      a.endpoint,
		DefaultHeader: make(map[string]string),
		UserAgent:     "Pipeline/go",
	})
}

// transform quick and dirty solution for transformations between anchore types and pipeline types
// static casting doesn't work recursively, plain json transformation fails due to snake notation and camel case
// notation differences
// WARNING: Time values are lost during transformation, possible fix: https://github.com/mitchellh/mapstructure/issues/159
func (a anchoreClient) transform(fromType interface{}, toType interface{}) error {

	if err := mapstructure.Decode(fromType, toType); err != nil {
		return errors.WrapIf(err, "failed to unmarshal to 'toType' type")
	}

	return nil
}

// authenticatedResty sets up an authenticated resty client (this might be cached probably)
// WARNING: resty is temporarily used only as the generated client / openAPI spec seems not to be complete.
func (a anchoreClient) authenticatedResty() *resty.Request {
	return resty.R().SetBasicAuth(a.userName, a.password).SetHeader("User-Agent", "Pipeline/go")
}
