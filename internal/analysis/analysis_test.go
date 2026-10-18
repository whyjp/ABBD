package analysis

import (
	"reflect"
	"slices"
	"testing"

	"github.com/whyjp/cccv/internal/model"
)

const sampleRoot = "../../samples/code-only"

func analyze(t *testing.T, root string) *Result {
	t.Helper()
	r, err := Analyze(root)
	if err != nil {
		t.Fatalf("Analyze(%s): %v", root, err)
	}
	return r
}

func module(t *testing.T, r *Result, p string) ModuleFacts {
	t.Helper()
	for _, m := range r.Modules {
		if m.Path == p {
			return m
		}
	}
	t.Fatalf("module %s not found", p)
	return ModuleFacts{}
}

func findContract(r *Result, kind, selector string) (ContractFacts, bool) {
	for _, c := range r.Contracts {
		if c.Kind == kind && c.Selector == selector {
			return c, true
		}
	}
	return ContractFacts{}, false
}

func TestSampleModules(t *testing.T) {
	r := analyze(t, sampleRoot)
	var paths []string
	for _, m := range r.Modules {
		paths = append(paths, m.Path)
	}
	want := []string{"cmd/heartbeat-worker", "internal/api", "internal/heartbeat", "internal/kafka", "internal/storage"}
	if !slices.Equal(paths, want) {
		t.Fatalf("modules = %v, want %v", paths, want)
	}
	if !slices.Equal(r.Languages, []string{"go"}) {
		t.Errorf("languages = %v", r.Languages)
	}

	worker := module(t, r, "cmd/heartbeat-worker")
	if !worker.IsMain || worker.ImportPath != "example.com/heartbeat/cmd/heartbeat-worker" || worker.Analyzer != "go/ast" {
		t.Errorf("worker facts = %+v", worker)
	}
	if !slices.Equal(worker.Imports, []string{"internal/api", "internal/heartbeat", "internal/kafka", "internal/storage"}) {
		t.Errorf("worker imports = %v", worker.Imports)
	}

	hb := module(t, r, "internal/heartbeat")
	if !slices.Contains(hb.Files, "internal/heartbeat/heartbeat_test.go") {
		t.Errorf("heartbeat files miss its test file: %v", hb.Files)
	}
	if !slices.ContainsFunc(hb.Symbols, func(s model.Symbol) bool {
		return s.Name == "Consumer.Run" && s.Kind == "method" && s.Location == "internal/heartbeat/consumer.go:26"
	}) {
		t.Errorf("Consumer.Run symbol missing: %v", hb.Symbols)
	}
	if slices.ContainsFunc(hb.Symbols, func(s model.Symbol) bool { return s.Name == "newDedupe" }) {
		t.Error("unexported func listed as a symbol")
	}
}

func TestSampleCalls(t *testing.T) {
	r := analyze(t, sampleRoot)
	const hb = "example.com/heartbeat/internal/heartbeat"
	cases := []CallEdge{
		{From: "example.com/heartbeat/cmd/heartbeat-worker.main", To: hb + ".NewConsumer", Confidence: ConfidenceStatic},
		{From: "example.com/heartbeat/cmd/heartbeat-worker.main", To: hb + ".Consumer.Run", Confidence: ConfidenceStatic},
		{From: hb + ".Consumer.Handle", To: hb + ".Validate", Confidence: ConfidenceStatic},
		{From: hb + ".Consumer.Run", To: "example.com/heartbeat/internal/kafka.Reader.ReadMessage", Confidence: ConfidencePossible},
		{From: hb + ".Consumer.Handle", To: hb + ".Store.Save", Confidence: ConfidencePossible},
	}
	var all []CallEdge
	for _, m := range r.Modules {
		all = append(all, m.Calls...)
	}
	for _, want := range cases {
		if !slices.ContainsFunc(all, func(e CallEdge) bool {
			return e.From == want.From && e.To == want.To && e.Confidence == want.Confidence
		}) {
			t.Errorf("missing call edge %s -> %s (%s)", want.From, want.To, want.Confidence)
		}
	}
}

