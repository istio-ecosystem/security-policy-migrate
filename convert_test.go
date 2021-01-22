package main

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	authnpb "istio.io/api/authentication/v1alpha1"
	betapb "istio.io/api/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func parseInputs(yaml string) ([]*objectStruct, error) {
	yamlDecoder := kubeyaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(yaml)), 512*1024)
	var ret []*objectStruct
	for {
		obj := objectStruct{}
		err := yamlDecoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if reflect.DeepEqual(obj, objectStruct{}) {
			continue
		}
		ret = append(ret, &obj)
	}
	return ret, nil
}

func specToProto(t *testing.T, spec map[string]interface{}, msg proto.Message) {
	t.Helper()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec %v: %v", spec, err)
	}
	if err := jsonpb.UnmarshalString(string(data), msg); err != nil {
		t.Fatalf("failed to convert to protobuf %v: %v", string(data), err)
	}
}

func inputPolicy(t *testing.T, yaml string) *InputPolicy {
	t.Helper()
	objects, err := parseInputs(yaml)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", yaml, err)
	}
	if len(objects) != 1 {
		t.Fatalf("expect 1 config but got %d config", len(objects))
	}
	object := objects[0]
	if object.APIVersion != "authentication.istio.io/v1alpha1" {
		t.Fatalf("unsupported input policy api version: %s", object.APIVersion)
	}

	policy := &authnpb.Policy{}
	specToProto(t, object.Spec, policy)
	return &InputPolicy{
		Name:      object.Name,
		Namespace: object.Namespace,
		Policy:    policy,
	}
}

func outputPolicy(t *testing.T, yaml string) []*OutputPolicy {
	t.Helper()
	objects, err := parseInputs(yaml)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", yaml, err)
	}
	var output []*OutputPolicy
	for _, object := range objects {
		if object.APIVersion != "security.istio.io/v1beta1" {
			t.Fatalf("unsupported output policy api version: %s", object.APIVersion)
		}
		switch object.Kind {
		case "AuthorizationPolicy":
			policy := &betapb.AuthorizationPolicy{}
			specToProto(t, object.Spec, policy)
			output = append(output, &OutputPolicy{
				Name:      object.Name,
				Namespace: object.Namespace,
				Authz:     policy,
			})
		case "RequestAuthentication":
			policy := &betapb.RequestAuthentication{}
			specToProto(t, object.Spec, policy)
			output = append(output, &OutputPolicy{
				Name:         object.Name,
				Namespace:    object.Namespace,
				RequestAuthN: policy,
			})
		case "PeerAuthentication":
			policy := &betapb.PeerAuthentication{}
			specToProto(t, object.Spec, policy)
			output = append(output, &OutputPolicy{
				Name:      object.Name,
				Namespace: object.Namespace,
				PeerAuthN: policy,
			})
		default:
			t.Fatalf("invalid output policy api version: %s", object.APIVersion)
		}
	}
	return output
}

