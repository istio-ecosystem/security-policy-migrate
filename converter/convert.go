package converter

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	authnpb "istio.io/api/authentication/v1alpha1"
	betapb "istio.io/api/security/v1beta1"
	commonpb "istio.io/api/type/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

type mTLSMode int

// Unset, Strict and Permissive for the mTLS mode.
const (
	Unset      mTLSMode = 0
	Strict     mTLSMode = 1
	Permissive mTLSMode = 2
)

// ResultSummary includes the conversion summary.
type ResultSummary struct {
	Errors []string
}

func (r *ResultSummary) addError(err string) {
	r.Errors = append(r.Errors, err)
}

// Converter includes general mesh wide settings.
type Converter struct {
	RootNamespace string
	Service       *ServiceStore
}

// ServiceStore represents all services in the cluster.
type ServiceStore struct {
	Services map[string]*corev1.Service
}

// NewConverter constructs a Converter.
func NewConverter(rootNamespace string, svcList *corev1.ServiceList) *Converter {
	services := map[string]*corev1.Service{}
	if svcList != nil {
		for _, svc := range svcList.Items {
			svc := svc
			services[svc.Namespace+"."+svc.Name] = &svc
		}
	}
	return &Converter{
		RootNamespace: rootNamespace,
		Service:       &ServiceStore{Services: services},
	}
}

func (ss *ServiceStore) serviceToSelector(name, namespace string) (*commonpb.WorkloadSelector, error) {
	service := namespace + "." + name
	if svc, found := ss.Services[service]; found {
		return &commonpb.WorkloadSelector{MatchLabels: svc.Spec.Selector}, nil
	}
	return nil, fmt.Errorf("could not find service %s", service)
}

func (ss *ServiceStore) svcPortToWorkloadPort(name, namespace string, svcPort *authnpb.PortSelector) (uint32, error) {
	service := namespace + "." + name
	if svc, found := ss.Services[service]; found {
		for _, port := range svc.Spec.Ports {
			if (svcPort.GetName() != "" && port.Name == svcPort.GetName()) || (svcPort.GetNumber() != 0 && uint32(port.Port) == svcPort.GetNumber()) {
				if port.TargetPort.IntVal == 0 {
					return uint32(port.Port), nil
				}
				return uint32(port.TargetPort.IntVal), nil
			}
		}
	}
	return 0, fmt.Errorf("could not find port %v for service %s", svcPort, service)
}

// InputPolicy includes a v1alpha1 authentication policy.
type InputPolicy struct {
	Name      string
	Namespace string
	Policy    *authnpb.Policy
}

type outputSelector struct {
	Comment   string
	Name      string
	Namespace string
	Selector  *commonpb.WorkloadSelector
	Port      []uint32
}

// OutputPolicy includes the v1beta1 policy converted from v1alpha authentication policy.
type OutputPolicy struct {
	Name         string
	Namespace    string
	Comment      string // Could be added to the annotation, e.g. security.istio.io/autoConversionResult: "..."
	PeerAuthN    *betapb.PeerAuthentication
	RequestAuthN *betapb.RequestAuthentication
	Authz        *betapb.AuthorizationPolicy
}

