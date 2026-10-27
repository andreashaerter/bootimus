package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"

	"golang.org/x/crypto/bcrypt"
)

func TestBootPasswordUpdatesAndRedaction(test *testing.T) {
	for _, target := range []string{"menu", "image"} {
		test.Run(target, func(test *testing.T) {
			store := storagetest.New(test)
			image := &models.Image{Name: "Test", Filename: "test.iso", Enabled: true, Public: true}
			if err := store.CreateImage(image); err != nil {
				test.Fatal(err)
			}
			handler := &Handler{storage: store, BootAuthReady: func() bool { return true }}
			update := func(body, scheme string) *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodPut, scheme+"://bootimus/api/images?filename=test.iso", strings.NewReader(body))
				recorder := httptest.NewRecorder()
				if target == "image" {
					handler.UpdateImage(recorder, request)
				} else {
					handler.UpdateMenuTheme(recorder, request)
				}
				return recorder
			}
			read := func() (string, bool) {
				if target == "image" {
					current, err := store.GetImage("test.iso")
					if err != nil {
						test.Fatal(err)
					}
					return current.BootPasswordHash, current.BootPasswordRequired
				}
				current, err := store.GetMenuTheme()
				if err != nil {
					test.Fatal(err)
				}
				return current.BootPasswordHash, current.BootPasswordRequired
			}
			response := update(`{"boot_password":"secret+&=%"}`, "http")
			if response.Code != http.StatusBadRequest {
				test.Fatal("password accepted without HTTPS")
			}
			response = update(`{"boot_password":"secret+&=%"}`, "https")
			if response.Code != http.StatusOK {
				test.Fatalf("set password: %d", response.Code)
			}
			hash, required := read()
			if !required || bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret+&=%")) != nil {
				test.Fatal("password was not hashed and stored")
			}
			for _, forbidden := range []string{"secret+&=%", hash, "boot_password_hash"} {
				if strings.Contains(response.Body.String(), forbidden) {
					test.Fatal("API response exposes password material")
				}
			}
			response = update(`{"name":"Renamed","title":"Renamed","boot_password_required":false,"boot_password_hash":"injected"}`, "https")
			retained, required := read()
			if response.Code != http.StatusOK || retained != hash || !required {
				test.Fatal("ordinary edit or derived fields removed protection")
			}
			body, err := json.Marshal(map[string]string{"boot_password": strings.Repeat("x", 73)})
			if err != nil {
				test.Fatal(err)
			}
			if response = update(string(body), "https"); response.Code != http.StatusBadRequest {
				test.Fatal("overlong bcrypt password accepted")
			}
			if retained, _ := read(); retained != hash {
				test.Fatal("failed edit changed stored password")
			}
			if response = update(`{"boot_password":"replacement"}`, "https"); response.Code != http.StatusOK {
				test.Fatal("password rotation failed")
			}
			if retained, _ := read(); retained == hash {
				test.Fatal("rotation kept the old hash")
			}
			if response = update(`{"boot_password":""}`, "https"); response.Code != http.StatusOK {
				test.Fatal("password removal failed")
			}
			if retained, required := read(); retained != "" || required {
				test.Fatal("password removal did not persist")
			}
		})
	}
}

func TestBootPasswordRequiresActiveTLSListener(test *testing.T) {
	handler := &Handler{}
	request := httptest.NewRequest(http.MethodPut, "https://bootimus/api/theme", nil)
	password, hash, required := "secret", "", false
	if err := handler.updateBootPassword(request, &password, &hash, &required); err == nil {
		test.Fatal("accepted a password without an active authentication listener")
	}
}
