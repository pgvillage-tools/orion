// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

// The commands are tested in-process: run executes CmdCLI with the given arguments, and the cli talks over mTLS to a
// fake api (an httptest server with a mux per spec). The certificates are created once, in BeforeSuite, by running
// `orion-cli pki init` itself. The commands share global state (cobra flags, option structs, viper), so the specs
// don't run in parallel, and run resets every flag before it executes a command.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// the suite state: set by the BeforeSuite, read by the harness
var (
	// testHome is $HOME during the tests: the config file of the cli and its certificates (~/.orion/cli) are in there
	testHome string
	// testAPIDir has the certificates of the (fake) api, created by `orion-cli pki init`
	testAPIDir string
	// serverTLS is the TLS config of the fake api: it requires a client certificate of the CA of the cli
	serverTLS *tls.Config
)

var _ = BeforeSuite(func() {
	const testPollWait = 5 * time.Millisecond
	testHome = GinkgoT().TempDir()
	// initConfig writes ~/.orion/config.yml and the cli finds its certificates in ~/.orion/cli: keep both out of the
	// home of the user
	GinkgoT().Setenv("HOME", testHome)
	for _, env := range os.Environ() {
		if name, value, _ := strings.Cut(env, "="); strings.HasPrefix(name, "ORION") {
			Ω(os.Unsetenv(name)).To(Succeed())
			DeferCleanup(os.Setenv, name, value)
		}
	}
	originalPollInterval := waitPollInterval
	waitPollInterval = testPollWait
	DeferCleanup(func() { waitPollInterval = originalPollInterval })

	// The cli certificates go to the default --pki-dir (~/.orion/cli, in testHome), which every spec uses unless it
	// calls usePKIDir.
	testAPIDir = filepath.Join(testHome, "api")
	_, _, err := execute(context.Background(), "", "pki", "init", "--api-dir", testAPIDir)
	Ω(err).NotTo(HaveOccurred(), "pki init failed")
	serverTLS, err = pki.APIFiles(testAPIDir).ServerConfig()
	Ω(err).NotTo(HaveOccurred())
})

// usePKIDir makes the cli use dir as its --pki-dir for the rest of the spec. A viper.Set overrules the flag; the
// cleanup sets it to nil, which viper treats as not set, so later specs get --pki-dir (or its default) again.
func usePKIDir(dir string) {
	viper.Set(flagPKIDir, dir)
	DeferCleanup(func() { viper.Set(flagPKIDir, nil) })
}

// resetCommands sets every flag of c and its subcommands back to its default, and clears what an earlier Execute
// left behind on the commands (the context, SilenceUsage)
func resetCommands(c *cobra.Command) {
	c.SilenceUsage = false
	// cobra keeps the context of an earlier run on a subcommand: clear it, so ExecuteContext sets it again
	c.SetContext(nil)
	for _, flags := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
		flags.VisitAll(func(f *pflag.Flag) {
			if slice, ok := f.Value.(pflag.SliceValue); ok {
				_ = slice.Replace(nil)
			} else if err := f.Value.Set(f.DefValue); err != nil {
				panic(fmt.Sprintf("failed to reset --%s to %q: %v", f.Name, f.DefValue, err))
			}
			f.Changed = false
		})
	}
	for _, sub := range c.Commands() {
		resetCommands(sub)
	}
}

// execute runs the cli with args (and stdin as its input), and returns its output
func execute(ctx context.Context, stdin string, args ...string) (string, string, error) {
	resetCommands(CmdCLI)
	var outBuf, errBuf bytes.Buffer
	// with nil arguments, cobra parses os.Args: the flags of the test binary (like -ginkgo.v)
	CmdCLI.SetArgs(append([]string{}, args...))
	CmdCLI.SetOut(&outBuf)
	CmdCLI.SetErr(&errBuf)
	CmdCLI.SetIn(strings.NewReader(stdin))
	defer func() {
		CmdCLI.SetArgs(nil)
		CmdCLI.SetOut(nil)
		CmdCLI.SetErr(nil)
		CmdCLI.SetIn(nil)
	}()
	err := CmdCLI.ExecuteContext(ctx)
	return outBuf.String(), errBuf.String(), err
}

// run runs the cli with args, and returns its output
func run(args ...string) (string, string, error) {
	return execute(context.Background(), "", args...)
}

// recordedRequest is a request the fake api received
type recordedRequest struct {
	Method      string
	Path        string
	Body        string
	IfMatch     string
	ContentType string
}

// fakeAPI is an api server with only the routes a spec registers, which records the requests it gets
type fakeAPI struct {
	mux    *http.ServeMux
	server *httptest.Server
	mu     sync.Mutex
	reqs   []recordedRequest
}

