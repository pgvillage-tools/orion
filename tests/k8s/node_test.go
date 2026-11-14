package k8s_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// kindNode is the node container of the kind cluster of kubeContext: kind-<cluster> runs in <cluster>-control-plane
func kindNode() string {
	return strings.TrimPrefix(kubeContext(), "kind-") + "-control-plane"
}

// containerTool runs the kind nodes: podman when kind uses podman (see the Makefile), else docker
func containerTool() string {
	if os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == "podman" {
		return "podman"
	}
	return "docker"
}

// onNode runs a command on the kind node, and returns its output
func onNode(ctx context.Context, args ...string) string {
	args = append([]string{"exec", kindNode()}, args...)
	out, err := exec.CommandContext(ctx, containerTool(), args...).CombinedOutput()
	fmt.Fprintf(GinkgoWriter, "$ %s %s\n%s", containerTool(), strings.Join(args, " "), out)
	Ω(err).NotTo(HaveOccurred(), "%s %s: %s", containerTool(), strings.Join(args, " "), out)
	return string(out)
}

// signalContainer sends sig (e.g. STOP or CONT) to the main process of a container of a pod, from the kind node.
// From inside the pod that doesn't work: the kernel ignores STOP for the first process of a pid namespace when it
// comes from within that namespace.
func signalContainer(ctx context.Context, pod, container, sig string) {
	id := strings.TrimSpace(onNode(ctx, "crictl", "ps", "-q", "--name", "^"+container+"$",
		"--label", "io.kubernetes.pod.name="+pod, "--label", "io.kubernetes.pod.namespace="+namespace))
	Ω(id).NotTo(BeEmpty(), "no container %s in pod %s", container, pod)
	Ω(id).NotTo(ContainSubstring("\n"), "more than one container %s in pod %s", container, pod)
	var inspect struct {
		Info struct {
			Pid int `json:"pid"`
		} `json:"info"`
	}
	Ω(json.Unmarshal([]byte(onNode(ctx, "crictl", "inspect", "-o", "json", id)), &inspect)).To(Succeed())
	Ω(inspect.Info.Pid).To(BeNumerically(">", 1))
	onNode(ctx, "kill", "-"+sig, strconv.Itoa(inspect.Info.Pid))
}
