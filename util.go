package main

import (
	"fmt"

	"github.com/gogo/protobuf/jsonpb"
	authnpb "istio.io/api/authentication/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"

	convertpkg "github.com/istio-ecosystem/security-policy-migrate/converter"
)

func convertToPolicy(item unstructured.Unstructured) (*convertpkg.InputPolicy, error) {
	spec, ok := item.UnstructuredContent()["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to extract spec from item")
	}
	specString, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec %v to string: %w", spec, err)
	}
	policy := &authnpb.Policy{}
	if err := jsonpb.UnmarshalString(string(specString), policy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal string %s to proto: %w", specString, err)
	}
	name, ok, err := unstructured.NestedString(item.Object, "metadata", "name")
	if !ok || err != nil {
		return nil, fmt.Errorf("failed to extract name: %w", err)
	}
	namespace, _, err := unstructured.NestedString(item.Object, "metadata", "namespace")
	if err != nil {
		return nil, fmt.Errorf("failed to extract namespace: %w", err)
	}
	return &convertpkg.InputPolicy{Name: name, Namespace: namespace, Policy: policy}, nil
}
