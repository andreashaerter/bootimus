package storage

import (
	"reflect"

	"gorm.io/gorm"
)

func createKeepingZeroValues(db *gorm.DB, model interface{}, fields ...string) error {
	return writeKeepingZeroValues(db, model, fields, func(tx *gorm.DB) error { return tx.Create(model).Error })
}

func saveKeepingZeroValues(db *gorm.DB, model interface{}, fields ...string) error {
	return writeKeepingZeroValues(db, model, fields, func(tx *gorm.DB) error { return tx.Save(model).Error })
}

func writeKeepingZeroValues(db *gorm.DB, model interface{}, fields []string, write func(tx *gorm.DB) error) error {
	rv := reflect.ValueOf(model).Elem()
	wanted := make([]reflect.Value, len(fields))
	for i, name := range fields {
		wanted[i] = reflect.ValueOf(rv.FieldByName(name).Interface())
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := write(tx); err != nil {
			return err
		}
		for i, name := range fields {
			rv.FieldByName(name).Set(wanted[i])
		}
		return tx.Model(model).Select(fields).Updates(model).Error
	})
}
