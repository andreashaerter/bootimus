package storage

import (
	"testing"
	"time"

	"bootimus/internal/models"
)

func TestEnrolmentRuleRoundTripAndApply(t *testing.T) {
	store := newTestStore(t)
	group := &models.ClientGroup{Name: "lab", Enabled: true}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	profile := &models.DeploymentProfile{Name: "lab-profile"}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		t.Fatal(err)
	}
	second := &models.EnrolmentRule{Name: "second", Priority: 20, Enabled: true, ProductContains: "optiplex"}
	first := &models.EnrolmentRule{Name: "first", Priority: 10, Enabled: true, MACPrefix: "aa:bb", ClientGroupID: &group.ID, ProfileID: &profile.ID, MakeStatic: true, NamePattern: "pc-{{MAC_SUFFIX}}"}
	for _, r := range []*models.EnrolmentRule{second, first} {
		if err := store.CreateEnrolmentRule(r); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := store.ListEnrolmentRules()
	if err != nil || len(rules) != 2 || rules[0].Name != "first" {
		t.Fatalf("rules should list by priority: %+v err %v", rules, err)
	}

	for _, c := range []*models.Client{
		{MACAddress: "aa:bb:cc:00:00:01", Enabled: true, ShowPublicImages: true},
		{MACAddress: "aa:bb:cc:00:00:02", Enabled: true, ShowPublicImages: true, Static: true},
		{MACAddress: "aa:bb:cc:00:00:03", Enabled: true, ShowPublicImages: true, ClientGroupID: &group.ID},
	} {
		if err := store.CreateClient(c); err != nil {
			t.Fatal(err)
		}
	}
	unenrolled, err := store.ListUnenrolledClients()
	if err != nil || len(unenrolled) != 1 || unenrolled[0].MACAddress != "aa:bb:cc:00:00:01" {
		t.Fatalf("unenrolled = %+v err %v", unenrolled, err)
	}

	at := time.Now()
	if err := store.ApplyEnrolment("aa:bb:cc:00:00:01", EnrolmentUpdate{RuleID: first.ID, ClientGroupID: &group.ID, ProfileID: &profile.ID, MakeStatic: true, Name: "pc-000001", At: at}); err != nil {
		t.Fatal(err)
	}
	c, _ := store.GetClient("aa:bb:cc:00:00:01")
	if c.EnrolledByRuleID == nil || *c.EnrolledByRuleID != first.ID || c.ClientGroupID == nil || *c.ClientGroupID != group.ID || c.ProfileID == nil || *c.ProfileID != profile.ID || !c.Static || c.Name != "pc-000001" || c.EnrolledAt == nil {
		t.Fatalf("enrolment not applied: %+v", c)
	}
	r, _ := store.GetEnrolmentRule(first.ID)
	if r.MatchCount != 1 || r.LastMatchedAt == nil {
		t.Fatalf("match count not recorded: %+v", r)
	}
	if left, _ := store.ListUnenrolledClients(); len(left) != 0 {
		t.Errorf("client should no longer be unenrolled: %+v", left)
	}

	r.Enabled = false
	r.Priority = 5
	if err := store.UpdateEnrolmentRule(first.ID, r); err != nil {
		t.Fatal(err)
	}
	updated, _ := store.GetEnrolmentRule(first.ID)
	if updated.Enabled || updated.Priority != 5 || updated.MatchCount != 1 {
		t.Fatalf("update wrong: %+v", updated)
	}

	if err := store.DeleteEnrolmentRule(first.ID); err != nil {
		t.Fatal(err)
	}
	c, _ = store.GetClient("aa:bb:cc:00:00:01")
	if c.EnrolledByRuleID != nil {
		t.Errorf("deleting a rule should clear the client's reference to it")
	}
	if c.ClientGroupID == nil {
		t.Errorf("deleting a rule must not undo the enrolment itself")
	}
}
