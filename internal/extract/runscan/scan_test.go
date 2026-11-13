package runscan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// lines prints each record as "LINE KIND API to=TO ch=CHANNEL".
func lines(rs []Record) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "%d %s %s to=%s ch=%s\n", r.Evidence.Line, r.Kind, r.API, r.To.Name, r.Channel)
	}
	return b.String()
}

func check(t *testing.T, path, src, want string) {
	t.Helper()
	got := lines(ScanFile(path, []byte(strings.TrimPrefix(src, "\n"))))
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Errorf("%s records:\n%s\nwant:\n%s", path, got, strings.TrimPrefix(want, "\n"))
	}
}

func TestC(t *testing.T) {
	check(t, "lib/pool.c", `
#include <pthread.h>
#define ZSTD_pthread_create(a, b, c, d) pthread_create((a), (b), (c), (d))
int ZSTD_pthread_create(ZSTD_pthread_t* t, const void* u, void* (*start)(void*), void* arg);
/* pthread_create(&t, NULL, commented, NULL); */
static void* worker(void* opaque) {
    ZSTD_pthread_mutex_lock(&ctx->queueMutex);
    ZSTD_pthread_cond_wait(&ctx->queuePopCond, &ctx->queueMutex);
    return NULL;
}
void start(POOL_ctx* ctx) {
    const char* s = "pthread_create(";
    if (ZSTD_pthread_create(&ctx->threads[i], NULL, &worker, ctx)) return;
    h = CreateThread(NULL, 0, (LPTHREAD_START_ROUTINE)worker, ctx, 0, NULL);
    t = (HANDLE)_beginthreadex(NULL, 0, worker, ctx, 0, NULL);
    ZSTD_pthread_cond_signal(&ctx->queuePopCond);
    std::thread th(run, 3);
}
`, `
7 recv ZSTD_pthread_cond_wait to= ch=queuePopCond
12 spawn ZSTD_pthread_create to= ch=
13 spawn CreateThread to=worker ch=
14 spawn _beginthreadex to=worker ch=
15 send ZSTD_pthread_cond_signal to= ch=queuePopCond
16 spawn std::thread to=run ch=
`)
}

func TestRust(t *testing.T) {
	check(t, "src/app.rs", `
use tokio::sync::mpsc;
#[tokio::main]
async fn main() {
    let rt = tokio::runtime::Builder::new_multi_thread().build().unwrap();
    let (tx, mut rx) = mpsc::channel::<Event>(100);
    tokio::spawn(pump(tx));
    let handle = std::thread::spawn(move || loop {});
    // tokio::spawn(commented());
    let s = "tokio::spawn(x)";
    let c = Command::new("ls").spawn();
    while let Some(ev) = rx.recv().await {}
    tx.send(ev).await;
    let lt: &'static str = r#"thread::spawn("#;
    set.spawn(async move { work().await });
}
pub fn spawn(f: F) {}
#[cfg(test)]
mod tests {
    fn t() { tokio::spawn(x); }
}
`, `
2 spawn #[tokio::main] to= ch=
4 spawn tokio Builder::new_* to= ch=
6 spawn tokio::spawn to=pump ch=
7 spawn std::thread::spawn to= ch=
11 recv recv to= ch=tx/rx
12 send send to= ch=tx/rx
14 spawn spawn (method) to= ch=
`)
}

func TestGo(t *testing.T) {
	check(t, "internal/run/run.go", "\n"+`package run

func Start(jobs chan<- Job, done <-chan struct{}) {
	go worker(jobs)
	go func() { jobs <- Job{} }()
	s := "go nothing()"
	// go commented()
	select {
	case j := <-done:
		_ = j
	}
	v := <-results
	_ = s
}
`, `
4 spawn go to=worker ch=
5 send <- to= ch=jobs
5 spawn go to= ch=
9 recv <- to= ch=done
12 recv <- to= ch=results
`)
}

func TestTestCodeIsNotRead(t *testing.T) {
	for _, p := range []string{"tests/a.rs", "src/test_util/mod.rs", "x/run_test.go", "lib/fuzz/f.c", "README.md"} {
		if LangOf(p) != "" {
			t.Errorf("LangOf(%q) = %q, want none", p, LangOf(p))
		}
	}
	if LangOf(`src\win\thread.c`) != C {
		t.Errorf("a Windows path is not read as C")
	}
}

