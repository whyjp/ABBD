package runscan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lines prints each record as "LINE KIND API to=TO ch=CHANNEL".
func lines(rs []Record) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "%d %s %s to=%s ch=%s\n", r.Evidence.Line, r.Kind, r.API, r.To, r.Channel)
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
	write("a/a.rs", "fn f() {\r\n    tokio::spawn(g());\r\n}\r\n")
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
	want := `{"kind":"spawn","api":"tokio::spawn","from":"","to":"g","channel":"","evidence":{"path":"a/a.rs","line":2,"quote":"tokio::spawn(g());"},"configs":[],"confidence":"syntactic","via":"catalog"}
{"kind":"spawn","api":"go","from":"","to":"g","channel":"","evidence":{"path":"b/b.go","line":3,"quote":"go g()"},"configs":[],"confidence":"syntactic","via":"catalog"}
`
	if runs[0].String() != want {
		t.Errorf("scan:\n%s\nwant:\n%s", runs[0].String(), want)
	}
	if runs[0].String() != runs[1].String() {
		t.Errorf("two scans differ")
	}
}
