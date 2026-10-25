package storage

import (
	"errors"
	"testing"

	"bootimus/internal/models"
)

func TestCreateClientReportsExistingMAC(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:10"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "first"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	err := store.CreateClient(&models.Client{MACAddress: mac, Name: "second"})
	if !errors.Is(err, ErrClientExists) {
		t.Fatalf("CreateClient with existing MAC: error = %v, want ErrClientExists", err)
	}

	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.Name != "first" {
		t.Errorf("client name = %q, want first", client.Name)
	}
}
