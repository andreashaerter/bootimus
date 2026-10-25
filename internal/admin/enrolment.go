package admin

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"bootimus/internal/deployment"
	"bootimus/internal/models"
)

func (h *Handler) EnrolmentRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := h.storage.ListEnrolmentRules()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		if rules == nil {
			rules = []*models.EnrolmentRule{}
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: rules})
	case http.MethodPost:
		h.createEnrolmentRule(w, r)
	case http.MethodPut:
		h.updateEnrolmentRule(w, r)
	case http.MethodDelete:
		h.deleteEnrolmentRule(w, r)
	default:
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
	}
}

func (h *Handler) checkEnrolmentRuleReferences(rule *models.EnrolmentRule) error {
	if rule.ClientGroupID != nil {
		if _, err := h.storage.GetClientGroup(*rule.ClientGroupID); err != nil {
			return errors.New("client group not found")
		}
	}
	if rule.ProfileID != nil {
		if _, err := h.storage.GetDeploymentProfile(*rule.ProfileID); err != nil {
			return errors.New("deployment profile not found")
		}
	}
	return nil
}

func (h *Handler) decodeEnrolmentRule(w http.ResponseWriter, r *http.Request) (*models.EnrolmentRule, bool) {
	var rule models.EnrolmentRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return nil, false
	}
	if err := deployment.ValidateRule(&rule); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return nil, false
	}
	if err := h.checkEnrolmentRuleReferences(&rule); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return nil, false
	}
	return &rule, true
}

func (h *Handler) createEnrolmentRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.decodeEnrolmentRule(w, r)
	if !ok {
		return
	}
	rule.ID = 0
	rule.MatchCount = 0
	rule.LastMatchedAt = nil
	if err := h.storage.CreateEnrolmentRule(rule); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Enrolment rule created - %s (priority %d)", rule.Name, rule.Priority)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Enrolment rule created", Data: rule})
}

func (h *Handler) updateEnrolmentRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.enrolmentRuleID(w, r)
	if !ok {
		return
	}
	existing, err := h.storage.GetEnrolmentRule(id)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Enrolment rule not found"})
		return
	}
	rule, ok := h.decodeEnrolmentRule(w, r)
	if !ok {
		return
	}
	if err := h.storage.UpdateEnrolmentRule(id, rule); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	rule.MatchCount = existing.MatchCount
	rule.LastMatchedAt = existing.LastMatchedAt
	log.Printf("Admin: Enrolment rule updated - %s (priority %d, enabled %v)", rule.Name, rule.Priority, rule.Enabled)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Enrolment rule updated", Data: rule})
}

func (h *Handler) deleteEnrolmentRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.enrolmentRuleID(w, r)
	if !ok {
		return
	}
	rule, err := h.storage.GetEnrolmentRule(id)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Enrolment rule not found"})
		return
	}
	if err := h.storage.DeleteEnrolmentRule(id); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Enrolment rule deleted - %s", rule.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Enrolment rule deleted"})
}

func (h *Handler) enrolmentRuleID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil || id == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid enrolment rule ID"})
		return 0, false
	}
	return uint(id), true
}
