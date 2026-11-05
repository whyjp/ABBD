package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// abbd parts proposes a directory's parts from its code: a subdirectory is a
// node of its own, test files are left out, and each part cites the lines
// that hold it together, so an extraction script can place the parts it
// prints and a reader can check them.
func TestPartsProposesADirectorysResponsibilities(t *testing.T) {
	p, _ := bare(t)
	p.code("app/sessions.py", "from .cookies import CookiePolicy\n\n\nclass Session:\n    policy = CookiePolicy()\n    a = 1\n    b = 2\n")
	p.code("app/cookies.py", "class CookiePolicy:\n    pass\n")
	p.code("app/client.py", "from .sessions import Session\nfrom .output import write\n\n\ndef send():\n    write(Session())\n")
	p.code("app/output/writer.py", "def write(m):\n    return m\n")
	p.code("app/output/streams.py", "class Stream:\n    pass\n")
	p.code("app/test_client.py", "from .client import send\n")
	out := p.want(0, `"parts"`, "parts", "app", "--json")
	var got partsOut
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parts --json prints JSON: %v\n%s", err, out)
	}
	var session *partOut
	for i, part := range got.Parts {
		for _, n := range part.Nodes {
			if strings.Contains(n, "test_") {
				t.Fatalf("a test file is no node: %s", out)
			}
		}
		if slices.Contains(part.Nodes, "app/sessions.py") {
			session = &got.Parts[i]
		}
	}
	if session == nil || !slices.Equal(session.Nodes, []string{"app/cookies.py", "app/sessions.py"}) || session.Name != "session" {
		t.Fatalf("sessions and the cookies only it uses are one part named by its type: %s", out)
	}
	if len(session.Evidence) == 0 || session.Evidence[0] != (evidenceLine{"app/sessions.py", "1", "from .cookies import CookiePolicy"}) {
		t.Fatalf("the part cites the line that ties its files: %s", out)
	}
	text := p.want(0, "app/output/", "parts", "app")
	if !strings.Contains(text, "session\n  app/cookies.py\n  app/sessions.py\n") {
		t.Fatalf("the text form lists each part with its nodes:\n%s", text)
	}
	p.want(1, "is no directory", "parts", "app/none")
	p.code("docs/readme.md", "# app\n")
	p.want(1, "holds no source files", "parts", "docs")
}