// ToYAML converts output to yaml.
func (output *OutputPolicy) ToYAML() string {
	obj := &ObjectStruct{}
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

func specToYAML(obj *ObjectStruct, spec proto.Message) string {
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

// Convert converts an v1alpha1 authentication policy to the v1beta1 policies.
func (mc *Converter) Convert(input *InputPolicy) ([]*OutputPolicy, *ResultSummary) {
	result := &ResultSummary{}

	// Convert the service target to a list of workload selectors. Each workload selector is used in a new beta policy.
	var outputSelectors []*outputSelector
	foundTarget := map[string]struct{}{}
	if len(input.Policy.Targets) != 0 {
		for _, target := range input.Policy.Targets {
			if _, found := foundTarget[target.Name]; found {
				result.addError(fmt.Sprintf("found duplicate target %s", target.Name))
			} else {
				foundTarget[target.Name] = struct{}{}
			}
			if selector, err := mc.targetToSelector(input, target); err != nil {
				result.addError(fmt.Sprintf("failed to convert target (%s) to workload selector: %v", target.Name, err))
			} else {
				outputSelectors = append(outputSelectors, selector)
			}
		}
	} else {
		if input.Namespace == "" {
			outputSelectors = []*outputSelector{
				{
					Comment:   "mesh level policy",
					Name:      input.Name,
					Namespace: mc.RootNamespace,
				},
			}
		} else {
			outputSelectors = []*outputSelector{
				{
					Comment:   "namespace level policy",
					Name:      input.Name,
					Namespace: input.Namespace,
				},
			}
		}
	}

	outputPolicies := convertMTLS(outputSelectors, input, result)
	outputPolicies = append(outputPolicies, convertJWT(outputSelectors, input, result)...)

	return outputPolicies, result
}

func (mc *Converter) targetToSelector(input *InputPolicy, target *authnpb.TargetSelector) (*outputSelector, error) {
	selector, err := mc.Service.serviceToSelector(target.Name, input.Namespace)
	if err != nil {
		return nil, err
	}

	output := &outputSelector{
		Comment:   fmt.Sprintf("service %s", target.Name),
		Name:      fmt.Sprintf("%s-%s", input.Name, target.Name),
		Namespace: input.Namespace,
		Selector:  selector,
	}

	for _, port := range target.Ports {
		workloadPort, err := mc.Service.svcPortToWorkloadPort(target.Name, input.Namespace, port)
		if err != nil {
			return nil, err
		}
		output.Port = append(output.Port, workloadPort)
	}

	return output, nil
}

func convertMTLS(selectors []*outputSelector, input *InputPolicy, result *ResultSummary) []*OutputPolicy {
	mode := betapb.PeerAuthentication_MutualTLS_PERMISSIVE
	switch extractMTLS(input, result) {
	case Unset, Permissive:
		// Do not use DISABLE mode, it seems not working with autoMTLS and requires an extra DestinationRule.
		mode = betapb.PeerAuthentication_MutualTLS_PERMISSIVE
	case Strict:
		mode = betapb.PeerAuthentication_MutualTLS_STRICT
	}
	var output []*OutputPolicy
	for _, selector := range selectors {
		peerAuthn := &betapb.PeerAuthentication{
			Selector: selector.Selector,
		}

		if len(selector.Port) != 0 {
			peerAuthn.PortLevelMtls = map[uint32]*betapb.PeerAuthentication_MutualTLS{}
			for _, port := range selector.Port {
				peerAuthn.PortLevelMtls[port] = &betapb.PeerAuthentication_MutualTLS{Mode: mode}
			}
		} else {
			peerAuthn.Mtls = &betapb.PeerAuthentication_MutualTLS{
				Mode: mode,
			}
		}

		output = append(output, &OutputPolicy{
			Name:      selector.Name,
			Namespace: selector.Namespace,
			Comment:   fmt.Sprintf("converted from alpha authentication policy %s/%s, %s", input.Namespace, input.Name, selector.Comment),
			PeerAuthN: peerAuthn,
		})
	}
	return output
}

func convertJWT(selectors []*outputSelector, input *InputPolicy, result *ResultSummary) []*OutputPolicy {
	if len(input.Policy.Origins) == 0 {
		return nil
	}

	var output []*OutputPolicy
	for _, selector := range selectors {
		// Create a single request authentication policy for all JWT issuers.
		requestAuthn := &betapb.RequestAuthentication{
			Selector: selector.Selector,
		}
		for _, origin := range input.Policy.Origins {
			if origin.Jwt == nil {
				continue
			}
			jwt := origin.Jwt
			jwtRule := &betapb.JWTRule{
				Issuer:               jwt.Issuer,
				Audiences:            jwt.Audiences,
				JwksUri:              jwt.JwksUri,
				Jwks:                 jwt.Jwks,
				FromParams:           jwt.JwtParams,
				ForwardOriginalToken: true,
			}
			for _, header := range jwt.JwtHeaders {
				jwtRule.FromHeaders = append(jwtRule.FromHeaders, &betapb.JWTHeader{Name: header})
			}
			requestAuthn.JwtRules = append(requestAuthn.JwtRules, jwtRule)

			// Check some unsupported cases.
			if len(jwt.TriggerRules) > 0 {
				if len(input.Policy.Origins) > 1 {
					result.addError("triggerRule is not supported with multiple JWT issuer by the tool, please convert manually")
				}
				for _, rule := range jwt.TriggerRules {
					for _, path := range rule.IncludedPaths {
						if path.GetRegex() != "" {
							result.addError(fmt.Sprintf("triggerRule.regex (%q) is not supported in beta policy", path.GetRegex()))
						}
					}
					for _, path := range rule.ExcludedPaths {
						if path.GetRegex() != "" {
							result.addError(fmt.Sprintf("triggerRule.regex (%q) is not supported in beta policy", path.GetRegex()))
						}
					}
				}
			}
		}

		// Create an authorization policy if the JWT authentication is required (not optional).
		var authzPolicy *betapb.AuthorizationPolicy
		if !input.Policy.OriginIsOptional {
			authzPolicy = &betapb.AuthorizationPolicy{
				Selector: selector.Selector,
				Action:   betapb.AuthorizationPolicy_DENY, // Use DENY action with notRequestPrincipal to require JWT authentication.
			}
			newRule := func(paths, notPaths []string) *betapb.Rule {
				ret := &betapb.Rule{
					From: []*betapb.Rule_From{
						{
							Source: &betapb.Source{
								NotRequestPrincipals: []string{"*"},
							},
						},
					},
				}
				if len(paths) == 0 && len(notPaths) == 0 && len(selector.Port) == 0 {
					// Return to avoid generating empty operation.
					return ret
				}

				ret.To = []*betapb.Rule_To{{Operation: &betapb.Operation{
					Paths:    paths,
					NotPaths: notPaths,
					Ports:    toStr(selector.Port),
				}}}
				return ret
			}

			if len(input.Policy.Origins) == 1 && len(input.Policy.Origins[0].GetJwt().GetTriggerRules()) > 0 {
				// Support the trigger rule if there is only 1 JWT issuer.
				for _, trigger := range input.Policy.Origins[0].GetJwt().GetTriggerRules() {
					extractPaths := func(pathMatchers []*authnpb.StringMatch) []string {
						if len(pathMatchers) == 0 {
							return nil
						}
						var ret []string
						for _, path := range pathMatchers {
							if path.GetExact() != "" {
								ret = append(ret, path.GetExact())
							} else if path.GetPrefix() != "" {
								ret = append(ret, path.GetPrefix()+"*")
							} else if path.GetSuffix() != "" {
								ret = append(ret, "*"+path.GetSuffix())
							} else {
								return nil
							}
						}
						return ret
					}
					includePaths := extractPaths(trigger.IncludedPaths)
					excludePaths := extractPaths(trigger.ExcludedPaths)
					// Each trigger rule is translated to a separate authz rule.
					authzPolicy.Rules = append(authzPolicy.Rules, newRule(includePaths, excludePaths))
				}
			} else {
				// Only need a single authz rule if there is no trigger rule defined.
				authzPolicy.Rules = []*betapb.Rule{newRule(nil, nil)}
			}
		}

		output = append(output, &OutputPolicy{
			Name:         selector.Name,
			Namespace:    selector.Namespace,
			Comment:      fmt.Sprintf("converted from alpha authentication policy %s/%s, %s", input.Namespace, input.Name, selector.Comment),
			RequestAuthN: requestAuthn,
			Authz:        authzPolicy,
		})
	}
	return output
}

func extractMTLS(input *InputPolicy, result *ResultSummary) mTLSMode {
	if len(input.Policy.Peers) == 0 {
		return Unset
	}

	peerMethod := input.Policy.Peers[0]
	if peerMethod.GetJwt() != nil {
		result.addError(fmt.Sprintf("JWT is never supported in peer method"))
	} else if peerMethod.GetMtls() != nil {
		switch peerMethod.GetMtls().Mode {
		case authnpb.MutualTls_PERMISSIVE:
			return Permissive
		case authnpb.MutualTls_STRICT:
			return Strict
		default:
			result.addError(fmt.Sprintf("found unsupported mTLS mode %s", peerMethod.GetMtls().Mode))
		}
	} else {
		result.addError(fmt.Sprintf("Neither mTLS nor JWT peer method specified"))
	}

	return Unset
}

func toStr(ports []uint32) []string {
	var ret []string
	for _, p := range ports {
		ret = append(ret, fmt.Sprintf("%d", p))
	}
	return ret
}
