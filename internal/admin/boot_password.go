package admin

import (
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

func (handler *Handler) updateBootPassword(request *http.Request, password *string, hash *string, required *bool) error {
	if password == nil {
		return nil
	}
	if *password == "" {
		*hash = ""
		*required = false
		return nil
	}
	if request.TLS == nil || handler.BootAuthReady == nil || !handler.BootAuthReady() {
		return fmt.Errorf("boot passwords require admin HTTPS and a certificate trusted by the bootloader")
	}
	if len(*password) > 72 {
		return fmt.Errorf("boot passwords must be at most 72 bytes")
	}
	encoded, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash boot password: %w", err)
	}
	*hash = string(encoded)
	*required = true
	return nil
}
