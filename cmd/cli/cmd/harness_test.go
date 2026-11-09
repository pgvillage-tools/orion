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
// fake api (an httptest server with a mux per test). The certificates are created once, in TestMain, by running
// `orion-cli pki init` itself. The commands share global state (cobra flags, option structs, viper), so the tests
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
	"testing"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	testClusterName = "test-cluster"
	testPollWait    = 5 * time.Millisecond
)

var (
	// testHome is $HOME during the tests: the config file of the cli and its certificates (~/.orion/cli) are in there
	testHome string
	// testAPIDir has the certificates of the (fake) api, created by `orion-cli pki init`
	testAPIDir string
	// serverTLS is the TLS config of the fake api: it requires a client certificate of the CA of the cli
	serverTLS *tls.Config
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	var err error
	if testHome, err = os.MkdirTemp("", "orion-cli-test-"); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(testHome) }()
	// initConfig writes ~/.orion/config.yml and the cli finds its certificates in ~/.orion/cli: keep both out of the
	// home of the user
	if err = os.Setenv("HOME", testHome); err != nil {
		log.Fatal(err)
	}
	for _, env := range os.Environ() {
		if name, _, _ := strings.Cut(env, "="); strings.HasPrefix(name, "ORION") {
			_ = os.Unsetenv(name)
		}
	}
	waitPollInterval = testPollWait

	// The cli certificates go to the default --pki-dir (~/.orion/cli, in testHome): TestCLITLSFiles leaves a
	// viper.Set of that default behind, which overrules --pki-dir in later tests.
	testAPIDir = filepath.Join(testHome, "api")
	if _, _, err = execute(context.Background(), "", "pki", "init", "--api-dir", testAPIDir); err != nil {
		log.Fatalf("pki init failed: %v", err)
	}
	if serverTLS, err = pki.APIFiles(testAPIDir).ServerConfig(); err != nil {
		log.Fatal(err)
	}
	return m.Run()
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
	CmdCLI.SetArgs(args)
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
func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
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

// fakeAPI is an api server with only the routes a test registers, which records the requests it gets
type fakeAPI struct {
	t      *testing.T
	mux    *http.ServeMux
	server *httptest.Server
	mu     sync.Mutex
	reqs   []recordedRequest
}

// newFakeAPI starts a fake api that, like the real one, requires a client certificate signed by the CA of the cli
func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, mux: http.NewServeMux()}
	f.server = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.server.TLS = serverTLS.Clone()
	// a refused handshake is part of some tests: don't log it
	f.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	f.server.StartTLS()
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("failed to read the request body: %v", err)
	}
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
	_, port, found := strings.Cut(f.server.Listener.Addr().String(), "127.0.0.1:")
	if !found {
		f.t.Fatalf("unexpected listen address %s", f.server.Listener.Addr())
	}
	return port
}

// url returns the url of path on the fake api
func (f *fakeAPI) url(path string) string {
	return "https://127.0.0.1:" + f.port() + path
}

// run runs the cli against the fake api: args, followed by the flags for the api and the store
func (f *fakeAPI) run(args ...string) (string, string, error) {
	f.t.Helper()
	return f.runWith(context.Background(), "", args...)
}

// runWith runs the cli against the fake api with a context and stdin
func (f *fakeAPI) runWith(ctx context.Context, stdin string, args ...string) (string, string, error) {
	f.t.Helper()
	args = append(args, "--host", "127.0.0.1", "--port", f.port(), "--timeout", "5s",
		"--cluster-name", testClusterName, "--store-backend", "etcdv3")
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

// expectError fails t unless err is an error that starts with prefix and ends with suffix (when not empty)
func expectError(t *testing.T, err error, prefix, suffix string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error starting with %q, got none", prefix)
	}
	if !strings.HasPrefix(err.Error(), prefix) || !strings.HasSuffix(err.Error(), suffix) {
		t.Fatalf("expected an error like %q...%q, got %q", prefix, suffix, err)
	}
}

// expectNoUsage fails t when the output of the cli has the usage: it's only printed for flag errors
func expectNoUsage(t *testing.T, stdout, stderr string) {
	t.Helper()
	if strings.Contains(stdout+stderr, "Usage:") {
		t.Errorf("expected no usage for an error after parsing the flags, got:\n%s%s", stdout, stderr)
	}
}

// expectContains fails t unless s has every one of parts
func expectContains(t *testing.T, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			t.Errorf("expected the output to contain %q, got:\n%s", p, s)
		}
	}
}

// httpCodeSuffix is the end of an api error with code
func httpCodeSuffix(code int) string {
	return " (http " + strconv.Itoa(code) + ")"
}
