package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// The guard reads a shell command the way a shell would split it, closely
// enough to see an abbd human command or a write to a protected path. It is
// defense in depth, not a boundary: a script file, a variable that names the
// command, or any tool the hooks do not see gets past it.

// segment is one simple command: its words with quotes removed, and the files
// its output redirections write.
type segment struct {
	words     []string
	redirects []string
	pipeline  int
	// literal is a here-string's body when the segment is only that value.
	literal *body
}

// body is text a command reads on stdin: a heredoc, a here-string, or a bash
// <<< word. quoted bodies are not expanded, so $(...) in them does not run.
type body struct {
	text   string
	quoted bool
	seg    int
}

type parsed struct {
	segs   []segment
	bodies []body
	// subs are command substitutions, which run wherever they appear.
	subs []string
}

type shellParser struct {
	s   string
	i   int
	out parsed
	cur segment
	w   strings.Builder
	// inWord is true once the current word has any character, even an empty quote.
	inWord   bool
	pipeline int
	pending  []body
	// next says what the word being read is for: a redirect target, stdin, or a <<< body.
	next byte
}

func parseShell(s string) parsed {
	p := &shellParser{s: s}
	p.run()
	return p.out
}

func (p *shellParser) peek(k int) byte {
	if p.i+k < len(p.s) {
		return p.s[p.i+k]
	}
	return 0
}

func (p *shellParser) endWord() {
	if !p.inWord {
		return
	}
	w := p.w.String()
	p.w.Reset()
	p.inWord = false
	switch p.next {
	case '>':
		if !strings.HasPrefix(w, "&") {
			p.cur.redirects = append(p.cur.redirects, w)
		}
	case '<':
	case 'h':
		p.out.bodies = append(p.out.bodies, body{text: w, quoted: true, seg: len(p.out.segs)})
	default:
		p.cur.words = append(p.cur.words, w)
	}
	p.next = 0
}

func (p *shellParser) endSegment(pipe bool) {
	p.endWord()
	if len(p.cur.words) > 0 || len(p.cur.redirects) > 0 || p.cur.literal != nil {
		p.cur.pipeline = p.pipeline
		p.out.segs = append(p.out.segs, p.cur)
	}
	p.cur = segment{}
	if !pipe {
		p.pipeline++
	}
}

// balanced reads from just past an opening $( to its closing paren.
func (p *shellParser) balanced() string {
	depth, start := 1, p.i
	for ; p.i < len(p.s); p.i++ {
		switch p.s[p.i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				sub := p.s[start:p.i]
				p.i++
				return sub
			}
		}
	}
	return p.s[start:]
}

func (p *shellParser) until(c byte) string {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] != c {
		p.i++
	}
	text := p.s[start:p.i]
	if p.i < len(p.s) {
		p.i++
	}
	return text
}

// line reads the rest of the current line, without its newline.
func (p *shellParser) line() (string, bool) {
	if p.i >= len(p.s) {
		return "", false
	}
	l := p.until('\n')
	return strings.TrimSuffix(l, "\r"), true
}

func (p *shellParser) heredocBodies() {
	for _, h := range p.pending {
		delim := h.text
		var lines []string
		for {
			l, ok := p.line()
			if !ok || strings.TrimSpace(l) == delim {
				break
			}
			lines = append(lines, l)
		}
		p.out.bodies = append(p.out.bodies, body{text: strings.Join(lines, "\n"), quoted: h.quoted, seg: h.seg})
	}
	p.pending = nil
}

