package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	authnpb "istio.io/api/authentication/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/yaml"
)

type objectStruct struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              map[string]interface{} `json:"spec,omitempty"`
}

func convertToPolicy(item unstructured.Unstructured) (*InputPolicy, error) {
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

func (output *OutputPolicy) toYAML() string {
	obj := &objectStruct{}
	obj.SetName(output.Name)
	obj.SetNamespace(output.Namespace)
	if output.Comment != "" {
		obj.SetAnnotations(map[string]string{"security.istio.io/alpha-policy-convert": output.Comment})
	}

	var data strings.Builder
	if output.PeerAuthN != nil {
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "security.istio.io", Version: "v1beta1", Kind: "PeerAuthentication"})
		data.WriteString(specToYAML(obj, output.PeerAuthN))
		data.WriteString("\n---\n")
	}
	if output.RequestAuthN != nil {
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "security.istio.io", Version: "v1beta1", Kind: "RequestAuthentication"})
		data.WriteString(specToYAML(obj, output.RequestAuthN))
		data.WriteString("\n---\n")
	}
	if output.Authz != nil {
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "security.istio.io", Version: "v1beta1", Kind: "AuthorizationPolicy"})
		data.WriteString(specToYAML(obj, output.Authz))
		data.WriteString("\n---\n")
	}
	return data.String()
}

func specToYAML(obj *objectStruct, spec proto.Message) string {
	m := jsonpb.Marshaler{}
	jsonStr, err := m.MarshalToString(spec)
	if err != nil {
		log.Fatalf("failed to marshal to string: %v", err)
	}
	obj.Spec = map[string]interface{}{}
	if err := json.Unmarshal([]byte(jsonStr), &obj.Spec); err != nil {
		log.Fatalf("failed to unmarshal to object: %v", err)
	}
	jsonOut, err := json.Marshal(obj)
	if err != nil {
		log.Fatalf("failed to marshal policy: %v", err)
	}
	yamlOut, err := yaml.JSONToYAML(jsonOut)
	if err != nil {
		log.Fatalf("failed to convert JSON to YAML: %v", err)
	}

	return string(yamlOut)
}
