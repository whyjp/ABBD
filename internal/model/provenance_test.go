package model

import "testing"

func TestWhyFieldsRejectExtractedAndObserved(t *testing.T) {
	for _, o := range []Origin{Extracted, Observed, UnknownOrigin} {
		if _, err := NewSemantic("x", o); err == nil {
			t.Errorf("a why-field accepted origin %s", o)
		}
	}
	for _, o := range []Origin{UserStated, Adopted, Inferred} {
		if _, err := NewSemantic("x", o); err != nil {
			t.Errorf("a why-field rejected origin %s: %v", o, err)
		}
	}
}

func TestBadgeIsDerivedFromAxes(t *testing.T) {
	cases := []struct {
		s    Semantic
		want Badge
	}{
		{MustSemantic("x", UserStated), BadgeConfirmed},
		{MustSemantic("x", Inferred), BadgeInferred},
		{Semantic{Value: "x", Origin: Inferred, Confirmation: Confirmed}, BadgeConfirmed},
		{Semantic{Value: "x", Origin: Inferred, Confirmation: Rejected}, BadgeRejected},
		{MustSemantic("x", Adopted), BadgeAdopted},
	}
	for _, c := range cases {
		if got := c.s.Badge(); got != c.want {
			t.Errorf("%+v badge %s, want %s", c.s, got, c.want)
		}
	}
	if !MustSemantic("x", Inferred).Pending() || MustSemantic("x", UserStated).Pending() {
		t.Error("only unconfirmed non-user meanings are pending")
	}
}

func TestNextIDIgnoresOtherKinds(t *testing.T) {
	if got := NextID(KindFeature, []string{"FEAT-002", "MOD-009", "FEAT-010"}); got != "FEAT-011" {
		t.Fatalf("NextID = %s", got)
	}
}
