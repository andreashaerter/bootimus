package storage

import (
	"testing"

	"bootimus/internal/models"
)

func TestCreateKeepsExplicitZeroValues(t *testing.T) {
	store := newTestStore(t)

	rule := &models.EnrolmentRule{Name: "off", MACPrefix: "aa", Enabled: false, Priority: 0}
	if err := store.CreateEnrolmentRule(rule); err != nil {
		t.Fatal(err)
	}
	gotRule, err := store.GetEnrolmentRule(rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRule.Enabled || gotRule.Priority != 0 {
		t.Errorf("rule: want enabled=false priority=0, got enabled=%v priority=%d", gotRule.Enabled, gotRule.Priority)
	}
	defaulted := &models.EnrolmentRule{Name: "on", MACPrefix: "bb", Enabled: true, Priority: 100}
	if err := store.CreateEnrolmentRule(defaulted); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetEnrolmentRule(defaulted.ID); !got.Enabled || got.Priority != 100 {
		t.Errorf("rule with defaults: got enabled=%v priority=%d", got.Enabled, got.Priority)
	}

	group := &models.ClientGroup{Name: "disabled-group", Enabled: false}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetClientGroup(group.ID); err != nil || got.Enabled {
		t.Errorf("client group: want enabled=false, got %+v err %v", got, err)
	}

	igroup := &models.ImageGroup{Name: "disabled-images", Enabled: false}
	if err := store.CreateImageGroup(igroup); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetImageGroup(igroup.ID); err != nil || got.Enabled {
		t.Errorf("image group: want enabled=false, got %+v err %v", got, err)
	}

	task := &models.ScheduledTask{Name: "paused", Enabled: false, CronExpr: "0 * * * *", ClientGroupID: group.ID, ActionType: "wake"}
	if err := store.CreateScheduledTask(task); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetScheduledTask(task.ID); err != nil || got.Enabled {
		t.Errorf("scheduled task: want enabled=false, got %+v err %v", got, err)
	}

	client := &models.Client{MACAddress: "aa:bb:cc:00:00:09", Enabled: true, ShowPublicImages: false}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetClient(client.MACAddress); err != nil || got.ShowPublicImages {
		t.Errorf("client: want show_public_images=false, got %+v err %v", got, err)
	}

	cfg := &models.WebhookConfig{URL: "http://hook", Enabled: true, OnBootStarted: false, OnInstall: false}
	if err := store.UpdateWebhookConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetWebhookConfig(); err != nil || got.OnBootStarted || got.OnInstall {
		t.Errorf("webhook: want on_boot_started=false on_install=false, got %+v err %v", got, err)
	}
}