func TestScanIsSortedAndDeterministic(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b/b.go", "package b\nfunc f() {\n\tgo g()\n}\n")
	write("a/a.rs", "fn f() {\r\n    tokio::spawn(g());\r\n}\r\nasync fn g() {}\r\n")
	write("tests/t.rs", "fn f() { tokio::spawn(g()); }\n")
	var runs [2]bytes.Buffer
	for i := range runs {
		rs, err := Scan(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteJSON(&runs[i], rs); err != nil {
			t.Fatal(err)
		}
	}
	want := `{"kind":"spawn","api":"tokio::spawn","module":"","from":"","to":{"name":"g","def":"a/a.rs:4"},"channel":"","evidence":{"path":"a/a.rs","line":2,"quote":"tokio::spawn(g());"},"configs":[],"confidence":"syntactic","via":"catalog"}
{"kind":"spawn","api":"go","module":"","from":"","to":{"name":"g"},"channel":"","evidence":{"path":"b/b.go","line":3,"quote":"go g()"},"configs":[],"confidence":"syntactic","via":"catalog"}
`
	if runs[0].String() != want {
		t.Errorf("scan:\n%s\nwant:\n%s", runs[0].String(), want)
	}
	if runs[0].String() != runs[1].String() {
		t.Errorf("two scans differ")
	}
}

func TestCWrappersPoolsAndDirectives(t *testing.T) {
	check(t, "lib/io.cpp", `
void start(int n) {
#if defined(X)
    use_x();
#endif
    pthread_create(&t, NULL, run, NULL);
    ThreadPool executor(n);
    threads_.emplace_back([this] { loop(); });
    ctx->pool = POOL_create_advanced(n, 0, mem);
    job = AIO_IOPool_createIoJob(ctx, size);
#pragma omp parallel default(shared)
    { work(); }
}
`, `
5 spawn pthread_create to=run ch=
6 spawn thread pool object to= ch=
7 spawn thread container to= ch=
8 spawn POOL_create_advanced to= ch=
10 spawn #pragma omp parallel to= ch=
`)
}

func TestRustWrappersAndTestGatedCode(t *testing.T) {
	check(t, "src/topology/running.rs", `
pub fn spawn_named<T: Future>(task: T, name: &str) { tokio::spawn(task.instrument(span(name))); }
fn start(&mut self) {
    let h = spawn_named(task, "sink");
    auth.spawn_regenerate_token();
    crate::spawn_in_current_span(async move {});
    let respawn_on_exit = true;
}
fn serve(addr: Addr) { tokio::spawn(async move { listen(addr).await }); }
fn run() { serve(addr); }
#[cfg(not(test))]
fn real() { tokio::spawn(x()); }
#[cfg(all(test, feature = "redis-integration-tests"))]
mod integration_tests {
    fn t() { tokio::spawn(x()); spawn_named(y(), "t"); }
}
`, `
1 spawn tokio::spawn to=instrument ch=
3 spawn running::spawn_named to=task ch=
8 spawn tokio::spawn to= ch=
11 spawn tokio::spawn to=x ch=
`)
	if rs := ScanFile("src/sinks/http/tests.rs", []byte("fn f() { tokio::spawn(x()); }\n")); len(rs) != 0 {
		t.Errorf("a tests.rs module is read: %v", rs)
	}
}

// scanTree writes files under a temporary root, scans it and prints its
// spawn records as "PATH:LINE API to=TO def=DEF".
func scanTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, s := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(strings.TrimPrefix(s, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, r := range rs {
		if r.Kind == Spawn {
			fmt.Fprintf(&b, "%s:%d %s to=%s def=%s\n", r.Evidence.Path, r.Evidence.Line, r.API, r.To.Name, r.To.Def)
		}
	}
	return b.String()
}

func TestWrappersAndAliasesAcrossFiles(t *testing.T) {
	got := scanTree(t, map[string]string{
		"core/src/async_runtime.rs": `
use tokio::runtime::{Handle as TokioHandle, Runtime as TokioRuntime};
pub fn spawn<F: Future>(task: F) -> JoinHandle<F::Output> {
  let runtime = RUNTIME.get_or_init(default_runtime);
  runtime.spawn(task)
}
fn default_runtime() -> Runtime { Runtime::Tokio(TokioRuntime::new().unwrap()) }
`,
		"core/src/app.rs": `
fn setup() {
  crate::async_runtime::spawn(async move { listen().await });
  let child = SharedChild::spawn(&mut cmd);
  let rt = TokioRuntime::new();
}
`,
		"lib/thread.h": `
#define start_thread_alias pthread_create
int os_thread_run(os_thread_t *t, void *(*fn)(void *), void *arg);
`,
		"lib/thread.c": `
int os_thread_run(os_thread_t *t, void *(*fn)(void *), void *arg)
{
	return pthread_create(&t->handle, NULL, fn, arg);
}
static void *run_loop(void *p) { return p; }
static int start_loop(struct ctx *c)
{
	return pthread_create(&c->thread, NULL, run_loop, c);
}
void begin(void)
{
	os_thread_run(&t, run_loop, NULL);
	start_loop(&c);
	start_thread_alias(&t, NULL, run_loop, NULL);
}
`,
		"lib/user.c": `
void other(void) { os_thread_run(&t, worker, NULL); }
`,
		"run/pool.go": `
package run

func Go(f func()) { go func() { f() }() }
func Watch(dir string, on func()) { go func() { for range events(dir) { on() } }() }
func Start(n int) { go serve(n) }
`,
		"cmd/main.go": `
package main

func main() { run.Go(work); run.Start(3); run.Watch(".", work) }
`,
	})
	want := `cmd/main.go:3 run.Go to=work def=
core/src/app.rs:2 async_runtime::spawn to= def=
run/pool.go:3 go to= def=
run/pool.go:5 go to=serve def=
core/src/async_runtime.rs:4 spawn (method) to=task def=
core/src/async_runtime.rs:6 tokio Runtime::new to= def=
lib/thread.c:3 pthread_create to=fn def=
lib/thread.c:8 pthread_create to=run_loop def=lib/thread.c:5
lib/thread.c:12 os_thread_run to=run_loop def=lib/thread.c:5
lib/thread.c:14 pthread_create to=run_loop def=lib/thread.c:5
lib/user.c:1 os_thread_run to=worker def=
`
	for _, line := range strings.Split(strings.TrimSpace(want), "\n") {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("missing %q in:\n%s", line, got)
		}
	}
	for _, bad := range []string{"core/src/app.rs:3 ", "lib/thread.c:13 ", "cmd/main.go:3 run.Start", "run.Watch"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q is no spawn through a wrapper:\n%s", bad, got)
		}
	}
}

