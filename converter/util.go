package converter

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/jsonpb"
	authnpb "istio.io/api/authentication/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ObjectStruct struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              map[string]interface{} `json:"spec,omitempty"`
}

// ConvertToPolicy converts unstructured object to InputPolicy.
func ConvertToPolicy(item unstructured.Unstructured) (*InputPolicy, error) {
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
	return &InputPolicy{Name: name, Namespace: namespace, Policy: policy}, nil
}
