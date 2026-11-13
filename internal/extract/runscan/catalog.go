package runscan

import "regexp"

// Lang is a source language the catalog knows.
type Lang string

const (
	C    Lang = "c"
	Rust Lang = "rust"
	Go   Lang = "go"
)

// Entry is one catalog pattern. Pattern runs over the code with its
// comments and string contents blanked out. A named group "at" marks where
// the site starts, "api" names the call when one pattern covers several,
// "fn" the function a spawn runs and "ch" the channel. Without "fn" or
// "ch", FnArg and ChanArg pick the call's argument, counted from 1, so the
// pattern then ends at the call's opening parenthesis.
type Entry struct {
	Lang    Lang
	Kind    Kind
	API     string
	Pattern *regexp.Regexp
	FnArg   int
	ChanArg int
	// Call marks a named call, which reads as a declaration when a type
	// name stands right before it, as in "int thread_create(".
	Call bool
	// Args marks a call that counts only with arguments, as a method named
	// spawn on a process builder takes none.
	Args bool
	// Directive marks a preprocessor line that starts threads itself, such
	// as #pragma omp parallel, which the scan reads although it skips the
	// other preprocessor lines.
	Directive bool
	// Needles are lower-case words one of which stands on every line the
	// pattern can match, so the pattern runs only near them.
	Needles []string
}

func entry(l Lang, k Kind, api, pattern string, needles ...string) Entry {
	return Entry{Lang: l, Kind: k, API: api, Pattern: regexp.MustCompile(pattern), Needles: needles}
}

func (e Entry) fn(n int) Entry   { e.FnArg = n; return e }
func (e Entry) ch(n int) Entry   { e.ChanArg = n; return e }
func (e Entry) call() Entry      { e.Call = true; return e }
func (e Entry) withArgs() Entry  { e.Args = true; return e }
func (e Entry) directive() Entry { e.Directive = true; return e }

