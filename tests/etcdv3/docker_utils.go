package etcdv3_test

import (
	"context"

	"github.com/moby/moby/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
)

// pauseContainer pauses cnt (like docker pause): its processes are frozen, but it keeps its network address, so
// clients get no answer instead of a refused connection. It is unpaused when the spec ends, or earlier with
// unpauseContainer.
func pauseContainer(ctx context.Context, cnt testcontainers.Container) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	Ω(err).NotTo(HaveOccurred())
	defer func() { _ = cli.Close() }()
	_, err = cli.ContainerPause(ctx, cnt.GetContainerID(), client.ContainerPauseOptions{})
	Ω(err).NotTo(HaveOccurred())
	DeferCleanup(func(ctx context.Context) {
		// it may be unpaused already
		_ = unpause(ctx, cnt)
	})
}

// unpauseContainer resumes a container that was paused with pauseContainer
func unpauseContainer(ctx context.Context, cnt testcontainers.Container) {
	Ω(unpause(ctx, cnt)).To(Succeed())
}

func unpause(ctx context.Context, cnt testcontainers.Container) error {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()
	_, err = cli.ContainerUnpause(ctx, cnt.GetContainerID(), client.ContainerUnpauseOptions{})
	return err
}

// signalContainer sends signal (e.g. SIGTERM) to the main process of cnt, like docker kill --signal
func signalContainer(ctx context.Context, cnt testcontainers.Container, signal string) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	Ω(err).NotTo(HaveOccurred())
	defer func() { _ = cli.Close() }()
	_, err = cli.ContainerKill(ctx, cnt.GetContainerID(), client.ContainerKillOptions{Signal: signal})
	Ω(err).NotTo(HaveOccurred())
}