func (p *shellParser) run() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '\'':
			p.i++
			p.w.WriteString(p.until('\''))
			p.inWord = true
		case c == '"':
			p.i++
			text := p.until('"')
			p.out.subs = append(p.out.subs, substitutions(text)...)
			p.w.WriteString(text)
			p.inWord = true
		case c == '`':
			p.i++
			p.out.subs = append(p.out.subs, p.until('`'))
			p.inWord = true
		case (c == '$' || c == '<' || c == '>') && p.peek(1) == '(':
			p.i += 2
			p.out.subs = append(p.out.subs, p.balanced())
			p.inWord = true
		case c == '@' && !p.inWord && (p.peek(1) == '\'' || p.peek(1) == '"') && (p.peek(2) == '\n' || p.peek(2) == '\r'):
			p.hereString(p.peek(1))
		case c == '#' && !p.inWord:
			p.until('\n')
			p.newline()
		case c == '\n':
			p.i++
			p.newline()
		case c == ' ' || c == '\t' || c == '\r':
			p.i++
			p.endWord()
		case c == '|' && p.peek(1) == '|', c == '&' && p.peek(1) == '&':
			p.i += 2
			p.endSegment(false)
		case c == '|':
			p.i++
			if p.peek(0) == '&' {
				p.i++
			}
			p.endSegment(true)
		case c == '&' && p.peek(1) == '>':
			p.i++
		case c == ';' || c == '&' || c == '(' || c == ')',
			(c == '{' || c == '}') && !p.inWord:
			p.i++
			p.endSegment(false)
		case c == '<' && p.peek(1) == '<' && p.peek(2) == '<':
			p.i += 3
			p.endWord()
			p.next = 'h'
		case c == '<' && p.peek(1) == '<':
			p.i += 2
			p.heredoc()
		case c == '<':
			p.i++
			p.endWord()
			p.next = '<'
		case c == '>':
			p.redirect()
		default:
			p.w.WriteByte(c)
			p.inWord = true
			p.i++
		}
	}
	p.endSegment(false)
	p.heredocBodies()
}

func (p *shellParser) newline() {
	p.endSegment(false)
	p.heredocBodies()
}

// redirect reads >, >>, >|, 2> and PowerShell's *> . A leading fd number
// belongs to the operator, and a target of &1 duplicates a stream.
func (p *shellParser) redirect() {
	if w := p.w.String(); p.inWord && (w == "*" || strings.Trim(w, "0123456789") == "") {
		p.w.Reset()
		p.inWord = false
	}
	p.endWord()
	p.i++
	for p.peek(0) == '>' || p.peek(0) == '|' {
		p.i++
	}
	if p.peek(0) == '&' {
		p.i++
		p.until(' ')
		return
	}
	p.next = '>'
}

func (p *shellParser) heredoc() {
	if p.peek(0) == '-' {
		p.i++
	}
	for p.peek(0) == ' ' || p.peek(0) == '\t' {
		p.i++
	}
	start := p.i
	for p.i < len(p.s) && !strings.ContainsRune(" \t\n;&|<>()", rune(p.s[p.i])) {
		p.i++
	}
	raw := p.s[start:p.i]
	delim := strings.NewReplacer(`'`, "", `"`, "", `\`, "").Replace(raw)
	p.pending = append(p.pending, body{text: delim, quoted: delim != raw, seg: len(p.out.segs)})
}

// hereString reads PowerShell's @'...'@ or @"..."@, whose closing mark starts a line.
func (p *shellParser) hereString(q byte) {
	p.i += 2
	p.line()
	var lines []string
	for {
		start := p.i
		l, ok := p.line()
		if !ok {
			break
		}
		if strings.HasPrefix(l, string(q)+"@") {
			p.i = start + 2
			break
		}
		lines = append(lines, l)
	}
	p.endWord()
	p.cur.literal = &body{text: strings.Join(lines, "\n"), quoted: q == '\''}
}

// substitutions finds the $(...) and backtick commands inside expanded text.
func substitutions(text string) []string {
	p := &shellParser{s: text}
	var out []string
	for p.i < len(p.s) {
		switch {
		case p.s[p.i] == '$' && p.peek(1) == '(':
			p.i += 2
			out = append(out, p.balanced())
		case p.s[p.i] == '`':
			p.i++
			out = append(out, p.until('`'))
		default:
			p.i++
		}
	}
	return out
}

