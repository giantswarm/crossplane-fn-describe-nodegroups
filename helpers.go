package main

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getKubeClient() (c client.Client, err error) {
	var config *rest.Config

	if config, err = rest.InClusterConfig(); err != nil {
		err = errors.Wrap(err, "cannot get cluster config")
		return
	}

	if c, err = client.New(config, client.Options{}); err != nil {
		err = errors.Wrap(err, "failed to create cluster client")
	}
	return
}

func (f *Function) getAssumeRoleArn() (arn string, err error) {
	var (
		unstructuredData *unstructured.Unstructured = &unstructured.Unstructured{}
		cl               client.Client
	)
	if cl, err = getKubeClient(); err != nil {
		err = errors.Wrap(err, "error setting up kubernetes client")
		return
	}
	// Get the provider context
	unstructuredData.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "aws.upbound.io",
		Kind:    "ProviderConfig",
		Version: "v1beta1",
	})

	if err = cl.Get(context.Background(), client.ObjectKey{
		Name: f.composed.ObservedComposite.Spec.AwsProviderConfigRef,
	}, unstructuredData); err != nil {
		err = errors.Wrap(err, "failed to load providerconfig")
		return
	}

	type _spec struct {
		AssumeRoleChain []struct {
			RoleARN string `json:"roleARN"`
		} `json:"assumeRoleChain"`
	}

	var spec _spec
	if err = toObject(unstructuredData.Object["spec"], &spec); err != nil {
		err = errors.Wrapf(err, "unable to decode provider config")
		return
	}

	f.log.Debug(composedName, "unstructured is", spec)

	// We only care about the first in the chain here.
	arn = spec.AssumeRoleChain[0].RoleARN
	return
}

func toObject(resource any, jsonObject any) (err error) {
	var b []byte
	if b, err = json.Marshal(resource); err != nil {
		return
	}

	err = json.Unmarshal(b, jsonObject)
	return
}