func TestSampleContracts(t *testing.T) {
	r := analyze(t, sampleRoot)
	cases := []struct{ kind, selector, direction, owner string }{
		{KindKafka, "heartbeat-events", DirConsumes, "cmd/heartbeat-worker"},
		{KindKafka, "message-events", DirProduces, "internal/api"},
		{KindHTTPRoute, "POST /messages", DirExposes, "internal/api"},
		{KindHTTPRoute, "GET /health", DirExposes, "internal/api"},
		{KindEnvVar, "HEARTBEAT_DB_URL", DirReads, "cmd/heartbeat-worker"},
		{KindDBTable, "heartbeats", DirOwns, "internal/storage"},
		{KindCLICommand, "cmd:heartbeat-worker", DirExposes, "cmd/heartbeat-worker"},
	}
	for _, tc := range cases {
		t.Run(tc.kind+" "+tc.selector, func(t *testing.T) {
			c, ok := findContract(r, tc.kind, tc.selector)
			if !ok {
				t.Fatalf("contract not found in %+v", r.Contracts)
			}
			if c.Direction != tc.direction || c.OwnerPath != tc.owner || c.Origin != model.Extracted || c.Location == "" {
				t.Errorf("got %+v, want direction=%s owner=%s origin=EXTRACTED", c, tc.direction, tc.owner)
			}
		})
	}
	if c, _ := findContract(r, KindHTTPRoute, "POST /messages"); c.Symbol != "example.com/heartbeat/internal/api.Server.handlePostMessage" {
		t.Errorf("route handler symbol = %q", c.Symbol)
	}
	for _, c := range r.Contracts {
		if c.Kind == KindKafka && c.Selector != "heartbeat-events" && c.Selector != "message-events" {
			t.Errorf("unexpected topic %+v", c)
		}
	}
}

func TestSampleServing(t *testing.T) {
	r := analyze(t, sampleRoot)
	if len(r.Serving) != 1 {
		t.Fatalf("serving = %+v, want one Dockerfile unit", r.Serving)
	}
	s := r.Serving[0]
	if s.DeployUnit != "heartbeat-worker" || s.Source != "Dockerfile" || s.Origin != model.Extracted {
		t.Errorf("serving = %+v", s)
	}
	for _, p := range []string{"cmd/heartbeat-worker", "internal/heartbeat", "internal/storage"} {
		if !slices.Contains(s.ModulePaths, p) {
			t.Errorf("deploy unit does not ship %s: %v", p, s.ModulePaths)
		}
	}
}

func TestSampleTestsAndCommands(t *testing.T) {
	r := analyze(t, sampleRoot)
	byName := map[string]TestFacts{}
	for _, tf := range r.Tests {
		byName[tf.Name] = tf
	}
	for _, name := range []string{"TestValidate", "TestConsumerSkipsDuplicates", "TestWriterSaveIsIdempotent", "TestPostMessagePublishes", "TestHealth"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("test %s not discovered", name)
		}
	}
	if got := byName["TestValidate"]; got.ModulePath != "internal/heartbeat" || got.Command != "go test ./internal/heartbeat -run '^TestValidate$'" {
		t.Errorf("TestValidate = %+v", got)
	}
	if !slices.Contains(r.TestCommands, "go test ./...") || !slices.Contains(r.BuildCommands, "go vet ./...") || slices.Contains(r.BuildCommands, "go build ./...") {
		t.Errorf("commands = %v / %v", r.TestCommands, r.BuildCommands)
	}
	if len(r.Docs) != 0 {
		t.Errorf("code-only sample should have no docs, got %v", r.Docs)
	}
}

func TestAnalyzeIsDeterministic(t *testing.T) {
	a, b := analyze(t, sampleRoot), analyze(t, sampleRoot)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two runs over the same tree produced different results")
	}
}

func TestRuntimeAssembledSelectorsAreKeptAsUnknown(t *testing.T) {
	r := analyze(t, "testdata/runtime-topic")
	var topic, env *ContractFacts
	for i, c := range r.Contracts {
		switch c.Kind {
		case KindKafka:
			topic = &r.Contracts[i]
		case KindEnvVar:
			env = &r.Contracts[i]
		}
	}
	if topic == nil || topic.Origin != model.UnknownOrigin || topic.Note != "topic assembled at runtime" ||
		topic.Direction != DirConsumes || topic.OwnerPath != "consumer" {
		t.Errorf("runtime topic = %+v", topic)
	}
	if env == nil || env.Origin != model.UnknownOrigin {
		t.Errorf("runtime env var = %+v", env)
	}
}
