package model

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Entry is one part of a contract as the code spells it: a route, an RPC
// method, a command's flag, a field of a file. Name is the code's own
// spelling and Key the same name reduced to the letters a note's name
// takes, so a reader's note on the entry is ID.entries.KEY. Shape is what
// the code says the entry looks like: a handler's signature, a flag's
// type, a field's type. Required is nil when the code does not say.
type Entry struct {
	Key      string `yaml:"key" json:"key,omitempty"`
	Name     string `yaml:"name" json:"name"`
	Shape    string `yaml:"shape,omitempty" json:"shape,omitempty"`
	Required *bool  `yaml:"required,omitempty" json:"required,omitempty"`
}

var notKey = regexp.MustCompile(`[^a-z0-9]+`)

// EntryKey reduces an entry's name to a note key: "GET /users/{id}" is
// get-users-id, "--check" is check and "high_water" is high-water.
func EntryKey(name string) string {
	return strings.Trim(notKey.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// Need says whether the entry is required, optional, or "" when the code does
// not say.
func (e Entry) Need() string {
	switch {
	case e.Required == nil:
		return ""
	case *e.Required:
		return "required"
	}
	return "optional"
}

// Look is the entry's shape and need as one diff line shows them.
func (e Entry) Look() string {
	return strings.TrimSpace(e.Shape + " " + e.Need())
}

// EntryNamed finds an entry of the unit by key.
func (u Unit) EntryNamed(key string) (Entry, bool) {
	i := slices.IndexFunc(u.Entries, func(e Entry) bool { return e.Key == key })
	if i < 0 {
		return Entry{}, false
	}
	return u.Entries[i], true
}

// ValidateEntries checks that each entry has a name and a key of its own.
func ValidateEntries(es []Entry) error {
	seen := map[string]string{}
	for _, e := range es {
		switch {
		case strings.TrimSpace(e.Name) == "":
			return fmt.Errorf("an entry needs a name")
		case !noteKey.MatchString(e.Key):
			return fmt.Errorf("entry %q: key %q is not lower-case letters, digits and hyphens", e.Name, e.Key)
		case seen[e.Key] != "":
			return fmt.Errorf("entries %q and %q share the key %q; give one a key of its own", seen[e.Key], e.Name, e.Key)
		}
		seen[e.Key] = e.Name
	}
	return nil
}
