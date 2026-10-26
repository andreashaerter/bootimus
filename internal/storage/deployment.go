package storage

import (
	"errors"
	"fmt"
	"strings"

	"bootimus/internal/models"

	"gorm.io/gorm"
)

var ErrDeploymentProfileExists = errors.New("a deployment profile with that name already exists")

func ValidateDeploymentProfile(p *models.DeploymentProfile) error {
	p.Name = strings.TrimSpace(p.Name)
	p.ImageFilename = strings.TrimSpace(p.ImageFilename)
	p.AutoInstallFile = strings.TrimSpace(p.AutoInstallFile)
	p.BootloaderSet = strings.TrimSpace(p.BootloaderSet)
	p.FirstBootAction = strings.TrimSpace(p.FirstBootAction)
	if p.Name == "" {
		return errors.New("deployment profile name is required")
	}
	switch p.FirstBootAction {
	case models.FirstBootActionNone:
	case models.FirstBootActionInstall:
		if p.ImageFilename == "" {
			return errors.New("a first-boot install needs an image")
		}
	case models.FirstBootActionDeploy:
		if p.DiskImageID == nil || *p.DiskImageID == 0 {
			return errors.New("a first-boot deploy needs a disk image")
		}
	default:
		return fmt.Errorf("first-boot action must be install, deploy or empty, not %q", p.FirstBootAction)
	}
	if p.SANTargetID != nil && *p.SANTargetID == 0 {
		p.SANTargetID = nil
	}
	if p.DiskImageID != nil && *p.DiskImageID == 0 {
		p.DiskImageID = nil
	}
	if p.BootFromSAN && p.SANTargetID == nil {
		return errors.New("boot from SAN by default needs a SAN target")
	}
	return nil
}

func translateDeploymentProfileError(db *gorm.DB, err error) error {
	if err == nil {
		return nil
	}
	if translator, ok := db.Dialector.(gorm.ErrorTranslator); ok && errors.Is(translator.Translate(err), gorm.ErrDuplicatedKey) {
		return ErrDeploymentProfileExists
	}
	return err
}

func listDeploymentProfiles(db *gorm.DB) ([]*models.DeploymentProfile, error) {
	var profiles []*models.DeploymentProfile
	if err := db.Order("name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

func getDeploymentProfile(db *gorm.DB, id uint) (*models.DeploymentProfile, error) {
	var profile models.DeploymentProfile
	if err := db.First(&profile, id).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

func createDeploymentProfile(db *gorm.DB, profile *models.DeploymentProfile) error {
	if err := ValidateDeploymentProfile(profile); err != nil {
		return err
	}
	return translateDeploymentProfileError(db, db.Create(profile).Error)
}

func updateDeploymentProfile(db *gorm.DB, id uint, profile *models.DeploymentProfile) error {
	if err := ValidateDeploymentProfile(profile); err != nil {
		return err
	}
	profile.ID = id
	return translateDeploymentProfileError(db, db.Model(&models.DeploymentProfile{}).Where("id = ?", id).
		Select("Name", "Description", "ImageFilename", "AutoInstallFile", "SANTargetID", "BootFromSAN", "BootloaderSet", "DiskImageID", "FirstBootAction", "ShowPublicImages", "UpdatedAt").
		Updates(profile).Error)
}

func deleteDeploymentProfile(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.EnrolmentRule{}).
			Where("profile_id = ? AND client_group_id IS NULL AND make_static = ? AND TRIM(COALESCE(name_pattern, '')) = ?", id, false, "").
			Update("enabled", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.EnrolmentRule{}).Where("profile_id = ?", id).Update("profile_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Client{}).Where("profile_id = ?", id).Update("profile_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.ClientGroup{}).Where("profile_id = ?", id).Update("profile_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&models.DeploymentProfile{}, id).Error
	})
}

func countDeploymentProfileUsers(db *gorm.DB, id uint) (clients int64, groups int64, err error) {
	if err = db.Model(&models.Client{}).Where("profile_id = ?", id).Count(&clients).Error; err != nil {
		return
	}
	err = db.Model(&models.ClientGroup{}).Where("profile_id = ?", id).Count(&groups).Error
	return
}

func (s *SQLiteStore) ListDeploymentProfiles() ([]*models.DeploymentProfile, error) {
	return listDeploymentProfiles(s.db)
}

func (s *SQLiteStore) GetDeploymentProfile(id uint) (*models.DeploymentProfile, error) {
	return getDeploymentProfile(s.db, id)
}

func (s *SQLiteStore) CreateDeploymentProfile(profile *models.DeploymentProfile) error {
	return createDeploymentProfile(s.db, profile)
}

func (s *SQLiteStore) UpdateDeploymentProfile(id uint, profile *models.DeploymentProfile) error {
	return updateDeploymentProfile(s.db, id, profile)
}

func (s *SQLiteStore) DeleteDeploymentProfile(id uint) error {
	return deleteDeploymentProfile(s.db, id)
}

func (s *SQLiteStore) CountDeploymentProfileUsers(id uint) (int64, int64, error) {
	return countDeploymentProfileUsers(s.db, id)
}

func (s *PostgresStore) ListDeploymentProfiles() ([]*models.DeploymentProfile, error) {
	return listDeploymentProfiles(s.db)
}

func (s *PostgresStore) GetDeploymentProfile(id uint) (*models.DeploymentProfile, error) {
	return getDeploymentProfile(s.db, id)
}

func (s *PostgresStore) CreateDeploymentProfile(profile *models.DeploymentProfile) error {
	return createDeploymentProfile(s.db, profile)
}

func (s *PostgresStore) UpdateDeploymentProfile(id uint, profile *models.DeploymentProfile) error {
	return updateDeploymentProfile(s.db, id, profile)
}

func (s *PostgresStore) DeleteDeploymentProfile(id uint) error {
	return deleteDeploymentProfile(s.db, id)
}

func (s *PostgresStore) CountDeploymentProfileUsers(id uint) (int64, int64, error) {
	return countDeploymentProfileUsers(s.db, id)
}
