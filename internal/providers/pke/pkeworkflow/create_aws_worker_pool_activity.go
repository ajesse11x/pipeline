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

package pkeworkflow

import (
	"context"
	"fmt"
	"io/ioutil"
	"strconv"

	"emperror.dev/emperror"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/pkg/errors"
	"go.uber.org/cadence/activity"

	"github.com/banzaicloud/pipeline/internal/providers/amazon"
	pkgCloudformation "github.com/banzaicloud/pipeline/pkg/providers/amazon/cloudformation"
)

const CreateWorkerPoolActivityName = "pke-create-aws-worker-pool-activity"

type CreateWorkerPoolActivity struct {
	clusters       Clusters
	tokenGenerator TokenGenerator
}

func NewCreateWorkerPoolActivity(clusters Clusters, tokenGenerator TokenGenerator) *CreateWorkerPoolActivity {
	return &CreateWorkerPoolActivity{
		clusters:       clusters,
		tokenGenerator: tokenGenerator,
	}
}

type CreateWorkerPoolActivityInput struct {
	ClusterID                 uint
	Pool                      NodePool
	VPCID                     string
	VPCDefaultSecurityGroupID string
	SubnetID                  string
	WorkerInstanceProfile     string
	ClusterSecurityGroup      string
	ExternalBaseUrl           string
	ExternalBaseUrlInsecure   bool
	ImageID                   string
	SSHKeyName                string
}

func (a *CreateWorkerPoolActivity) Execute(ctx context.Context, input CreateWorkerPoolActivityInput) (string, error) {
	log := activity.GetLogger(ctx).Sugar().With("clusterID", input.ClusterID)
	cluster, err := a.clusters.GetCluster(ctx, input.ClusterID)
	if err != nil {
		return "", err
	}

	stackName := fmt.Sprintf("pke-pool-%s-worker-%s", cluster.GetName(), input.Pool.Name)

	awsCluster, ok := cluster.(AWSCluster)
	if !ok {
		return "", errors.New(fmt.Sprintf("can't get AWS client for %t", cluster))
	}

	ver, err := awsCluster.GetKubernetesVersion()
	if err != nil {
		return "", emperror.Wrap(err, "can't get Kubernetes version")
	}

	imageID, err := getDefaultImageID(cluster.GetLocation(), ver)
	if err != nil {
		return "", emperror.Wrapf(err, "failed to get default image for Kubernetes version %s", ver)
	}
	if input.Pool.ImageID != "" {
		imageID = input.Pool.ImageID
	}

	_, signedToken, err := a.tokenGenerator.GenerateClusterToken(cluster.GetOrganizationId(), cluster.GetID())
	if err != nil {
		return "", emperror.Wrap(err, "can't generate Pipeline token")
	}

	bootstrapCommand, err := awsCluster.GetBootstrapCommand(input.Pool.Name, input.ExternalBaseUrl, input.ExternalBaseUrlInsecure, signedToken)
	if err != nil {
		return "", emperror.Wrap(err, "failed to fetch bootstrap command")
	}

	client, err := awsCluster.GetAWSClient()
	if err != nil {
		return "", emperror.Wrap(err, "failed to connect to AWS")
	}

	cfClient := cloudformation.New(client)

	buf, err := ioutil.ReadFile("templates/pke/worker.cf.yaml")
	if err != nil {
		return "", emperror.Wrap(err, "loading CF template")
	}

	spotPrice, err := strconv.ParseFloat(input.Pool.SpotPrice, 64)
	if err != nil || spotPrice <= 0.0 {
		input.Pool.SpotPrice = ""
	}

	clusterName := cluster.GetName()

	autoscaling := aws.String("false")
	if input.Pool.Autoscaling {
		autoscaling = aws.String("true")
	}

	desired := input.Pool.Count
	if desired < input.Pool.MinCount {
		desired = input.Pool.MinCount
	}
	if desired > input.Pool.MaxCount {
		desired = input.Pool.MaxCount
	}

	subnetID := input.SubnetID
	if len(input.Pool.Subnets) > 0 {
		subnetID = input.Pool.Subnets[0]
	}

	stackInput := &cloudformation.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(string(buf)),
		// ClientRequestToken: aws.String(string(activity.GetInfo(ctx).ActivityID)),
		Parameters: []*cloudformation.Parameter{
			{
				ParameterKey:   aws.String("ClusterName"),
				ParameterValue: &clusterName,
			},
			{
				ParameterKey:   aws.String("NodeGroupName"),
				ParameterValue: &input.Pool.Name,
			},
			{
				ParameterKey:   aws.String("PkeCommand"),
				ParameterValue: &bootstrapCommand,
			},
			{
				ParameterKey:   aws.String("InstanceType"),
				ParameterValue: aws.String(input.Pool.InstanceType),
			},
			{
				ParameterKey:   aws.String("VPCId"),
				ParameterValue: &input.VPCID,
			},
			{
				ParameterKey:   aws.String("VPCDefaultSecurityGroupId"),
				ParameterValue: &input.VPCDefaultSecurityGroupID,
			},
			{
				ParameterKey:   aws.String("SubnetIds"),
				ParameterValue: &subnetID,
			},
			{
				ParameterKey:   aws.String("IamInstanceProfile"),
				ParameterValue: &input.WorkerInstanceProfile,
			},
			{
				ParameterKey:   aws.String("ImageId"),
				ParameterValue: aws.String(imageID),
			},
			{
				ParameterKey:   aws.String("PkeVersion"),
				ParameterValue: aws.String(pkeVersion),
			},
			{
				ParameterKey:   aws.String("KeyName"),
				ParameterValue: aws.String(input.SSHKeyName),
			},
			{
				ParameterKey:   aws.String("MinSize"),
				ParameterValue: aws.String(strconv.Itoa(input.Pool.MinCount)),
			},
			{
				ParameterKey:   aws.String("MaxSize"),
				ParameterValue: aws.String(strconv.Itoa(input.Pool.MaxCount)),
			},
			{
				ParameterKey:   aws.String("DesiredCapacity"),
				ParameterValue: aws.String(strconv.Itoa(desired)),
			},
			{
				ParameterKey:   aws.String("ClusterSecurityGroup"),
				ParameterValue: aws.String(input.ClusterSecurityGroup),
			},
			{
				ParameterKey:   aws.String("NodeSpotPrice"),
				ParameterValue: aws.String(input.Pool.SpotPrice),
			},
			{
				ParameterKey:   aws.String("ClusterAutoscalerEnabled"),
				ParameterValue: autoscaling,
			},
		},
		Tags: amazon.PipelineTags(),
	}

	output, err := cfClient.CreateStack(stackInput)
	if err, ok := err.(awserr.Error); ok {
		switch err.Code() {
		case cloudformation.ErrCodeAlreadyExistsException:
			log.Infof("stack already exists: %s", err.Message())
		default:
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	err = cfClient.WaitUntilStackCreateCompleteWithContext(ctx, &cloudformation.DescribeStacksInput{StackName: aws.String(stackName)})
	if err != nil {
		return "", emperror.Wrap(pkgCloudformation.NewAwsStackFailure(err, stackName, cfClient), "waiting for stack creation")
	}

	if output.StackId != nil {
		return *output.StackId, nil
	}
	return stackName, nil
}