func compareOutputPolicy(t *testing.T, got []*OutputPolicy, want []*OutputPolicy) {
	toMap := func(data []*OutputPolicy) map[string]*OutputPolicy {
		result := map[string]*OutputPolicy{}
		for _, policy := range data {
			name := policy.Namespace + "." + policy.Name
			if old, found := result[name]; found {
				if old.PeerAuthN != nil && policy.PeerAuthN != nil {
					t.Errorf("got duplicate PeerAuthN %s of config %s, previous config %s", name, policy.PeerAuthN, old.PeerAuthN)
				} else if policy.PeerAuthN != nil {
					old.PeerAuthN = policy.PeerAuthN
				}
				if old.RequestAuthN != nil && policy.RequestAuthN != nil {
					t.Errorf("got duplicate RequestAuthN %s of config %s, previous config %s", name, policy.RequestAuthN, old.RequestAuthN)
				} else if policy.RequestAuthN != nil {
					old.RequestAuthN = policy.RequestAuthN
				}
				if old.Authz != nil && policy.Authz != nil {
					t.Errorf("got duplicate Authz %s of config %s, previous config %s", name, policy.Authz, old.Authz)
				} else if policy.Authz != nil {
					old.Authz = policy.Authz
				}
			} else {
				result[name] = policy
			}
		}
		return result
	}
	gotMap := toMap(got)
	wantMap := toMap(want)
	for k, v := range gotMap {
		if want, found := wantMap[k]; found {
			if diff := cmp.Diff(want.PeerAuthN, v.PeerAuthN, protocmp.Transform()); diff != "" {
				t.Errorf("PeerAuthN diff (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(want.RequestAuthN, v.RequestAuthN, protocmp.Transform()); diff != "" {
				t.Errorf("RequestAuthN diff (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(want.Authz, v.Authz, protocmp.Transform()); diff != "" {
				t.Errorf("Authz diff (-want +got):\n%s", diff)
			}
		} else {
			t.Errorf("got unexpected policy %s of config %v", k, v)
		}
	}
	for k, v := range wantMap {
		if _, found := gotMap[k]; !found {
			t.Errorf("not found policy %s of config %v", k, v)
		}
	}
}

func TestConverter_Convert_Fail(t *testing.T) {
	cases := []struct {
		wantError   string
		svcList     *corev1.ServiceList
		inputPolicy *InputPolicy
	}{
		{
			wantError: "found duplicate target my-service",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service
  - name: my-service
`),
		},
		{
			wantError: "failed to convert target (my-service) to workload selector: could not find port number:8000",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
							Ports: []corev1.ServicePort{
								{
									Name: "http",
									Port: 7000,
								},
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service
    ports:
    - number: 8000
`),
		},
		{
			wantError: "failed to convert target (my-service) to workload selector: could not find port name:\"tcp\"",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
							Ports: []corev1.ServicePort{
								{
									Name: "http",
									Port: 7000,
								},
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service
    ports:
    - name: tcp
`),
		},
		{
			wantError: "failed to convert target (my-service-bla) to workload selector: could not find service bar.my-service-bla",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service-bla
`),
		},
		{
			wantError: "triggerRule is not supported with multiple JWT issuer",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
      triggerRules:
      - includedPaths:
        - exact: /includeExact
  principalBinding: USE_ORIGIN
`),
		},
		{
			wantError: "triggerRule.regex (\"some-regex\") is not supported",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
      triggerRules:
      - includedPaths:
        - regex: some-regex
  principalBinding: USE_ORIGIN
`),
		},
		{
			wantError: "JWT is never supported in peer method",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  peers:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.wantError, func(t *testing.T) {
			mc := newConverter("istio-system", tc.svcList)
			output, result := mc.Convert(tc.inputPolicy)
			if len(result.errors) == 0 {
				t.Errorf("want error %q but got no error: %v", tc.wantError, output)
			}
			for _, gotErr := range result.errors {
				if !strings.HasPrefix(gotErr, tc.wantError) {
					t.Errorf("want error %q but got %q", tc.wantError, gotErr)
				}
			}
		})
	}
}

