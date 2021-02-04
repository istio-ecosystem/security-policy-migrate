package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hashicorp/go-multierror"
)

const (
	// https://raw.githubusercontent.com/istio/istio/release-1.4/security/tools/jwt/samples/demo.jwt
	token = "eyJhbGciOiJSUzI1NiIsImtpZCI6IkRIRmJwb0lVcXJZOHQyenBBMnFYZkNtcjVWTzVaRXI0UnpIVV8tZW52dlEiLCJ0eXAiOiJKV1QifQ.eyJleHAiOjQ2ODU5ODk3MDAsImZvbyI6ImJhciIsImlhdCI6MTUzMjM4OTcwMCwiaXNzIjoidGVzdGluZ0BzZWN1cmUuaXN0aW8uaW8iLCJzdWIiOiJ0ZXN0aW5nQHNlY3VyZS5pc3Rpby5pbyJ9.CfNnxWP2tcnR9q0vxyxweaF3ovQYHYZl82hAUsn21bwQd9zP7c-LS9qd_vpdLG4Tn1A15NxfCjp5f7QNBUo-KC9PJqYpgGbaXhaGx7bEdFWjcwv3nZzvc7M__ZpaCERdwU7igUmJqYGBYQ51vr2njU9ZimyKkfDe3axcyiBZde7G6dabliUosJvvKOPcKIWPccCgefSj_GNfwIip3-SsFdlR7BtbVUcqR-yv-XOxJ3Uc1MI0tz3uMiiZcyPV7sNCU4KRnemRIMHVOfuvHsU60_GhGbiSFzgPTAa9WTltbnarTbxudb_YEOx12JiwYToeX0DCPb43W1tzIBxgm8NxUg"
)

var (
	istioCluster14 = "YOUR_ISTIO_14_CLUSTER"
	istioCluster16 = "YOUR_ISTIO_16_CLUSTER"
	trustDomain    = "cluster.local"
)

// newIdentity returns the Istio mTLS identity with a custom prefix.
func newIdentity(prefix, ns, sa string) string {
	return prefix + "spiffe://" + trustDomain + "/ns/" + ns + "/sa/" + sa
}

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
	f.Truncate(0)
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
	name := fmt.Sprintf("verify request %s -> %s%s", vc.from, vc.to, vc.path)
	if vc.jwt {
		name += " (jwt token)"
	}
	t.Log(name)

	split := strings.Split(vc.from, ".")
	fromDeploy, fromNs := split[0], split[1]

	ctx := istioCluster16
	getPodCmd := exec.Command("kubectl", "get", "pod", "-l", fmt.Sprintf("app=%s", fromDeploy), "-n", fromNs,
		"-o", "jsonpath={.items..metadata.name}", "--context", ctx)
	fromPod, err := getPodCmd.CombinedOutput()
	if err != nil {
		t.Fatalf(fmt.Sprintf("failed to get from pod %s: %v\n%s", vc.from, err, fromPod))
	}

	verifyFn := func() error {
		var err error
		requestCmd := exec.Command("kubectl", "exec", "-t", string(fromPod), "-n", fromNs, "--context", ctx, "-c", fromDeploy, "--",
			"curl", "-v", "-s", fmt.Sprintf("http://%s%s", vc.to, vc.path))
		if vc.jwt {
			requestCmd.Args = append(requestCmd.Args, "--header", fmt.Sprintf("Authorization: Bearer %s", token))
		}
		out, sendErr := requestCmd.CombinedOutput()

		var wantButNotFound []string
		for _, want := range vc.want {
			if !strings.Contains(string(out), want) {
				wantButNotFound = append(wantButNotFound, want)
			}
		}
		var notWantButFound []string
		for _, notWant := range vc.notWant {
			if strings.Contains(string(out), notWant) {
				notWantButFound = append(notWantButFound, notWant)
			}
		}

		if len(wantButNotFound) != 0 || len(notWantButFound) != 0 {
			var errmsg []string
			if len(wantButNotFound) != 0 {
				errmsg = append(errmsg, fmt.Sprintf("could not find [%s]", strings.Join(wantButNotFound, ", ")))
			}
			if len(notWantButFound) != 0 {
				errmsg = append(errmsg, fmt.Sprintf("found unexpected [%s]", strings.Join(notWantButFound, ", ")))
			}
			errmsg = append(errmsg, fmt.Sprintf("got response:\n%s", out))
			err = multierror.Append(err, fmt.Errorf("%s", strings.Join(errmsg, ", ")))
		}

		// Request error is expected in some cases.
		// Fail on request error only if there are unmatched `want` or `notWant`.
		if err != nil {
			if sendErr != nil {
				err = multierror.Append(err, fmt.Errorf(fmt.Sprintf("failed to send request %s -> %s%s: %v\n%s", vc.from, vc.to, vc.path, err, out)))
			}
		}

		return err
	}

	retry(t, 10, 3, verifyFn)
}