func TestReviewedEdgeCases(t *testing.T) {
	check(t, "x/turkish.go", "\npackage p\n\nfunc İşle(done chan int) {\n\tgo worker(done)\n<-done\n\tx := 1\n<-done\n}\n", `
4 spawn go to=worker ch=
5 recv <- to= ch=done
7 recv <- to= ch=done
`)
	check(t, "win/t.c", `
void start(void) {
#ifdef _WIN32
    CreateThread(NULL, 0, run, NULL, 0, NULL);
#endif
}
`, `
3 spawn CreateThread to=run ch=
`)
}

func TestAPostsToQuotingAnyEndOfItsPairIsStored(t *testing.T) {
	rs := []Record{
		rec(Send, "cond_signal", "", "popCond", "lib/pool/pool.c", 20, "cond_signal(&ctx->popCond);"),
		rec(Send, "cond_signal", "", "popCond", "lib/pool/pool.c", 30, "cond_broadcast(&ctx->popCond);"),
		rec(Recv, "cond_wait", "", "popCond", "lib/pool/pool.c", 40, "cond_wait(&ctx->popCond, &m);"),
	}
	m := mod("MOD-0002", "lib/pool")
	m.Relations = []model.Relation{{To: "EXEC-0001", Kind: model.PostsTo, Evidence: []model.Evidence{
		{Path: "lib/pool/pool.c", Lines: "30", Quote: "cond_broadcast"},
		{Path: "lib/pool/pool.c", Lines: "40", Quote: "cond_wait"},
	}}}
	cs := Candidates(rs, []model.Unit{m})
	if len(cs) != 1 || !cs[0].Stored {
		t.Errorf("a posts_to quoting the second send and the recv should hold the pair:\n%s", show(cs))
	}
}
