package settings

import "testing"

func TestSmartNotificationsPersistAndApplySpecificRules(t *testing.T) {
	s := NewStore(openTestDB(t))
	p, err := s.GetSmartNotificationSettings()
	if err != nil || !p.Enabled || p.Rules == nil {
		t.Fatalf("defaults: %#v, %v", p, err)
	}
	p.Rules = []NotificationRule{{Scope: "domain", Match: " @Example.COM ", Action: "mute"}, {Scope: "sender", Match: " VIP@Example.COM ", Action: "notify"}}
	if err := s.SaveSmartNotificationSettings(p); err != nil {
		t.Fatal(err)
	}
	p, err = s.GetSmartNotificationSettings()
	if err != nil {
		t.Fatal(err)
	}
	if p.Rules[0].Match != "example.com" || p.Rules[0].ID == "" {
		t.Fatalf("not normalized/persisted: %#v", p)
	}
	for _, tc := range []struct {
		sender string
		want   bool
	}{{"Other <other@example.com>", false}, {"VIP <vip@EXAMPLE.com>", true}, {"other@elsewhere.fi", true}, {"invalid sender", true}} {
		if got := p.ShouldNotify(tc.sender); got != tc.want {
			t.Errorf("%q: got %v want %v", tc.sender, got, tc.want)
		}
	}
	p.Enabled = false
	if p.ShouldNotify("vip@example.com") {
		t.Fatal("disabled alerts must override notify rule")
	}
}

func TestSmartNotificationsRejectInvalidRulesWithoutReplacingSavedState(t *testing.T) {
	s := NewStore(openTestDB(t))
	good := SmartNotificationSettings{Enabled: false, Rules: []NotificationRule{{Scope: "sender", Match: "a@example.com", Action: "mute"}}}
	if err := s.SaveSmartNotificationSettings(good); err != nil {
		t.Fatal(err)
	}
	cases := [][]NotificationRule{
		{{Scope: "all", Match: "example.com", Action: "mute"}},
		{{Scope: "sender", Match: "Display <a@example.com>", Action: "mute"}},
		{{Scope: "domain", Match: "a@example.com", Action: "mute"}},
		{{Scope: "domain", Match: "", Action: "mute"}},
		{{Scope: "sender", Match: "a@example.com", Action: "other"}},
		{{Scope: "domain", Match: "example.com", Action: "mute"}, {Scope: "domain", Match: "@EXAMPLE.COM", Action: "notify"}},
	}
	for _, rules := range cases {
		if err := s.SaveSmartNotificationSettings(SmartNotificationSettings{Enabled: true, Rules: rules}); err == nil {
			t.Errorf("accepted invalid rules: %#v", rules)
		}
		got, err := s.GetSmartNotificationSettings()
		if err != nil || got.Enabled || len(got.Rules) != 1 || got.Rules[0].Match != "a@example.com" {
			t.Fatalf("invalid save replaced existing preferences: %#v %v", got, err)
		}
	}
}

func TestSmartNotificationsCorruptValueAllowsExistingAlerts(t *testing.T) {
	s := NewStore(openTestDB(t))
	if err := s.Set(KeySmartNotifications, "not json"); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetSmartNotificationSettings()
	if err != nil || !p.Enabled || !p.ShouldNotify("a@example.com") {
		t.Fatalf("corrupt settings silenced mail: %#v %v", p, err)
	}
}

func TestNotificationBatchNeverSuppressesUnknownOrMixedArrivals(t *testing.T) {
	prefs := SmartNotificationSettings{Enabled: true, Rules: []NotificationRule{{Scope: "domain", Match: "muted.example", Action: "mute"}}}
	cases := []struct {
		senders  []string
		expected int
		want     bool
	}{
		{[]string{"one@muted.example"}, 1, false},
		{[]string{"one@muted.example", "important@elsewhere.fi"}, 2, true},
		{[]string{"one@muted.example"}, 2, true},
		{nil, 1, true},
		{[]string{"one@muted.example", "two@muted.example"}, 1, true},
		{[]string{"one@muted.example"}, 101, true},
		{[]string{"not an email"}, 1, true},
	}
	for _, tc := range cases {
		if got := prefs.ShouldNotifyBatch(tc.senders, tc.expected); got != tc.want {
			t.Errorf("%#v expected count %d: got %v want %v", tc.senders, tc.expected, got, tc.want)
		}
	}
}