func retry(t *testing.T, failLimit, passLimit int, verify func() error) {
	var err error
	for i := 1; i <= failLimit; i++ {
		for j := 1; j <= passLimit; j++ {
			err = verify()
			if err == nil {
				if j == passLimit {
					return
				}
			} else {
				break
			}
		}
		t.Logf("failed and retrying...")
	}
	t.Errorf("all %d retries failed: %v", failLimit, err)
}

func TestE2E(t *testing.T) {
	// The test assumes the following deployment:
	// Namespace foo and bar: sleep, httpbin (8000, 80), helloworld (5000) and tcp-echo (9000, 9001)
	// Namespace naked (no sidecar): sleep, httpbin (8000, 80), helloworld (5000) and tcp-echo (9000, 9001)
	testCases := []struct {
		name   string
		verify []verifyCmd
	}{
		{
			name: "both-basic",
			verify: []verifyCmd{
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers", jwt: true,
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.bar", to: "helloworld.bar:5000", path: "/hello",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.bar", to: "helloworld.bar:5000", path: "/hello", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.naked", to: "helloworld.bar:5000", path: "/hello",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.naked", to: "helloworld.bar:5000", path: "/hello", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "both-port-level",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/10/1",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/10/1", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:80", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:80", path: "/links/10/1",
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:80", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "jwt-basic",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "jwt-jwks",
			verify: []verifyCmd{
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "jwt-port",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "jwt-trigger-rule-all",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/10/1",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/9/1", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/1/1",
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "jwt-trigger-rule-exclude",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/ip",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/ip", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/10/1",
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/links/9/1",
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "jwt-trigger-rule-include",
			verify: []verifyCmd{
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"RBAC: access denied"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/ip",
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/links/10/1", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/links/9/1", jwt: true,
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/links/8/1",
					want: []string{"RBAC: access denied"},
				},
			},
		},
		{
			name: "mtls-httpbin-helloworld",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.foo", to: "helloworld.foo:5000", path: "/hello",
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{newIdentity("By=", "foo", "httpbin")},
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
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.bar", to: "helloworld.bar:5000", path: "/hello",
					want: []string{"HTTP/1.1 200 OK"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{newIdentity("By=", "foo", "httpbin")},
				},
				{
					from: "sleep.naked", to: "helloworld.bar:5000", path: "/hello",
					want: []string{"HTTP/1.1 200 OK"},
				},
			},
		},
		{
			name: "mtls-mesh",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.foo", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.bar", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "bar", "sleep")},
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
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.foo", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.bar", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{newIdentity("By=", "foo", "httpbin")},
				},
			},
		},
		{
			name: "mtls-port",
			verify: []verifyCmd{
				{
					from: "sleep.foo", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.foo", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "foo", "sleep")},
				},
				{
					from: "sleep.bar", to: "httpbin.bar:8000", path: "/headers",
					want: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.bar", to: "httpbin.foo:8000", path: "/headers",
					want: []string{newIdentity("By=", "foo", "httpbin"), newIdentity("URI=", "bar", "sleep")},
				},
				{
					from: "sleep.naked", to: "httpbin.foo:8000", path: "/headers",
					want: []string{"Connection reset by peer"},
				},
				{
					from: "sleep.naked", to: "httpbin.bar:8000", path: "/headers",
					want:    []string{"HTTP/1.1 200 OK"},
					notWant: []string{newIdentity("By=", "bar", "httpbin"), newIdentity("By=", "foo", "httpbin")},
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
