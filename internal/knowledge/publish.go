package knowledge

import (
	"time"

	"github.com/whyjp/abbd/internal/analysis"
	"github.com/whyjp/abbd/internal/graph"
	"github.com/whyjp/abbd/internal/html"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/review"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/workspace"
)

// Publish writes the md originals, then derives graph and HTML from them with one watermark.
// settle=true moves the sync watermark to this workspace snapshot; it is passed only when the
// knowledge is verified against it (completed turn, bootstrap, reconcile). A draft publish
// leaves the watermark behind so status keeps reporting DRAFT/STALE honestly.
func Publish(s *ssot.Snapshot, res *analysis.Result, ws workspace.Snapshot, settle bool, note string) (*graph.Graph, error) {
	if err := s.Save(); err != nil {
		return nil, err
	}
	hash := ssot.Hash(s.Root)
	g := graph.Build(s, res, ws.Revision, ws.Fingerprint)
	g.SSOTHash = hash
	g.BuiltAt = time.Now().UTC().Format(time.RFC3339)
	if err := graph.Save(s.Root, g); err != nil {
		return nil, err
	}
	fresh := model.Current
	if !settle {
		fresh = model.Draft
	}
	if err := html.Render(s, g, html.Meta{Revision: ws.Revision, Fingerprint: ws.Fingerprint, SSOTHash: hash, Freshness: fresh, Note: note}); err != nil {
		return nil, err
	}
	if err := review.Write(s, ws.Fingerprint); err != nil {
		return nil, err
	}
	if settle {
		s.State.Revision, s.State.Fingerprint, s.State.SSOTHash = ws.Revision, ws.Fingerprint, hash
		s.State.Files = ws.Files
		s.State.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return g, s.SaveState()
}