func TestConverter_Convert_Success(t *testing.T) {
	cases := []struct {
		name        string
		svcList     *corev1.ServiceList
		inputPolicy *InputPolicy
		wantOutput  []*OutputPolicy
		wantResult  *ResultSummary
	}{
		{
			name: "mesh-level-strict",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: MeshPolicy
metadata:
  name: default
  namespace: istio-system
spec:
  peers:
  - mtls:
      mode: STRICT
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: istio-system
spec:
  mtls:
    mode: STRICT
`),
		},

		{
			name: "mesh-level-permissive",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: MeshPolicy
metadata:
  name: default
  namespace: istio-system
spec:
  peers:
  - mtls:
      mode: PERMISSIVE
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: istio-system
spec:
  mtls:
    mode: PERMISSIVE
`),
		},

		{
			name: "namespace-level",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: default
  namespace: foo
spec:
  peers:
  - mtls:
      mode: STRICT
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: foo
spec:
  mtls:
    mode: STRICT
`),
		},

		{
			name: "service-level-single-target",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service
  peers:
  - mtls:
      mode: STRICT
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: httpbin-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  mtls:
    mode: STRICT
`),
		},

		{
			name: "service-level-multiple-targets",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service-1",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service-1",
								"ver": "v1",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service-1",
							Namespace: "foo",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service-1",
								"ver": "v2",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service-2",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service-2",
								"ver": "v1",
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service-1
  - name: my-service-2
  peers:
  - mtls:
      mode: PERMISSIVE
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: httpbin-my-service-1
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service-1
      ver: v1
  mtls:
    mode: PERMISSIVE
---
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: httpbin-my-service-2
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service-2
      ver: v1
  mtls:
    mode: PERMISSIVE
`),
		},

		{
			name: "port-level",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
							Ports: []corev1.ServicePort{
								{
									Name:       "http",
									Port:       8000,
									TargetPort: intstr.IntOrString{IntVal: 80},
								},
								{
									Name:       "http2",
									Port:       8001,
									TargetPort: intstr.IntOrString{IntVal: 81},
								},
								{
									Name: "tcp",
									Port: 8002,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service-2",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service-2",
								"ver": "v1",
							},
							Ports: []corev1.ServicePort{
								{
									Name:       "grpc",
									Port:       9000,
									TargetPort: intstr.IntOrString{IntVal: 90},
								},
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: httpbin
  namespace: bar
spec:
  targets:
  - name: my-service
    ports:
    - number: 8000
    - number: 8001
    - number: 8002
  - name: my-service-2
    ports:
    - name: grpc
  peers:
  - mtls:
      mode: STRICT
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: httpbin-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  portLevelMtls:
    80:
      mode: STRICT
    81:
      mode: STRICT
    8002:
      mode: STRICT
---
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: httpbin-my-service-2
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service-2
      ver: v1
  portLevelMtls:
    90:
      mode: STRICT
`),
		},

		{
			name: "jwt",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: default
  namespace: bar
spec:
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
  - jwt:
      issuer: "testing2@secure.istio.io"
      jwksUri: "https://secure2.istio.io"
  principalBinding: USE_ORIGIN
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: bar
spec:
  mtls:
    mode: PERMISSIVE
---
apiVersion: security.istio.io/v1beta1
kind: RequestAuthentication
metadata:
  name: default
  namespace: bar
spec:
  jwtRules:
  - issuer: "testing@secure.istio.io"
    jwksUri: "https://secure.istio.io"
  - issuer: "testing2@secure.istio.io"
    jwksUri: "https://secure2.istio.io"
---
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: default
  namespace: bar
spec:
  action: DENY
  rules:
  - from:
    - source:
        notRequestPrincipals: ["*"]
`),
		},

		{
			name: "jwt-port-level",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
							Ports: []corev1.ServicePort{
								{
									Name:       "http",
									Port:       8000,
									TargetPort: intstr.IntOrString{IntVal: 80},
								},
								{
									Name:       "http2",
									Port:       8001,
									TargetPort: intstr.IntOrString{IntVal: 81},
								},
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: jwt
  namespace: bar
spec:
  targets:
  - name: my-service
    ports:
    - number: 8000
    - number: 8001
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
  - jwt:
      issuer: "testing2@secure.istio.io"
      jwksUri: "https://secure2.istio.io"
  principalBinding: USE_ORIGIN
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  portLevelMtls:
    80:
      mode: PERMISSIVE
    81:
      mode: PERMISSIVE
---
apiVersion: security.istio.io/v1beta1
kind: RequestAuthentication
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  jwtRules:
  - issuer: "testing@secure.istio.io"
    jwksUri: "https://secure.istio.io"
  - issuer: "testing2@secure.istio.io"
    jwksUri: "https://secure2.istio.io"
---
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  action: DENY
  rules:
  - from:
    - source:
        notRequestPrincipals: ["*"]
    to:
    - operation:
        ports: ["80", "81"]
`),
		},

		{
			name: "jwt-trigger-rule",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
							Ports: []corev1.ServicePort{
								{
									Name:       "http",
									Port:       8000,
									TargetPort: intstr.IntOrString{IntVal: 80},
								},
								{
									Name:       "http2",
									Port:       8001,
									TargetPort: intstr.IntOrString{IntVal: 81},
								},
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: jwt
  namespace: bar
spec:
  targets:
  - name: my-service
    ports:
    - number: 8000
    - number: 8001
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
      triggerRules:
      - includedPaths:
        - exact: /includeExact1
        - prefix: /includePrefix1
        - suffix: includeSuffix1
        excludedPaths:
        - exact: /excludeExact1
        - prefix: /excludePrefix1
        - suffix: excludeSuffix1
      - includedPaths:
        - exact: /includeExact2
        - prefix: /includePrefix2
        - suffix: includeSuffix2
        excludedPaths:
        - exact: /excludeExact2
        - prefix: /excludePrefix2
        - suffix: excludeSuffix2
  principalBinding: USE_ORIGIN
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  portLevelMtls:
    80:
      mode: PERMISSIVE
    81:
      mode: PERMISSIVE
---
apiVersion: security.istio.io/v1beta1
kind: RequestAuthentication
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  jwtRules:
  - issuer: "testing@secure.istio.io"
    jwksUri: "https://secure.istio.io"
---
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  action: DENY
  rules:
  - from:
    - source:
        notRequestPrincipals: ["*"]
    to:
    - operation:
        ports: ["80", "81"]
        paths: ["/includeExact1", "/includePrefix1*", "*includeSuffix1"]
        notPaths: ["/excludeExact1", "/excludePrefix1*", "*excludeSuffix1"]
  - from:
    - source:
        notRequestPrincipals: ["*"]
    to:
    - operation:
        ports: ["80", "81"]
        paths: ["/includeExact2", "/includePrefix2*", "*includeSuffix2"]
        notPaths: ["/excludeExact2", "/excludePrefix2*", "*excludeSuffix2"]
`),
		},

		{
			name: "jwt-trigger-rule-only-path",
			svcList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-service",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{
								"app": "my-service",
								"ver": "v1",
							},
						},
					},
				},
			},
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: jwt
  namespace: bar
spec:
  targets:
  - name: my-service
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
      triggerRules:
      - includedPaths:
        - exact: /includeExact1
        - prefix: /includePrefix1
        - suffix: includeSuffix1
        excludedPaths:
        - exact: /excludeExact1
        - prefix: /excludePrefix1
        - suffix: excludeSuffix1
  principalBinding: USE_ORIGIN
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  mtls:
    mode: PERMISSIVE
---
apiVersion: security.istio.io/v1beta1
kind: RequestAuthentication
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  jwtRules:
  - issuer: "testing@secure.istio.io"
    jwksUri: "https://secure.istio.io"
---
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: jwt-my-service
  namespace: bar
spec:
  selector:
    matchLabels:
      app: my-service
      ver: v1
  action: DENY
  rules:
  - from:
    - source:
        notRequestPrincipals: ["*"]
    to:
    - operation:
        paths: ["/includeExact1", "/includePrefix1*", "*includeSuffix1"]
        notPaths: ["/excludeExact1", "/excludePrefix1*", "*excludeSuffix1"]
`),
		},

		{
			name: "jwt-optional",
			inputPolicy: inputPolicy(t, `
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: default
  namespace: bar
spec:
  origins:
  - jwt:
      issuer: "testing@secure.istio.io"
      jwksUri: "https://secure.istio.io"
  - jwt:
      issuer: "testing2@secure.istio.io"
      jwksUri: "https://secure2.istio.io"
  principalBinding: USE_ORIGIN
  originIsOptional: true
`),
			wantOutput: outputPolicy(t, `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: bar
spec:
  mtls:
    mode: PERMISSIVE
---
apiVersion: security.istio.io/v1beta1
kind: RequestAuthentication
metadata:
  name: default
  namespace: bar
spec:
  jwtRules:
  - issuer: "testing@secure.istio.io"
    jwksUri: "https://secure.istio.io"
  - issuer: "testing2@secure.istio.io"
    jwksUri: "https://secure2.istio.io"
`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mc := newConverter("istio-system", tc.svcList)
			output, result := mc.Convert(tc.inputPolicy)
			compareOutputPolicy(t, output, tc.wantOutput)
			if t.Failed() {
				t.Logf("got result: %v", result)
			}
		})
	}
}
