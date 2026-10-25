package storage

import (
	"time"

	"bootimus/internal/models"

	"gorm.io/gorm"
)

type EnrolmentUpdate struct {
	RuleID        uint
	ClientGroupID *uint
	ProfileID     *uint
	MakeStatic    bool
	Name          string
	At            time.Time
}

func listEnrolmentRules(db *gorm.DB) ([]*models.EnrolmentRule, error) {
	var rules []*models.EnrolmentRule
	if err := db.Order("priority ASC, id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func getEnrolmentRule(db *gorm.DB, id uint) (*models.EnrolmentRule, error) {
	var rule models.EnrolmentRule
	if err := db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func createEnrolmentRule(db *gorm.DB, rule *models.EnrolmentRule) error {
	return db.Create(rule).Error
}

func updateEnrolmentRule(db *gorm.DB, id uint, rule *models.EnrolmentRule) error {
	rule.ID = id
	return db.Model(&models.EnrolmentRule{}).Where("id = ?", id).
		Select("Name", "Priority", "Enabled", "MACPrefix", "ManufacturerContains", "ProductContains", "SerialPattern", "UUIDPrefix", "Firmware", "ClientGroupID", "ProfileID", "MakeStatic", "NamePattern", "UpdatedAt").
		Updates(rule).Error
}

func deleteEnrolmentRule(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Client{}).Where("enrolled_by_rule_id = ?", id).Update("enrolled_by_rule_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&models.EnrolmentRule{}, id).Error
	})
}

func applyEnrolment(db *gorm.DB, mac string, update EnrolmentUpdate) error {
	return db.Transaction(func(tx *gorm.DB) error {
		fields := map[string]interface{}{
			"enrolled_by_rule_id": update.RuleID,
			"enrolled_at":         update.At,
		}
		if update.ClientGroupID != nil {
			fields["client_group_id"] = *update.ClientGroupID
		}
		if update.ProfileID != nil {
			fields["profile_id"] = *update.ProfileID
		}
		if update.MakeStatic {
			fields["static"] = true
		}
		if update.Name != "" {
			fields["name"] = update.Name
		}
		if err := tx.Model(&models.Client{}).Where("mac_address = ?", mac).Updates(fields).Error; err != nil {
			return err
		}
		return tx.Model(&models.EnrolmentRule{}).Where("id = ?", update.RuleID).
			Updates(map[string]interface{}{"match_count": gorm.Expr("match_count + 1"), "last_matched_at": update.At}).Error
	})
}

func listUnenrolledClients(db *gorm.DB) ([]*models.Client, error) {
	var clients []*models.Client
	err := db.Where("static = ? AND client_group_id IS NULL AND profile_id IS NULL AND enrolled_by_rule_id IS NULL", false).
		Order("created_at ASC").Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func (s *SQLiteStore) ListEnrolmentRules() ([]*models.EnrolmentRule, error) {
	return listEnrolmentRules(s.db)
}

func (s *SQLiteStore) GetEnrolmentRule(id uint) (*models.EnrolmentRule, error) {
	return getEnrolmentRule(s.db, id)
}

func (s *SQLiteStore) CreateEnrolmentRule(rule *models.EnrolmentRule) error {
	return createEnrolmentRule(s.db, rule)
}

func (s *SQLiteStore) UpdateEnrolmentRule(id uint, rule *models.EnrolmentRule) error {
	return updateEnrolmentRule(s.db, id, rule)
}

func (s *SQLiteStore) DeleteEnrolmentRule(id uint) error {
	return deleteEnrolmentRule(s.db, id)
}

func (s *SQLiteStore) ApplyEnrolment(mac string, update EnrolmentUpdate) error {
	return applyEnrolment(s.db, mac, update)
}

func (s *SQLiteStore) ListUnenrolledClients() ([]*models.Client, error) {
	return listUnenrolledClients(s.db)
}

func (s *PostgresStore) ListEnrolmentRules() ([]*models.EnrolmentRule, error) {
	return listEnrolmentRules(s.db)
}

func (s *PostgresStore) GetEnrolmentRule(id uint) (*models.EnrolmentRule, error) {
	return getEnrolmentRule(s.db, id)
}

func (s *PostgresStore) CreateEnrolmentRule(rule *models.EnrolmentRule) error {
	return createEnrolmentRule(s.db, rule)
}

func (s *PostgresStore) UpdateEnrolmentRule(id uint, rule *models.EnrolmentRule) error {
	return updateEnrolmentRule(s.db, id, rule)
}

func (s *PostgresStore) DeleteEnrolmentRule(id uint) error {
	return deleteEnrolmentRule(s.db, id)
}

func (s *PostgresStore) ApplyEnrolment(mac string, update EnrolmentUpdate) error {
	return applyEnrolment(s.db, mac, update)
}

func (s *PostgresStore) ListUnenrolledClients() ([]*models.Client, error) {
	return listUnenrolledClients(s.db)
}
