package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bootimus/internal/deployment"
	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/webhook"
)

func (s *Server) enrolClient(client *models.Client, inv *models.HardwareInventory, remoteAddr string) (*models.EnrolmentRule, bool) {
	if s.config.Storage == nil || !deployment.Eligible(client) {
		return nil, false
	}
	rules, err := s.config.Storage.ListEnrolmentRules()
	if err != nil || len(rules) == 0 {
		return nil, false
	}
	if inv == nil {
		inv, _ = s.config.Storage.GetLatestHardwareInventory(client.MACAddress)
	}
	facts := deployment.FactsFor(client, inv)
	rule := deployment.Evaluate(rules, facts)
	if rule == nil {
		return nil, false
	}
	plan := deployment.Plan(client, rule, facts)
	update := storage.EnrolmentUpdate{RuleID: plan.RuleID, ClientGroupID: plan.ClientGroupID, ProfileID: plan.ProfileID, MakeStatic: plan.MakeStatic, Name: plan.Name, At: plan.At}
	if err := s.config.Storage.ApplyEnrolment(client.MACAddress, update); err != nil {
		log.Printf("Enrolment: failed to apply rule %q to %s: %v", rule.Name, client.MACAddress, err)
		return nil, false
	}
	client.EnrolledByRuleID = &rule.ID
	client.EnrolledAt = &plan.At
	if plan.ClientGroupID != nil {
		client.ClientGroupID = plan.ClientGroupID
	}
	if plan.ProfileID != nil {
		client.ProfileID = plan.ProfileID
	}
	if plan.MakeStatic {
		client.Static = true
	}
	if plan.Name != "" {
		client.Name = plan.Name
	}
	s.logAndBroadcast("Client %s: auto-enrolled by rule %q (group %s, profile %s, name %q)", client.MACAddress, rule.Name, idOrNone(plan.ClientGroupID), idOrNone(plan.ProfileID), plan.Name)
	ip := remoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	s.webhookNotifier.Fire(webhook.Event{
		Event:      webhook.EventClientAutoEnrolled,
		MAC:        client.MACAddress,
		ClientName: client.Name,
		IP:         ip,
		Metadata: map[string]string{
			"rule":         rule.Name,
			"group_id":     idOrNone(plan.ClientGroupID),
			"profile_id":   idOrNone(plan.ProfileID),
			"made_static":  boolString(plan.MakeStatic),
			"manufacturer": facts.Manufacturer,
			"product":      facts.Product,
			"serial":       facts.Serial,
		},
	})
	return rule, true
}

func idOrNone(id *uint) string {
	if id == nil {
		return "none"
	}
	return strconv.FormatUint(uint64(*id), 10)
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type enrolmentMatch struct {
	MAC          string `json:"mac_address"`
	Name         string `json:"name,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Product      string `json:"product,omitempty"`
	Serial       string `json:"serial,omitempty"`
	Firmware     string `json:"firmware,omitempty"`
	Eligible     bool   `json:"eligible"`
	WouldName    string `json:"would_name,omitempty"`
}

func (s *Server) matchesForRule(rule *models.EnrolmentRule) ([]enrolmentMatch, error) {
	clients, err := s.config.Storage.ListClients()
	if err != nil {
		return nil, err
	}
	probe := *rule
	probe.Enabled = true
	matches := []enrolmentMatch{}
	for _, c := range clients {
		inv, _ := s.config.Storage.GetLatestHardwareInventory(c.MACAddress)
		facts := deployment.FactsFor(c, inv)
		if !deployment.Matches(&probe, facts) {
			continue
		}
		m := enrolmentMatch{MAC: c.MACAddress, Name: c.Name, Manufacturer: facts.Manufacturer, Product: facts.Product, Serial: facts.Serial, Firmware: facts.Firmware(), Eligible: deployment.Eligible(c)}
		if strings.TrimSpace(c.Name) == "" {
			m.WouldName = deployment.RenderName(rule.NamePattern, facts)
		}
		matches = append(matches, m)
	}
	return matches, nil
}

func (s *Server) handleEnrolmentRuleTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}
	if s.config.Storage == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Enrolment rules require a database"})
		return
	}
	var rule models.EnrolmentRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
		return
	}
	if rule.Name == "" {
		rule.Name = "test"
	}
	if err := deployment.ValidateRule(&rule); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	matches, err := s.matchesForRule(&rule)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"matches": matches, "total": len(matches)}})
}

func (s *Server) handleEnrolmentRulesApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}
	if s.config.Storage == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Enrolment rules require a database"})
		return
	}
	clients, err := s.config.Storage.ListUnenrolledClients()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	enrolled := []map[string]string{}
	for _, c := range clients {
		if rule, ok := s.enrolClient(c, nil, r.RemoteAddr); ok {
			enrolled = append(enrolled, map[string]string{"mac_address": c.MACAddress, "name": c.Name, "rule": rule.Name})
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": strconv.Itoa(len(enrolled)) + " client(s) enrolled",
		"data":    map[string]interface{}{"enrolled": enrolled, "considered": len(clients)},
	})
}
