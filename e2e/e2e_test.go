package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var (
	istioCluster14 = "addon116"
	istioCluster16 = "asm-test"
)

func setup(t *testing.T, name string) func() {
	createPolicy14 := exec.Command("kubectl", "apply", "-f", fmt.Sprintf("testdata/%s-input.yaml", name), "--context", istioCluster14)
	if out, err := createPolicy14.CombinedOutput(); err != nil {
		t.Fatalf(fmt.Sprintf("failed to create policy in 1.4 cluster: %v\n%s", err, out))
	} else {
		t.Logf("create policy in 1.4 cluster: \n%s", out)
	}

	convertPolicy := exec.Command("../out/convert", "--context", istioCluster14)
	f, err := os.OpenFile(fmt.Sprintf("testdata/%s-output.yaml", name), os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	convertPolicy.Stdout = f
	errout := &strings.Builder{}
	convertPolicy.Stderr = errout
	if err := convertPolicy.Run(); err != nil {
		t.Fatalf(fmt.Sprintf("failed to convert policy in 1.4 cluster: %v\n%s", err, errout))
	} else {
		t.Logf("convert policy in 1.4 cluster: \n%s", errout)
	}

	createPolicy16 := exec.Command("kubectl", "apply", "-f", fmt.Sprintf("testdata/%s-output.yaml", name), "--context", istioCluster16)
	if out, err := createPolicy16.CombinedOutput(); err != nil {
		t.Fatalf(fmt.Sprintf("failed to create policy in 1.6 cluster: %v\n%s", err, out))
	} else {
		t.Logf("create policy in 1.6 cluster: \n%s", out)
	}

	return func() {
		deletePolicy14 := exec.Command("kubectl", "delete", "-f", fmt.Sprintf("testdata/%s-input.yaml", name), "--context", istioCluster14)
		if out, err := deletePolicy14.CombinedOutput(); err != nil {
			t.Fatalf(fmt.Sprintf("failed to delete policy in 1.4 cluster: %v\n%s", err, out))
		} else {
			t.Logf("delete policy in 1.4 cluster: \n%s", out)
		}

		deletePolicy16 := exec.Command("kubectl", "delete", "-f", fmt.Sprintf("testdata/%s-output.yaml", name), "--context", istioCluster16)
		if out, err := deletePolicy16.CombinedOutput(); err != nil {
			t.Fatalf(fmt.Sprintf("failed to delete policy in 1.6 cluster: %v\n%s", err, out))
		} else {
			t.Logf("delete policy in 1.6 cluster: \n%s", out)
		}
	}
}

type verifyCmd struct {
	from    string
	to      string
	jwt     bool
	path    string
	want    []string
	notWant []string
}

func (vc verifyCmd) run(t *testing.T) {
	t.Logf("verify request %s -> %s%s", vc.from, vc.to, vc.path)

	split := strings.Split(vc.from, ".")
	fromDeploy, fromNs := split[0], split[1]

	ctx := istioCluster16
	getPodCmd := exec.Command("kubectl", "get", "pod", "-l", fmt.Sprintf("app=%s", fromDeploy), "-n", fromNs,
		"-o", "jsonpath={.items..metadata.name}", "--context", ctx)
	fromPod, err := getPodCmd.CombinedOutput()
	if err != nil {
		t.Fatalf(fmt.Sprintf("failed to get from pod %s: %v\n%s", vc.from, err, fromPod))
	}

	requestCmd := exec.Command("kubectl", "exec", "-t", string(fromPod), "-n", fromNs, "--context", ctx, "-c", fromDeploy, "--",
		"curl", "-v", "-s", fmt.Sprintf("http://%s%s", vc.to, vc.path))
	out, sendErr := requestCmd.CombinedOutput()

	for _, want := range vc.want {
		if !strings.Contains(string(out), want) {
			t.Errorf("want %q but not found in response:\n%s", want, out)
		}
	}
	for _, notWant := range vc.notWant {
		if strings.Contains(string(out), notWant) {
			t.Errorf("not want %s but found in response:\n%s", notWant, out)
		}
	}

	// Request error is expected in some cases.
	// Fail on request error only if there are unmatched `want` or `notWant`.
	if t.Failed() {
		if sendErr != nil {
			t.Errorf(fmt.Sprintf("failed to send request %s -> %s%s: %v\n%s", vc.from, vc.to, vc.path, err, out))
		}
	}
}

func TestE2E(t *testing.T) {
	// The test assumes the following deployment:
	// Namespace foo: sleep, httpbin and helloworld
	// Namespace foo: sleep, httpbin and helloworld
	// Namespace naked (no sidecar): sleep, httpbin and helloworld
	testCases := []struct {
		name   string
		verify []verifyCmd
	}{
		{
			name: "mtls-httpbin-helloworld",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/sleep"},
				},
				{
					from: "sleep.foo", to: "helloworld.foo:5000", path: "/hello",
					want: []string{"helloworld"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin"},
				},
				{
					from: "sleep.naked", to: "helloworld.foo:5000", path: "/hello",
					want: []string{"Connection reset by peer"},
				},
			},
		},
		{
			name: "mtls-httpbin",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/sleep"},
				},
				{
					from: "sleep.foo", to: "helloworld.foo:5000", path: "/hello",
					want: []string{"helloworld"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin"},
				},
				{
					from: "sleep.naked", to: "helloworld.foo:5000", path: "/hello",
					want: []string{"helloworld"},
				},
			},
		},
		{
			name: "mtls-mesh",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/sleep"},
				},
				{
					from: "sleep.foo", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/sleep"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/sleep"},
				},
				{
					from: "sleep.bar", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/sleep"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
			},
		},
		{
			name: "mtls-namespace",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/sleep"},
				},
				{
					from: "sleep.foo", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/sleep"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/sleep"},
				},
				{
					from: "sleep.bar", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin", "URI=spiffe://ymzhu-istio.svc.id.goog/ns/bar/sa/sleep"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{"By=spiffe://ymzhu-istio.svc.id.goog/ns/foo/sa/httpbin"},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clean := setup(t, tc.name)
			t.Cleanup(clean)
			for _, v := range tc.verify {
				v.run(t)
			}
		})
	}
}