// commandName is a word as a command name: lower case, without a directory,
// an .exe, or the backslashes a shell escape left.
func commandName(w string) string {
	w = strings.ToLower(strings.ReplaceAll(w, `\`, "/"))
	w = path.Base(strings.TrimRight(w, "/"))
	return strings.TrimSuffix(w, ".exe")
}

// readers only read their arguments and stdin, so a protected name in them is
// text, not a target.
var readers = map[string]bool{
	"cat": true, "type": true, "more": true, "less": true, "head": true, "tail": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true, "findstr": true,
	"wc": true, "sort": true, "uniq": true, "diff": true, "cmp": true, "ls": true, "dir": true,
	"tree": true, "stat": true, "file": true, "sha256sum": true, "sha1sum": true, "md5sum": true,
	"echo": true, "printf": true, "true": true, "false": true, "test": true, "[": true,
	"which": true, "where": true, "jq": true, "cut": true, "tr": true, "column": true,
	"get-content": true, "gc": true, "select-string": true, "sls": true, "get-childitem": true,
	"gci": true, "test-path": true, "get-item": true, "resolve-path": true, "measure-object": true,
	"select-object": true, "where-object": true, "write-output": true, "write-host": true,
	"out-string": true, "format-list": true, "format-table": true, "gh": true,
}

// shells run their arguments, or stdin, as commands.
var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true,
	"pwsh": true, "powershell": true, "cmd": true, "eval": true, "iex": true,
	"invoke-expression": true, "xargs": true, "env": true, "sudo": true, "nohup": true,
	"time": true, "exec": true, "command": true, "start-process": true,
}

var deleters = map[string]bool{
	"rm": true, "rmdir": true, "rd": true, "del": true, "erase": true, "unlink": true,
	"remove-item": true, "ri": true,
}

var cds = map[string]bool{"cd": true, "pushd": true, "set-location": true, "sl": true, "chdir": true}

// gitWriters rewrite working-tree files, so their pathspecs are targets.
var gitWriters = map[string]bool{"checkout": true, "restore": true, "rm": true, "mv": true, "apply": true, "clean": true, "stash": true, "reset": true}

// humans are the abbd commands only a human runs while the mode is on.
var humans = map[string]bool{
	"align": true, "approve": true, "reject": true, "confirm": true, "couple": true, "setup": true, "mode off": true,
}

// abbdCommand returns the abbd subcommand a segment runs, or "" when it runs
// none. A reader's arguments are text, but any other command may run abbd
// through an argument, as go run ./cmd/abbd does.
func abbdCommand(words []string) string {
	if readers[commandName(words[0])] {
		return ""
	}
	for i := range words {
		if commandName(words[i]) != "abbd" {
			continue
		}
		rest := words[i+1:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			if rest[0] == "--root" || rest[0] == "--session" {
				rest = rest[1:]
			}
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return ""
		}
		sub := strings.ToLower(strings.ReplaceAll(rest[0], `\`, ""))
		if sub == "mode" && len(rest) > 1 {
			sub += " " + strings.ToLower(strings.ReplaceAll(rest[1], `\`, ""))
		}
		return sub
	}
	return ""
}

// guard holds what a tool call may not reach. cwd is where the command starts.
type guard struct {
	root, cwd string
	// setupOpen is the greenfield window before the first extraction script is registered.
	setupOpen bool
	// scripts are the registered script's files, absolute and lower case.
	scripts []string
}

func (e env) guard(cwd string) guard {
	g := guard{root: absPath(e.root), cwd: absPath(e.root)}
	if cwd != "" {
		g.cwd = absPath(cwd)
	}
	p, err := e.st.LoadProject()
	if err != nil {
		return g
	}
	g.setupOpen = len(p.Extract) == 0
	for _, f := range scriptFiles(e.root, p.Extract) {
		g.scripts = append(g.scripts, strings.ToLower(f))
	}
	return g
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// scriptFiles are the argv words of a registered script that name a file in
// the project, as absolute paths.
func scriptFiles(root string, argv []string) []string {
	var out []string
	r := absPath(root)
	for _, a := range argv {
		f := a
		if !filepath.IsAbs(f) {
			f = filepath.Join(r, f)
		}
		if rel, err := filepath.Rel(r, f); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() {
			out = append(out, f)
		}
	}
	return out
}

func (g guard) resolve(cwd, p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if !filepath.IsAbs(p) && !strings.HasPrefix(p, "/") {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(filepath.FromSlash(p))
}

func components(p string) []string {
	return strings.Split(strings.ToLower(filepath.ToSlash(p)), "/")
}

func matches(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return pattern == name || (err == nil && ok)
}

// protected says whether writing p would change what only the CLI writes:
// anything under .abbd/ (but the extractor while setup is open), session
// state under the git directory, and the registered script's files. A
// deletion also reaches whatever holds them.
func (g guard) protected(cwd, p string, deleting bool) bool {
	abs := g.resolve(cwd, p)
	cs := components(abs)
	for i, c := range cs {
		if c == ".abbd" && !g.extractor(cs[i+1:]) {
			return true
		}
		if c == ".git" && (deleting || slices.Contains(cs[i+1:], "abbd")) {
			return true
		}
	}
	if slices.Contains(g.scripts, strings.ToLower(abs)) {
		return true
	}
	if rel, err := filepath.Rel(g.root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		// A glob is expanded by the shell, so .ab* or * at the root reaches .abbd/.
		rs := components(rel)
		if strings.ContainsAny(rs[0], "*?[") && ((matches(rs[0], ".abbd") && !g.extractor(rs[1:])) || (deleting && matches(rs[0], ".git"))) {
			return true
		}
	}
	if rel, err := filepath.Rel(abs, g.root); deleting && err == nil && !strings.HasPrefix(rel, "..") {
		return true
	}
	return false
}

// extractor says whether the path below a .abbd directory is the extractor
// an agent may still write while setup is open.
func (g guard) extractor(below []string) bool {
	return g.setupOpen && len(below) > 0 && below[0] == "extractor"
}

// mentions catches a protected name inside an argument that is not a plain
// path, such as a script passed to python -c.
func (g guard) mentions(arg string) bool {
	l := strings.ToLower(strings.ReplaceAll(arg, `\`, "/"))
	for i := strings.Index(l, ".abbd"); i >= 0; i = next(l, ".abbd", i) {
		end := i + len(".abbd")
		if end < len(l) && (isWordByte(l[end]) || l[end] == '.' || l[end] == '-') {
			continue
		}
		if !(g.setupOpen && strings.HasPrefix(l[end:], "/extractor")) {
			return true
		}
	}
	return strings.Contains(l, ".git/abbd")
}

func next(s, sub string, i int) int {
	if j := strings.Index(s[i+1:], sub); j >= 0 {
		return i + 1 + j
	}
	return -1
}

func isWordByte(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z'
}

// toolPath is the guard for the Write, Edit and NotebookEdit tools.
func (g guard) toolPath(file string) string {
	if file == "" || !g.protected(g.cwd, file, false) {
		return ""
	}
	return fmt.Sprintf("%s is written only by the abbd CLI or a human. Approvals, alignment, the turn record, session state and the registered extraction script are not an agent's to change.", relTo(g.root, file))
}

const maxDepth = 4

// command returns why cmd is denied, or "".
func (g guard) command(cmd string) string {
	return g.check(cmd, g.cwd, 0)
}

func (g guard) check(cmd, cwd string, depth int) string {
	if depth > maxDepth {
		return "this command nests shells too deeply to check"
	}
	p := parseShell(cmd)
	inner := func(text string) string { return g.check(text, cwd, depth+1) }
	for _, s := range p.subs {
		if r := inner(s); r != "" {
			return r
		}
	}
	for _, b := range p.bodies {
		if r := g.checkBody(p, b, inner); r != "" {
			return r
		}
	}
	for i, s := range p.segs {
		if s.literal != nil {
			b := *s.literal
			b.seg = i
			if r := g.checkBody(p, b, inner); r != "" {
				return r
			}
		}
		if len(s.words) == 0 {
			continue
		}
		if sub := abbdCommand(s.words); humans[sub] {
			if !(sub == "setup" && g.setupOpen && depth == 0 && registersOnly(p)) {
				return fmt.Sprintf("abbd %s is a human's command while ABBD mode is on. Stop and give the human the exact command to run.", sub)
			}
		}
		for _, r := range s.redirects {
			if g.protected(cwd, r, false) {
				return protectedReason(r)
			}
		}
		name := commandName(s.words[0])
		args := s.words[1:]
		switch {
		case cds[name]:
			if len(args) > 0 && !strings.ContainsAny(args[len(args)-1], "$~") {
				cwd = g.resolve(cwd, args[len(args)-1])
			}
		case name == "abbd" || readers[name]:
		case shells[name]:
			for len(args) > 0 && (strings.HasPrefix(args[0], "-") || slices.Contains([]string{"/c", "/k", "/s", "/q"}, strings.ToLower(args[0]))) {
				args = args[1:]
			}
			if r := inner(strings.Join(args, " ")); r != "" {
				return r
			}
		case name == "git":
			if r := g.checkGit(cwd, args); r != "" {
				return r
			}
		default:
			for _, a := range args {
				if g.mentions(a) {
					return protectedReason(a)
				}
				for _, part := range strings.Split(a, "=") {
					if part != "" && !strings.HasPrefix(part, "-") && g.protected(cwd, part, deleters[name]) {
						return protectedReason(part)
					}
				}
			}
		}
	}
	return ""
}

// checkBody reads a heredoc or here-string body as data when every command it
// flows into only reads, and as commands otherwise.
func (g guard) checkBody(p parsed, b body, inner func(string) string) string {
	if b.seg >= len(p.segs) {
		return inner(b.text)
	}
	data := true
	for j := b.seg; j < len(p.segs) && p.segs[j].pipeline == p.segs[b.seg].pipeline; j++ {
		s := p.segs[j]
		if len(s.words) == 0 {
			continue
		}
		name := commandName(s.words[0])
		if !(readers[name] || name == "git" || (name == "abbd" && !humans[abbdCommand(s.words)])) {
			data = false
		}
	}
	if !data {
		return inner(b.text)
	}
	if !b.quoted {
		for _, s := range substitutions(b.text) {
			if r := inner(s); r != "" {
				return r
			}
		}
	}
	return ""
}

// checkGit denies the git commands that rewrite protected working-tree files.
// A clean, stash or hard reset with no pathspec reaches the whole tree.
func (g guard) checkGit(cwd string, args []string) string {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if args[i] == "-C" || args[i] == "-c" {
			i++
		}
		i++
	}
	if i >= len(args) || !gitWriters[strings.ToLower(args[i])] {
		return ""
	}
	sub, rest := strings.ToLower(args[i]), args[i+1:]
	var paths []string
	for _, a := range rest {
		if !strings.HasPrefix(a, "-") {
			paths = append(paths, a)
		}
	}
	switch {
	case sub == "stash" && len(paths) > 0 && (paths[0] == "list" || paths[0] == "show"):
		return ""
	case sub == "reset" && !slices.Contains(rest, "--hard"):
		return ""
	case len(paths) == 0 && (sub == "clean" || sub == "stash" || sub == "reset"):
		return fmt.Sprintf("git %s rewrites the whole working tree, .abbd/ included. Name the paths it should touch.", sub)
	}
	for _, p := range paths {
		if g.protected(cwd, p, true) {
			return protectedReason(p)
		}
	}
	return ""
}

func protectedReason(p string) string {
	return fmt.Sprintf("this command writes or deletes %s, which only the abbd CLI or a human changes (.abbd/, session state in the git directory, the registered extraction script). Read .abbd/ files with the Read tool, or use abbd turn show and abbd status.", p)
}

// registersOnly is the one setup an agent runs in the mode, a greenfield
// project registering its first script, and only as a command that does
// nothing else.
func registersOnly(p parsed) bool {
	if len(p.segs) != 1 || len(p.subs) > 0 || len(p.bodies) > 0 {
		return false
	}
	s := p.segs[0]
	w := s.words
	return len(s.redirects) == 0 && s.literal == nil && len(w) > 3 &&
		commandName(w[0]) == "abbd" && w[1] == "setup" && w[2] == "--"
}
