package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/webhook"
)

func postInventory(t *testing.T, env *accessEnv, mac string, fields map[string]string) {
	t.Helper()
	form := url.Values{"mac": {mac}}
	for k, v := range fields {
		form.Set(k, v)
	}
	req := httptest.NewRequest(http.MethodPost, "/inventory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "10.0.0.55:4011"
	rec := httptest.NewRecorder()
	env.server.handleInventoryReport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inventory report: want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestInventoryReportEnrolsNewClientOnce(t *testing.T) {
	env := newAccessEnv(t)
	var mu sync.Mutex
	var events []webhook.Event
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev webhook.Event
		json.NewDecoder(r.Body).Decode(&ev)
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}))
	defer hook.Close()
	if err := env.store.UpdateWebhookConfig(&models.WebhookConfig{ID: 1, URL: hook.URL, Enabled: true, OnAutoEnrol: true}); err != nil {
		t.Fatal(err)
	}
	env.server.webhookNotifier = webhook.New(env.store)

	group := &models.ClientGroup{Name: "lab", Enabled: true}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	profile := createProfile(t, env, &models.DeploymentProfile{Name: "lab-profile"})
	rules := []*models.EnrolmentRule{
		{Name: "servers", Priority: 10, Enabled: true, ManufacturerContains: "supermicro", MakeStatic: true},
		{Name: "lab-dell", Priority: 20, Enabled: true, ManufacturerContains: "dell", Firmware: "uefi", ClientGroupID: &group.ID, ProfileID: &profile.ID, MakeStatic: true, NamePattern: "lab-{{HOSTNAME_SAFE}}"},
	}
	for _, r := range rules {
		if err := env.store.CreateEnrolmentRule(r); err != nil {
			t.Fatal(err)
		}
	}

	mac := "aa:bb:cc:dd:ee:71"
	postInventory(t, env, mac, map[string]string{"manufacturer": "Dell Inc.", "product": "OptiPlex 7090", "serial": "LAB 42", "platform": "efi", "buildarch": "x86_64"})
	c, err := env.store.GetClient(mac)
	if err != nil {
		t.Fatal(err)
	}
	if c.EnrolledByRuleID == nil || *c.EnrolledByRuleID != rules[1].ID {
		t.Fatalf("client not enrolled by the Dell rule: %+v", c)
	}
	if c.ClientGroupID == nil || *c.ClientGroupID != group.ID || c.ProfileID == nil || *c.ProfileID != profile.ID || !c.Static || c.Name != "lab-lab-42" {
		t.Fatalf("enrolment applied the wrong settings: %+v", c)
	}
	rule, _ := env.store.GetEnrolmentRule(rules[1].ID)
	if rule.MatchCount != 1 {
		t.Errorf("match count = %d, want 1", rule.MatchCount)
	}

	c.Name = "renamed-by-human"
	c.ClientGroupID = nil
	if err := env.store.UpdateClient(mac, c); err != nil {
		t.Fatal(err)
	}
	postInventory(t, env, mac, map[string]string{"manufacturer": "Dell Inc.", "product": "OptiPlex 7090", "serial": "LAB 42", "platform": "efi", "buildarch": "x86_64"})
	again, _ := env.store.GetClient(mac)
	if again.Name != "renamed-by-human" || again.ClientGroupID != nil {
		t.Errorf("an enrolled client must not be re-enrolled on later reports: %+v", again)
	}
	rule, _ = env.store.GetEnrolmentRule(rules[1].ID)
	if rule.MatchCount != 1 {
		t.Errorf("match count changed on re-report: %d", rule.MatchCount)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range events {
		if ev.Event == webhook.EventClientAutoEnrolled && ev.MAC == mac && ev.Metadata["rule"] == "lab-dell" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a client.auto_enrolled webhook, got %+v", events)
	}
}

func TestInventoryReportLeavesUnmatchedAndHumanManagedClientsAlone(t *testing.T) {
	env := newAccessEnv(t)
	group := &models.ClientGroup{Name: "lab", Enabled: true}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateEnrolmentRule(&models.EnrolmentRule{Name: "dell", Priority: 10, Enabled: true, ManufacturerContains: "dell", ClientGroupID: &group.ID}); err != nil {
		t.Fatal(err)
	}

	postInventory(t, env, "aa:bb:cc:dd:ee:72", map[string]string{"manufacturer": "Lenovo", "product": "ThinkCentre"})
	if c, _ := env.store.GetClient("aa:bb:cc:dd:ee:72"); c.EnrolledByRuleID != nil || c.ClientGroupID != nil {
		t.Errorf("non-matching client must stay untouched: %+v", c)
	}

	other := &models.ClientGroup{Name: "finance", Enabled: true}
	if err := env.store.CreateClientGroup(other); err != nil {
		t.Fatal(err)
	}
	createClient(t, env, &models.Client{MACAddress: "aa:bb:cc:dd:ee:73", Enabled: true, ShowPublicImages: true, ClientGroupID: &other.ID})
	postInventory(t, env, "aa:bb:cc:dd:ee:73", map[string]string{"manufacturer": "Dell Inc."})
	if c, _ := env.store.GetClient("aa:bb:cc:dd:ee:73"); c.EnrolledByRuleID != nil || *c.ClientGroupID != other.ID {
		t.Errorf("a client placed in a group by a human must not be moved: %+v", c)
	}
}

