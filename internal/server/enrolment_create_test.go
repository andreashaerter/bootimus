package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"bootimus/internal/models"
)

func TestCreateEnrolmentRulePreservesExplicitValues(t *testing.T) {
	cases := []struct {
		name     string
		fields   map[string]interface{}
		enabled  bool
		priority int
	}{
		{"defaults", map[string]interface{}{}, true, 100},
		{"disabled", map[string]interface{}{"enabled": false}, false, 100},
		{"zero priority", map[string]interface{}{"priority": 0}, true, 0},
		{"false and zero", map[string]interface{}{"enabled": false, "priority": 0}, false, 0},
		{"nonzero priority", map[string]interface{}{"enabled": true, "priority": 7}, true, 7},
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			env := newAccessEnv(t)
			testcase.fields["name"] = "new-rule"
			testcase.fields["mac_prefix"] = "aa:bb"
			testcase.fields["name_pattern"] = "node-{{MAC_SUFFIX}}"
			code, body := env.doJSON(http.MethodPost, "/api/enrolment-rules", env.admin, testcase.fields)
			if code != http.StatusCreated {
				t.Fatalf("create: %d %s", code, body)
			}
			var response struct {
				Data models.EnrolmentRule `json:"data"`
			}
			if err := json.Unmarshal([]byte(body), &response); err != nil {
				t.Fatal(err)
			}
			stored, err := env.store.GetEnrolmentRule(response.Data.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, rule := range []*models.EnrolmentRule{&response.Data, stored} {
				if rule.Enabled != testcase.enabled || rule.Priority != testcase.priority {
					t.Errorf("enabled=%v priority=%d; want %v and %d", rule.Enabled, rule.Priority, testcase.enabled, testcase.priority)
				}
			}
			if !testcase.enabled {
				client := &models.Client{MACAddress: "aa:bb:cc:dd:ee:88", Enabled: true}
				if err := env.store.CreateClient(client); err != nil {
					t.Fatal(err)
				}
				if code, body := env.doJSON(http.MethodPost, "/api/enrolment-rules/apply", env.admin, nil); code != http.StatusOK {
					t.Fatalf("apply: %d %s", code, body)
				}
				client, err = env.store.GetClient(client.MACAddress)
				if err != nil {
					t.Fatal(err)
				}
				if client.EnrolledByRuleID != nil {
					t.Error("disabled rule enrolled a client")
				}
			}
		})
	}
}