// Catalog lists, per language, the calls that start an execution context
// and the calls that hand work between contexts. Order matters: where two
// entries match at one place, the first one reports it.
var Catalog = []Entry{
	entry(C, Spawn, "pthread_create", `\bpthread_create\s*\(`, "pthread_create").fn(3).call(),
	entry(C, Spawn, "thrd_create", `\bthrd_create\s*\(`, "thrd_create").fn(2).call(),
	entry(C, Spawn, "CreateThread", `\bCreateThread\s*\(`, "createthread").fn(3).call(),
	entry(C, Spawn, "_beginthreadex", `\b_beginthreadex\s*\(`, "_beginthreadex").fn(3).call(),
	entry(C, Spawn, "_beginthread", `\b_beginthread\s*\(`, "_beginthread").fn(1).call(),
	entry(C, Spawn, "std::thread", `\bstd::j?thread\s*(?:\w+\s*)?[({]`, "std::").fn(1),
	entry(C, Spawn, "std::async", `\bstd::async\s*\(`, "std::async"),
	entry(C, Spawn, "thread pool object", `\b\w*ThreadPool\s+\w+\s*[({]`, "threadpool"),
	entry(C, Spawn, "thread container", `\b\w*threads?\w*\s*(?:\.|->)\s*(?:emplace_back|push_back|emplace)\s*\(`, "thread"),
	entry(C, Spawn, "#pragma omp parallel", `#\s*pragma\s+omp\s+parallel\b`, "omp").directive(),
	entry(C, Spawn, "thread wrapper", `(?i)\b(?P<api>\w*thread_?(?:create|start|spawn)\w*)\s*\(`, "thread").call(),
	entry(C, Spawn, "pool wrapper", `(?i)\b(?P<api>\w*pool_?create(?:_\w+)?|\w*create_?thread_?pool\w*)\s*\(`, "pool").call(),
	entry(C, Send, "signal", `\b(?P<api>\w*cond_(?:signal|broadcast)|sem_post|ReleaseSemaphore|SetEvent|WakeConditionVariable|WakeAllConditionVariable)\s*\(`, "cond_", "sem_post", "releasesemaphore", "setevent", "conditionvariable").ch(1).call(),
	entry(C, Recv, "wait", `\b(?P<api>\w*cond_(?:timed)?wait|sem_(?:timed)?wait|WaitForSingleObject|SleepConditionVariable(?:CS|SRW))\s*\(`, "cond_", "sem_", "waitforsingleobject", "conditionvariable").ch(1).call(),
	entry(C, Send, "queue push", `(?i)\b(?P<api>\w*(?:queue|deque|ring|circlebuf|fifo|pool)\w*_(?:push\w*|enqueue|put|add|tryadd|submit))\s*\(`, "queue", "deque", "ring", "circlebuf", "fifo", "pool").ch(1).call(),
	entry(C, Recv, "queue pop", `(?i)\b(?P<api>\w*(?:queue|deque|ring|circlebuf|fifo)\w*_(?:pop\w*|dequeue|take|get))\s*\(`, "queue", "deque", "ring", "circlebuf", "fifo").ch(1).call(),

	entry(Rust, Spawn, "std::thread::spawn", `\b(?:std::)?thread::spawn\s*\(`, "thread::spawn").fn(1),
	entry(Rust, Spawn, "tokio::spawn", `\btokio::(?:task::)?spawn(?:_blocking|_local)?\s*\(`, "tokio::").fn(1),
	entry(Rust, Spawn, "rayon::spawn", `\brayon::spawn(?:_fifo)?\s*\(`, "rayon::").fn(1),
	entry(Rust, Spawn, "spawn", `(?:^|[^\w.:])(?P<at>spawn(?:_blocking|_local)?)\s*\(`, "spawn").fn(1).call().withArgs(),
	entry(Rust, Spawn, "spawn (method)", `(?P<at>\.\s*spawn(?:_blocking|_local)?)\s*\(`, "spawn").fn(1).withArgs(),
	entry(Rust, Spawn, "spawn wrapper", `(?:^|[^\w.])(?P<at>(?:\w+::)*(?P<api>spawn_\w+|\w+_spawn\w*))\s*\(`, "spawn").call(),
	entry(Rust, Spawn, "spawn wrapper (method)", `(?P<at>\.\s*(?P<api>spawn_\w+|\w+_spawn\w*))\s*\(`, "spawn"),
	entry(Rust, Spawn, "tokio Builder::new_*", `\b(?:runtime::)?Builder::new_(?:multi_thread|current_thread)\s*\(`, "builder::new_"),
	entry(Rust, Spawn, "tokio Runtime::new", `\b(?:runtime::)?Runtime::new\s*\(`, "runtime::new"),
	entry(Rust, Spawn, "#[tokio::main]", `#\[tokio::main\b`, "tokio::main"),
	entry(Rust, Spawn, "rayon ThreadPoolBuilder", `\bThreadPoolBuilder::new\s*\(`, "threadpoolbuilder"),
	entry(Rust, Spawn, "EventLoop::run", `\b(?:EventLoop::run\w*\s*\(|(?P<at>\w*event_loop\s*\.\s*run\w*)\s*\()`, "eventloop::run", "event_loop"),
	entry(Rust, Send, "send", `\b(?P<ch>[A-Za-z_]\w*)\s*\.\s*(?P<api>send|try_send|blocking_send|send_timeout|send_async|unbounded_send)\s*\(`, "send"),
	entry(Rust, Recv, "recv", `\b(?P<ch>[A-Za-z_]\w*)\s*\.\s*(?P<api>recv|try_recv|blocking_recv|recv_timeout|recv_async)\s*\(`, "recv"),

	entry(Go, Spawn, "go", `(?m)(?:^|[{;])[ \t]*(?P<at>go)\s+(?:func\b|(?P<fn>[\w.]+)\s*\()`, "go"),
}

// rustChannel finds a channel made with both its ends named, so a send on
// one end and a recv on the other read as one channel.
var rustChannel = regexp.MustCompile(`let\s*\(\s*(?:mut\s+)?(\w+)\s*,\s*(?:mut\s+)?(\w+)\s*\)\s*(?::[^=;]*)?=\s*[\w:]*?(?:channel|bounded|unbounded)\w*\s*(?:::<[^;]*?>)?\s*\(`)

// rustTest finds an item that compiles only for tests, such as one under
// cfg(test) or cfg(all(test, feature = "x")). One under cfg(not(test)) is
// skipped by the caller.
var rustTest = regexp.MustCompile(`#\[(?:cfg\([^\]]*\btest\b[^\]]*\)|test|bench|tokio::test[^\]]*)\]`)
