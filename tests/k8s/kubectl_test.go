package k8s_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// namespace is where the tests deploy orion (the kustomization in manifests sets it too)
	namespace = "orion"
	// defaultKubeContext is the context of the kind cluster of make kind-e2e-setup
	defaultKubeContext = "kind-orion-e2e"
	// portForwardTimeout is how long kubectl port-forward may take to listen
	portForwardTimeout = 30 * time.Second
)

// kubeContext is the kubectl context of the cluster to test in: KUBE_CONTEXT, or the kind cluster of the Makefile
func kubeContext() string {
	if c := os.Getenv("KUBE_CONTEXT"); c != "" {
		return c
	}
	return defaultKubeContext
}

// kubectl runs kubectl with args in the namespace of the tests, and returns its output (stdout and stderr)
func kubectl(ctx context.Context, args ...string) (string, error) {
	args = append([]string{"--context", kubeContext(), "--namespace", namespace}, args...)
	out, err := exec.CommandContext(ctx, "kubectl", args...).CombinedOutput()
	fmt.Fprintf(GinkgoWriter, "$ kubectl %s\n%s", strings.Join(args, " "), out)
	if err != nil {
		return string(out), fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

// mustKubectl runs kubectl like kubectl, and fails the spec when it fails
func mustKubectl(ctx context.Context, args ...string) string {
	out, err := kubectl(ctx, args...)
	Ω(err).NotTo(HaveOccurred())
	return out
}

var forwardingRe = regexp.MustCompile(`Forwarding from 127\.0\.0\.1:(\d+) ->`)

// portForward forwards a free local port to port of target (e.g. svc/orion-api), and returns the local port. The
// forward stops when ctx is done.
func portForward(ctx context.Context, target string, port int) uint16 {
	cmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext(), "--namespace", namespace,
		"port-forward", target, fmt.Sprintf(":%d", port))
	stdout, err := cmd.StdoutPipe()
	Ω(err).NotTo(HaveOccurred())
	cmd.Stderr = GinkgoWriter
	Ω(cmd.Start()).To(Succeed())

	localPort := make(chan uint16, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Fprintf(GinkgoWriter, "port-forward %s: %s\n", target, scanner.Text())
			if m := forwardingRe.FindStringSubmatch(scanner.Text()); m != nil {
				if p, parseErr := strconv.ParseUint(m[1], 10, 16); parseErr == nil {
					localPort <- uint16(p)
				}
			}
		}
		_ = cmd.Wait()
	}()
	select {
	case p := <-localPort:
		return p
	case <-time.After(portForwardTimeout):
		Fail(fmt.Sprintf("kubectl port-forward %s didn't start within %s", target, portForwardTimeout))
	}
	return 0
}

// showState prints the state of the namespace and the logs of the orion pods, for a failed spec
func showState(ctx context.Context) {
	for _, args := range [][]string{
		{"get", "pods,statefulsets,deployments,services,leases,configmaps", "-o", "wide"},
		{"get", "events", "--sort-by", ".lastTimestamp"},
		{"logs", "-l", "orion-cluster", "--all-containers", "--prefix", "--tail", "300", "--max-log-requests", "10"},
		{"logs", "-l", "app=orion-api", "--all-containers", "--prefix", "--tail", "300", "--max-log-requests", "10"},
	} {
		out, _ := kubectl(ctx, args...)
		fmt.Printf("===== kubectl %s =====\n%s\n", strings.Join(args, " "), out)
	}
}
