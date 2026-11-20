package mailfilter

import (
	"context"
	"github.com/hkdb/aerion/internal/gmail"
	"testing"
)

type forwardingNativeProvider struct {
	*fakeNativeProvider
	addresses []gmail.ForwardingAddress
	reads     int
}

func (p *forwardingNativeProvider) ListForwardingAddresses(context.Context) ([]gmail.ForwardingAddress, error) {
	p.reads++
	return p.addresses, nil
}
func TestGmailForwardingPublicationRechecksAndNeverReplays(t *testing.T) {
	for _, mode := range []string{"verified", "revoked", "pending", "unknown outcome"} {
		t.Run(mode, func(t *testing.T) {
			s, r, f, base := publicationFixture(t)
			f.Action = gmail.FilterAction{Forward: "destination@example.test"}
			addresses := []gmail.ForwardingAddress{{Email: f.Action.Forward, VerificationStatus: "accepted"}}
			if _, err := s.PrepareGmailPublication(r.ID, "account1", f, nil, nil); err == nil {
				t.Fatal("old label-only entry accepted forwarding without catalog")
			}
			plan, err := s.PrepareGmailForwardingPublication(r.ID, "account1", f, nil, nil, addresses)
			if err != nil {
				t.Fatal(err)
			}
			p := &forwardingNativeProvider{fakeNativeProvider: base, addresses: addresses}
			if mode == "revoked" {
				p.addresses = nil
			}
			if mode == "pending" {
				p.addresses = []gmail.ForwardingAddress{{Email: f.Action.Forward, VerificationStatus: "pending"}}
			}
			if mode == "unknown outcome" {
				p.uncertain = true
				p.drop = true
			}
			got, err := s.PublishGmailFilter(context.Background(), plan.ID, p)
			if mode == "revoked" || mode == "pending" {
				if err == nil || p.writes != 0 {
					t.Fatalf("unsafe publication %v writes=%d", err, p.writes)
				}
				saved, e := s.GetGmailPublication(plan.ID)
				if e != nil || saved.State != "prepared" {
					t.Fatal(saved, e)
				}
				return
			}
			if p.writes != 1 || p.reads != 1 {
				t.Fatalf("writes=%d reads=%d", p.writes, p.reads)
			}
			if mode == "verified" && (err != nil || got.State != "published") {
				t.Fatal(got, err)
			}
			if _, err := s.PublishGmailFilter(context.Background(), plan.ID, p); err == nil || p.writes != 1 {
				t.Fatal("create replay", err, p.writes)
			}
			if mode == "unknown outcome" {
				p.filters = []gmail.Filter{{ID: "different", Criteria: f.Criteria, Action: gmail.FilterAction{Forward: "other@example.test"}}}
				saved, e := s.ReconcileGmailPublication(context.Background(), plan.ID, p)
				if e == nil && saved.State == "published" {
					t.Fatal("different destination accepted")
				}
				if p.writes != 1 {
					t.Fatal("recovery wrote")
				}
			}
		})
	}
}