// newFakeAPI starts a fake api that, like the real one, requires a client certificate signed by the CA of the cli;
// it stops at the end of the spec
func newFakeAPI() *fakeAPI {
	f := &fakeAPI{mux: http.NewServeMux()}
	f.server = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.server.TLS = serverTLS.Clone()
	// a refused handshake is part of some specs: don't log it
	f.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	f.server.StartTLS()
	DeferCleanup(f.server.Close)
	return f
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	// the handler runs in a goroutine of the server: a failed assertion must reach the spec
	defer GinkgoRecover()
	body, err := io.ReadAll(r.Body)
	Ω(err).NotTo(HaveOccurred(), "failed to read the request body")
	r.Body = io.NopCloser(bytes.NewReader(body))
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedRequest{
		Method: r.Method, Path: r.URL.Path, Body: string(body),
		IfMatch: r.Header.Get("If-Match"), ContentType: r.Header.Get("Content-Type"),
	})
	f.mu.Unlock()
	f.mux.ServeHTTP(w, r)
}

// handle registers handler for pattern (a route of pkg/api_endpoints, like "GET /v1/cluster")
func (f *fakeAPI) handle(pattern string, handler http.HandlerFunc) {
	f.mux.HandleFunc(pattern, handler)
}

// requests returns the requests the fake api received
func (f *fakeAPI) requests() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest{}, f.reqs...)
}

// requestsTo returns the requests with method to path
func (f *fakeAPI) requestsTo(method, path string) []recordedRequest {
	var result []recordedRequest
	for _, r := range f.requests() {
		if r.Method == method && r.Path == path {
			result = append(result, r)
		}
	}
	return result
}

// port returns the port the fake api listens on
func (f *fakeAPI) port() string {
	GinkgoHelper()
	_, port, found := strings.Cut(f.server.Listener.Addr().String(), "127.0.0.1:")
	Ω(found).To(BeTrue(), "unexpected listen address %s", f.server.Listener.Addr())
	return port
}

// url returns the url of path on the fake api
func (f *fakeAPI) url(path string) string {
	return "https://127.0.0.1:" + f.port() + path
}

// run runs the cli against the fake api: args, followed by the flags for the api
func (f *fakeAPI) run(args ...string) (string, string, error) {
	return f.runWith(context.Background(), "", args...)
}

// runWith runs the cli against the fake api with a context and stdin
func (f *fakeAPI) runWith(ctx context.Context, stdin string, args ...string) (string, string, error) {
	args = append(args, "--host", "127.0.0.1", "--port", f.port(), "--timeout", "5s")
	return execute(ctx, stdin, args...)
}

// writeJSON answers code with v as JSON, and etag as the ETag when set
func writeJSON(w http.ResponseWriter, code int, v any, etag string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// accepted answers 202 Accepted with the status as Location, like the real api for a change the sentinel applies
func accepted(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Location", "/v1/cluster/status")
	w.WriteHeader(http.StatusAccepted)
}

// expectError fails the spec unless err is an error that starts with prefix and ends with suffix (when not empty)
func expectError(err error, prefix, suffix string) {
	GinkgoHelper()
	Ω(err).To(HaveOccurred(), "expected an error starting with %q", prefix)
	Ω(err.Error()).To(And(HavePrefix(prefix), HaveSuffix(suffix)))
}

// expectNoUsage fails the spec when the output of the cli has the usage: it's only printed for flag errors
func expectNoUsage(stdout, stderr string) {
	GinkgoHelper()
	Ω(stdout+stderr).NotTo(ContainSubstring("Usage:"),
		"expected no usage for an error after parsing the flags")
}

// expectContains fails the spec unless s has every one of parts
func expectContains(s string, parts ...string) {
	GinkgoHelper()
	for _, p := range parts {
		Ω(s).To(ContainSubstring(p))
	}
}

// expectNoRequests fails the spec when api received a request
func expectNoRequests(api *fakeAPI) {
	GinkgoHelper()
	Ω(api.requests()).To(BeEmpty(), "expected no requests")
}

// httpCodeSuffix is the end of an api error with code
func httpCodeSuffix(code int) string {
	return " (http " + strconv.Itoa(code) + ")"
}

// missingFile in the arguments of a table entry is replaced by a file that doesn't exist
const missingFile = "<missing file>"

// withMissingFile returns s with missingFile replaced by missing
func withMissingFile(missing string, s ...string) []string {
	result := make([]string, len(s))
	for i, v := range s {
		result[i] = strings.ReplaceAll(v, missingFile, missing)
	}
	return result
}
