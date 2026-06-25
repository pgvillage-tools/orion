// Package etcdv3_test will run integration tests for using etcd as backend with v3 api
package etcdv3_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	apiCmd "github.com/pgvillage-tools/orion/cmd/api/cmd"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
	"github.com/testcontainers/testcontainers-go/network"
)

var _ = Describe("Smoke", Ordered, func() {
	const (
		numEtcd    = 1
		autoRemove = true
		numKeepers = 3
		localHost  = "127.0.0.1"

		pgPassword = "test123"
	)
	var (
		ctx              context.Context
		nw               *testcontainers.DockerNetwork
		etcdContainer    *etcd.EtcdContainer
		etcdEndpoints    string
		apiPort          uint16
		sentinelCnt      testcontainers.Container
		proxyCnt         testcontainers.Container
		keeperContainers []testcontainers.Container
		allContainers    []testcontainers.Container
		keeperSettings   = map[string]string{
			"pg-repl-password": pgPassword,
			"pg-su-password":   pgPassword,
		}
		pgConn = pgConnParams{
			"host":     "localhost",
			"user":     pgUser,
			"password": pgPassword,
			"dbname":   pgDatabase,
		}
	)

	BeforeAll(func() {
		// RYUK requires permissions we don't need and don't want to implement
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		ctx = context.Background()

		var nwErr error
		nw, nwErr = network.New(ctx)
		Ω(nwErr).NotTo(HaveOccurred())

		// setup etcd
		var etcdErr error
		etcdContainer, etcdEndpoints, etcdErr = runEtcd(ctx, etcdImage, nw)
		Ω(etcdErr).NotTo(HaveOccurred())
		allContainers = []testcontainers.Container{etcdContainer}

		// run api to control orion from this test framework
		aliases := map[string][]string{}
		apiCnt, initErr := runAPI(
			ctx,
			etcdEndpoints,
			nw,
			aliases,
		)
		Ω(initErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, apiCnt)
		port, err := apiCnt.MappedPort(ctx,
			fmt.Sprintf("%d/tcp", apiInternalPort))
		Ω(err).NotTo(HaveOccurred())
		apiPort = port.Num()

		initialCD := &apiv1.Spec{
			// DefaultSUReplAccessMode: util.ToPtr(apiv1.SUReplAccessStrict),
			DefaultSUReplAccessMode: util.ToPtr(apiv1.SUReplAccessAll),
			PGParameters:            apiv1.PGParameters{},
			PGHBA:                   []string{},
			InitMode:                util.ToPtr(apiv1.New),
		}
		jsonData, encodingErr := json.Marshal(initialCD)
		Ω(encodingErr).NotTo(HaveOccurred())
		initUrl := apiCmd.InitEndPoint.URL(apiCmd.HTTP, localHost, apiPort)
		initResp, initErr := http.Post(initUrl, "application/json", bytes.NewBuffer(jsonData))
		/*
			// If you want to something else then POST or GET:
			initReq, initReqErr := http.NewRequest("PATCH", initURL, bytes.NewBuffer(jsonData))
			Ω(initReqErr).NotTo(HaveOccurred())
			initReq.Header.Set("Content-Type", "application/json")
			client := &http.Client{}
			initResp, initRespErr := client.Do(initReq)
		*/
		Ω(initErr).NotTo(HaveOccurred())
		defer initResp.Body.Close()
		Ω(initResp.StatusCode).To(BeElementOf([]int{http.StatusOK, http.StatusCreated, http.StatusAccepted}))

		// Start sentinel
		var sentinelErr error
		sentinelCnt, sentinelErr = runSentinel(ctx, etcdEndpoints, nw)
		Ω(sentinelErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, sentinelCnt)

		// start keeper(s)
		for i := 0; i < numKeepers; i++ {
			alias := fmt.Sprintf("keeper_%d", i)
			settings := maps.Clone(keeperSettings)
			settings["pg-listen-address"] = alias
			aliases := map[string][]string{}
			aliases[nw.Name] = []string{alias}
			cnt, keeperErr := runKeeper(ctx, etcdEndpoints, nw, aliases, settings)
			Ω(keeperErr).NotTo(HaveOccurred())
			keeperContainers = append(keeperContainers, cnt)
			allContainers = append(allContainers, cnt)
		}

		// Start proxy
		var proxyErr error
		aliases[nw.Name] = []string{"proxy"}
		proxyCnt, proxyErr = runProxy(ctx, etcdEndpoints, nw, aliases)
		Ω(proxyErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, proxyCnt)

		/*
			logs, logErr := cnt.Logs(ctx)
			Ω(logErr).NotTo(HaveOccurred())
			data, readErr := io.ReadAll(logs)
			Ω(readErr).NotTo(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "DEBUG - Logs: %s", string(data))
		*/
		// wait for postgres to be available
	})
	AfterAll(func() {
		if !autoRemove {
			return
		}
		if CurrentSpecReport().Failed() {
			GinkgoWriter.Printf("Test failed! not cleaning containers")
			return
		}
		for _, cnt := range allContainers {
			Ω(cnt.Terminate(ctx)).NotTo(HaveOccurred())
		}
		Ω(nw.Remove(ctx)).NotTo(HaveOccurred())
	})
	Context("when connecting to the keepers", func() {
		It("should work properly", func() {
			for _, cnt := range keeperContainers {
				natPort, err := cnt.MappedPort(ctx,
					fmt.Sprintf("%d/tcp", keeperInternalPort))
				Ω(err).NotTo(HaveOccurred())
				Ω(pgPing(
					ctx,
					pgConn.setParam("port", natPort.Port())),
				).NotTo(HaveOccurred())
			}
		})
	})
	Context("when connecting through proxy", func() {
		It("should work properly", func() {
			proxyPort, err := proxyCnt.MappedPort(ctx,
				fmt.Sprintf("%d/tcp", proxyInternalPort))
			Ω(err).NotTo(HaveOccurred())
			proxyConnSettings := pgConn.setParam("port", proxyPort.Port())
			// This does not work directly after starting the container but does after 5s.
			// So, we will try this for 10 seconds
			isReadyCtx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second*10))
			defer cancelFunc()
			// every 100 miliseconds
			isReadyErr := isReady(isReadyCtx, proxyConnSettings, time.Millisecond*100)
			Ω(isReadyErr).NotTo(HaveOccurred())
		})
	})
})
