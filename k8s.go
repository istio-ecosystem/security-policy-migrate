package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/istio-ecosystem/security-policy-migrate/converter"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

const (
	meshConfigMapKey  = "mesh"
	meshConfigMapName = "istio"
	istioNamespace    = "istio-system"
)

var (
	gvrPolicies = []schema.GroupVersionResource{
		{Group: "authentication.istio.io", Version: "v1alpha1", Resource: "policies"},
		{Group: "authentication.istio.io", Version: "v1alpha1", Resource: "meshpolicies"},
	}
	gvrRbac = []schema.GroupVersionResource{
		{Group: "rbac.istio.io", Version: "v1alpha1", Resource: "rbacconfigs"},
		{Group: "rbac.istio.io", Version: "v1alpha1", Resource: "clusterrbacconfigs"},
		{Group: "rbac.istio.io", Version: "v1alpha1", Resource: "servicerolebindings"},
		{Group: "rbac.istio.io", Version: "v1alpha1", Resource: "serviceroles"},
	}
)

type kubeClient struct {
	dynamicClient dynamic.Interface
	kubeClient    *kubernetes.Clientset
	rootNamespace string
}

func newKubeClient(kubeconfig, configContext string) (*kubeClient, error) {
	if kubeconfig != "" {
		info, err := os.Stat(kubeconfig)
		if err != nil || info.Size() == 0 {
			return nil, fmt.Errorf("kubeconfig (%s) specified but could not be read: %w", kubeconfig, err)
		}
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeconfig
	configOverrides := &clientcmd.ConfigOverrides{
		ClusterDefaults: clientcmd.ClusterDefaults,
		CurrentContext:  configContext,
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
	if err != nil {
		return nil, err
	}

	kc := &kubeClient{dynamicClient: dynamic.NewForConfigOrDie(config), kubeClient: kubernetes.NewForConfigOrDie(config)}
	if err := kc.setRootnamespace(); err != nil {
		return nil, err
	}
	return kc, nil
}

func (kc *kubeClient) hasIstioNamespace() bool {
	ns, err := kc.kubeClient.CoreV1().Namespaces().Get(context.TODO(), istioNamespace, metav1.GetOptions{})
	return ns != nil && err == nil
}

func (kc *kubeClient) setRootnamespace() error {
	meshConfigMap, err := kc.kubeClient.CoreV1().ConfigMaps(istioNamespace).Get(context.TODO(), meshConfigMapName, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			log.Printf("could not find mesh config %s, using %s as default root namespace", meshConfigMapName, istioNamespace)
			kc.rootNamespace = istioNamespace
			return nil
		}
		return fmt.Errorf("failed to get meshconfig: %w", err)
	}
	configYaml, ok := meshConfigMap.Data[meshConfigMapKey]
	if !ok {
		return fmt.Errorf("missing config map key %q", meshConfigMapKey)
	}
	jsonData, err := yaml.YAMLToJSON([]byte(configYaml))
	if err != nil {
		return fmt.Errorf("failed converting YAML to JSON: %w", err)
	}
	jsonObject := map[string]interface{}{}
	if err := json.Unmarshal(jsonData, &jsonObject); err != nil {
		return fmt.Errorf("failed unmarshaling JSON object: %w", err)
	}
	if val, found := jsonObject["rootNamespace"]; found && val != nil {
		if v, ok := val.(string); ok && v != "" {
			kc.rootNamespace = v
			log.Printf("found root namespace: %s", kc.rootNamespace)
		}
	}
	if kc.rootNamespace == "" {
		log.Printf("root namespace not set, using %s as default", istioNamespace)
		kc.rootNamespace = istioNamespace
	}

	return nil
}

func (kc *kubeClient) convert() error {
	if !kc.hasIstioNamespace() {
		return fmt.Errorf("could not find %s namespace", istioNamespace)
	}

	// TODO: change to get specific service instead of listing all services.
	services, err := kc.kubeClient.CoreV1().Services(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}
	cvt := converter.NewConverter(kc.rootNamespace, services)
	hasError := false
	betaPolicyOutput := map[string]*strings.Builder{}
	for _, gvr := range gvrPolicies {
		objectList, err := kc.listResources(gvr)
		if err != nil {
			log.Printf("skipped resource %s: %v", gvr.Resource, err)
			continue
		}
		for _, item := range objectList.Items {
			policy, err := converter.ConvertToPolicy(item)
			if err != nil {
				return fmt.Errorf("failed to convert resource to authentication policy: %v", err)
			}
			output, summary := cvt.Convert(policy)
			if cnt := len(summary.Errors); cnt != 0 {
				errorOutput := fmt.Sprintf("\n\t* %s", strings.Join(summary.Errors, "\n\t* "))
				log.Printf("FAILED  converting policy %s/%s, found %d errors: %s", item.GetNamespace(), item.GetName(), cnt, errorOutput)
				hasError = true
			} else {
				log.Printf("SUCCESS converting policy %s/%s", item.GetNamespace(), item.GetName())
				for _, out := range output {
					key := "all"
					if perNamespace != "" {
						key = out.Namespace
					}
					if _, ok := betaPolicyOutput[key]; !ok {
						betaPolicyOutput[key] = &strings.Builder{}
					}
					betaPolicyOutput[key].WriteString(out.ToYAML())
				}
			}
		}
	}

	var rbacResources []string
	for _, gvr := range gvrRbac {
		objectList, err := kc.listResources(gvr)
		if err != nil {
			continue
		}
		for _, item := range objectList.Items {
			rbacResources = append(rbacResources, fmt.Sprintf("%s: %s/%s", item.GetKind(), item.GetNamespace(), item.GetName()))
		}
	}
	if len(rbacResources) != 0 {
		errorOutput := fmt.Sprintf("\n\t* %s", strings.Join(rbacResources, "\n\t* "))
		log.Printf("FAILED  found %d RBAC resources, this tool only supports converting authentication policy, "+
			"check https://istio.io/latest/blog/2019/v1beta1-authorization-policy/#migration-from-the-v1alpha1-policy for converting RBAC resources manually: %s", len(rbacResources), errorOutput)
		hasError = true
	}

	if hasError {
		if ignoreError {
			log.Printf("Found errors but ignored with --ignore-error, the converted policies may not work as expected")
		} else {
			// TODO: add a link to the istio.io conversion documentation.
			return fmt.Errorf("conversion failed, found errors during conversion, please fix errors and re-run the tool again")
		}
	}

	if perNamespace != "" {
		for ns, out := range betaPolicyOutput {
			filename := fmt.Sprintf("%s/ns-%s.yaml", perNamespace, ns)
			log.Printf("Writing to %s for namespace %s", filename, ns)
			err := ioutil.WriteFile(filename, []byte(out.String()), 0644)
			if err != nil {
				return fmt.Errorf("write to %s failed: %v", filename, err)
			}
		}
	} else {
		fmt.Printf(betaPolicyOutput["all"].String())
	}
	return nil
}

func (kc *kubeClient) listResources(gvr schema.GroupVersionResource) (*unstructured.UnstructuredList, error) {
	return kc.dynamicClient.Resource(gvr).Namespace(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
}
