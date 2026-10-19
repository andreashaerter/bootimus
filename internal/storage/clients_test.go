package storage

import (
	"testing"

	"bootimus/internal/models"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return store
}

func TestDeleteClientHardDeletesAndAllowsRecreate(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:ff"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "first"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.DeleteClient(mac); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	var count int64
	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 0 {
		t.Fatalf("expected client row to be gone, found %d rows", count)
	}

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "second"}); err != nil {
		t.Fatalf("CreateClient after delete: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient after recreate: %v", err)
	}
	if client.Name != "second" {
		t.Fatalf("expected recreated client, got %q", client.Name)
	}
}

func TestDeleteClientMissingMACIsNoop(t *testing.T) {
	store := newTestStore(t)
	if err := store.DeleteClient("00:00:00:00:00:00"); err != nil {
		t.Fatalf("DeleteClient on missing MAC: %v", err)
	}
}

func TestDeleteClientDetachesBootLogsAndInventory(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:01"

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if err := store.db.Create(&models.BootLog{ClientID: &client.ID, MACAddress: mac}).Error; err != nil {
		t.Fatalf("create boot log: %v", err)
	}
	if err := store.db.Create(&models.HardwareInventory{ClientID: &client.ID, MACAddress: mac}).Error; err != nil {
		t.Fatalf("create inventory: %v", err)
	}

	if err := store.DeleteClient(mac); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	var bootLog models.BootLog
	if err := store.db.Where("mac_address = ?", mac).First(&bootLog).Error; err != nil {
		t.Fatalf("boot log should survive client deletion: %v", err)
	}
	if bootLog.ClientID != nil {
		t.Fatalf("expected boot log client_id to be nulled, got %d", *bootLog.ClientID)
	}

	var inv models.HardwareInventory
	if err := store.db.Where("mac_address = ?", mac).First(&inv).Error; err != nil {
		t.Fatalf("inventory should survive client deletion: %v", err)
	}
	if inv.ClientID != nil {
		t.Fatalf("expected inventory client_id to be nulled, got %d", *inv.ClientID)
	}
}

func TestAutoMigratePurgesLegacySoftDeletedClients(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:02"

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var count int64
	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 1 {
		t.Fatalf("expected soft-deleted row to exist before migration, found %d", count)
	}

	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 0 {
		t.Fatalf("expected soft-deleted row to be purged, found %d", count)
	}

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient after purge: %v", err)
	}
}

func TestUpdateClientPersistsAutoInstallFile(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:03"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "node"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	if err := store.UpdateClient(mac, &models.Client{Name: "node", AutoInstallFile: "ubuntu/user-data.yaml"}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.AutoInstallFile != "ubuntu/user-data.yaml" {
		t.Fatalf("expected auto install file to persist, got %q", client.AutoInstallFile)
	}

	if err := store.UpdateClient(mac, &models.Client{Name: "node"}); err != nil {
		t.Fatalf("UpdateClient clear: %v", err)
	}
	client, err = store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient after clear: %v", err)
	}
	if client.AutoInstallFile != "" {
		t.Fatalf("expected auto install file to clear, got %q", client.AutoInstallFile)
	}
}

func TestUpdateClientGroupPersistsAutoInstallFile(t *testing.T) {
	store := newTestStore(t)

	group := &models.ClientGroup{Name: "fleet"}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatalf("CreateClientGroup: %v", err)
	}

	if err := store.UpdateClientGroup(group.ID, &models.ClientGroup{Name: "fleet", AutoInstallFile: "debian/preseed.cfg"}); err != nil {
		t.Fatalf("UpdateClientGroup: %v", err)
	}
	got, err := store.GetClientGroup(group.ID)
	if err != nil {
		t.Fatalf("GetClientGroup: %v", err)
	}
	if got.AutoInstallFile != "debian/preseed.cfg" {
		t.Fatalf("expected group auto install file to persist, got %q", got.AutoInstallFile)
	}

	if err := store.UpdateClientGroup(group.ID, &models.ClientGroup{Name: "fleet"}); err != nil {
		t.Fatalf("UpdateClientGroup clear: %v", err)
	}
	got, err = store.GetClientGroup(group.ID)
	if err != nil {
		t.Fatalf("GetClientGroup after clear: %v", err)
	}
	if got.AutoInstallFile != "" {
		t.Fatalf("expected group auto install file to clear, got %q", got.AutoInstallFile)
	}
}

func TestUpdateClientPersistsProxyDHCPNoVendorOption(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:04"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "i219"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	for _, want := range []bool{true, false} {
		if err := store.UpdateClient(mac, &models.Client{Name: "i219", ProxyDHCPNoVendorOption: want}); err != nil {
			t.Fatalf("UpdateClient(%v): %v", want, err)
		}
		client, err := store.GetClient(mac)
		if err != nil {
			t.Fatalf("GetClient: %v", err)
		}
		if client.ProxyDHCPNoVendorOption != want {
			t.Fatalf("ProxyDHCPNoVendorOption = %v, want %v", client.ProxyDHCPNoVendorOption, want)
		}
	}
}

func TestSyncImagesSetsUKIDefaultsOnCreateOnly(t *testing.T) {
	store := newTestStore(t)
	files := []models.SyncFile{
		{Name: "hello-world", Filename: "hello-world.efi", Size: 10, BootMethod: "uki", BootParams: "audit=0"},
		{Name: "ubuntu", Filename: "ubuntu.iso", Size: 20},
	}
	if err := store.SyncImages(files); err != nil {
		t.Fatal(err)
	}

	img, err := store.GetImage("hello-world.efi")
	if err != nil {
		t.Fatal(err)
	}
	if img.BootMethod != "uki" || img.BootParams != "audit=0" {
		t.Errorf("UKI image = %q/%q, want uki/audit=0", img.BootMethod, img.BootParams)
	}
	iso, err := store.GetImage("ubuntu.iso")
	if err != nil {
		t.Fatal(err)
	}
	if iso.BootMethod != "sanboot" {
		t.Errorf("ISO boot method = %q, want sanboot", iso.BootMethod)
	}

	img.BootParams = "audit=0 console=ttyS0"
	if err := store.UpdateImage(img.Filename, img); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncImages(files); err != nil {
		t.Fatal(err)
	}
	img, _ = store.GetImage("hello-world.efi")
	if img.BootParams != "audit=0 console=ttyS0" {
		t.Errorf("rescan overwrote edited boot params: %q", img.BootParams)
	}
}
