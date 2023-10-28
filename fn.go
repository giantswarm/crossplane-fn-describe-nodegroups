package main

import (
	"context"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/response"
)

const composedName = "function-describe-nodegroups"

// RunFunction Execute the desired reconcilliation state, creating any required resources
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (rsp *fnv1beta1.RunFunctionResponse, err error) {
	f.log.Info("preparing function", composedName, req.GetMeta().GetTag())
	rsp = response.To(req, response.DefaultTTL)

	if f.composed, err = RequestToComposition(req); err != nil {
		if errors.Is(err, &WaitingForSpec{}) {
			response.Normal(rsp, "waiting for resource")
			return rsp, nil
		}

		response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
		return rsp, nil
	}

	var (
		clusterName string = f.composed.ObservedComposite.Spec.ClusterName
		namespace   string = f.composed.ObservedComposite.Spec.ClaimRef.Namespace
		region      string = f.composed.ObservedComposite.Spec.Region
		provider    string = f.composed.ObservedComposite.Spec.CompositionSelector.MatchLabels.Provider

		labels      map[string]string = f.composed.ObservedComposite.Metadata.Labels
		annotations map[string]string = map[string]string{
			"cluster.x-k8s.io/managed-by": "crossplane",
		}
	)

	// Merge in the additional labels for kubernetes resources
	for k, v := range f.composed.ObservedComposite.Spec.KubernetesAdditionalLabels {
		labels[k] = v
	}

	switch strings.ToLower(provider) {
	case "aws":
		f.log.Info("discovered aws provider", composedName, req.GetMeta().GetTag())
		var arn string
		if arn, err = f.getAssumeRoleArn(); err != nil {
			response.Fatal(rsp, errors.Wrap(err, "error retrieving provider assume role arn "+composedName))
			return rsp, nil
		}

		if err = f.CreateAWSNodegroupSpec(clusterName, namespace, region, arn, labels, annotations); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composite resources from %T", req))
			return rsp, nil
		}
	case "azure":
		break
	case "gcp":
		break
	}

	if err = f.composed.ToResponse(rsp); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot convert composition to response %T", rsp))
		return
	}

	response.Normal(rsp, "Successful run")
	return rsp, nil
}