func TestEnrolmentRulesAPITestAndApply(t *testing.T) {
	env := newAccessEnv(t)
	group := &models.ClientGroup{Name: "lab", Enabled: true}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	for i, m := range []string{"Dell Inc.", "Dell Inc.", "HP"} {
		mac := fmt.Sprintf("aa:bb:cc:dd:ee:8%d", i)
		postInventory(t, env, mac, map[string]string{"manufacturer": m, "serial": fmt.Sprintf("S%d", i)})
	}

	body := map[string]interface{}{"name": "dell", "priority": 10, "enabled": true, "manufacturer_contains": "dell", "client_group_id": group.ID, "name_pattern": "dell-{{SERIAL}}"}
	code, resp := env.doJSON(http.MethodPost, "/api/enrolment-rules/test", env.admin, body)
	if code != http.StatusOK {
		t.Fatalf("test: want 200, got %d %s", code, resp)
	}
	var test struct {
		Data struct {
			Total   int              `json:"total"`
			Matches []enrolmentMatch `json:"matches"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp), &test); err != nil {
		t.Fatal(err)
	}
	if test.Data.Total != 2 || test.Data.Matches[0].WouldName != "dell-S0" || !test.Data.Matches[0].Eligible {
		t.Fatalf("unexpected dry run: %+v", test.Data)
	}
	if c, _ := env.store.GetClient("aa:bb:cc:dd:ee:80"); c.ClientGroupID != nil {
		t.Fatalf("dry run must not apply anything")
	}

	if code, resp := env.doJSON(http.MethodPost, "/api/enrolment-rules", env.admin, map[string]interface{}{"name": "bad", "manufacturer_contains": "dell", "client_group_id": 999}); code != http.StatusBadRequest {
		t.Errorf("unknown group: want 400, got %d %s", code, resp)
	}
	if code, resp := env.doJSON(http.MethodPost, "/api/enrolment-rules", env.admin, map[string]interface{}{"name": "bad", "client_group_id": group.ID}); code != http.StatusBadRequest {
		t.Errorf("rule without conditions: want 400, got %d %s", code, resp)
	}
	code, resp = env.doJSON(http.MethodPost, "/api/enrolment-rules", env.admin, body)
	if code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d %s", code, resp)
	}
	if code, _ := env.doJSON(http.MethodPost, "/api/enrolment-rules", env.viewer, body); code != http.StatusForbidden {
		t.Errorf("viewer create: want 403, got %d", code)
	}
	if code, resp := env.do(http.MethodGet, "/api/enrolment-rules", env.viewer); code != http.StatusOK || !strings.Contains(resp, `"name":"dell"`) {
		t.Errorf("viewer list: want 200 with the rule, got %d %s", code, resp)
	}

	code, resp = env.doJSON(http.MethodPost, "/api/enrolment-rules/apply", env.admin, nil)
	if code != http.StatusOK {
		t.Fatalf("apply: want 200, got %d %s", code, resp)
	}
	var applied struct {
		Data struct {
			Enrolled   []map[string]string `json:"enrolled"`
			Considered int                 `json:"considered"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Data.Considered != 3 || len(applied.Data.Enrolled) != 2 {
		t.Fatalf("apply result: %+v", applied.Data)
	}
	for _, mac := range []string{"aa:bb:cc:dd:ee:80", "aa:bb:cc:dd:ee:81"} {
		c, _ := env.store.GetClient(mac)
		if c.ClientGroupID == nil || *c.ClientGroupID != group.ID || !strings.HasPrefix(c.Name, "dell-S") {
			t.Errorf("%s not enrolled by apply: %+v", mac, c)
		}
	}
	if c, _ := env.store.GetClient("aa:bb:cc:dd:ee:82"); c.ClientGroupID != nil {
		t.Errorf("HP client must not be enrolled: %+v", c)
	}
	code, resp = env.doJSON(http.MethodPost, "/api/enrolment-rules/apply", env.admin, nil)
	if code != http.StatusOK || !strings.Contains(resp, `"considered":1`) {
		t.Errorf("second apply should only consider the remaining client: %d %s", code, resp)
	}
}
