package deployment

import (
	"testing"

	"bootimus/internal/models"
)

func facts() Facts {
	return Facts{MAC: "aa:bb:cc:dd:ee:ff", Manufacturer: "Dell Inc.", Product: "OptiPlex 7090", Serial: "ABC1234", UUID: "4c4c4544-0031-3510-8052-b7c04f4d4a32", Platform: "efi", BuildArch: "x86_64"}
}

func enabledRule() *models.EnrolmentRule {
	return &models.EnrolmentRule{Name: "r", Enabled: true}
}

func TestMatchesEachCondition(t *testing.T) {
	f := facts()
	cases := []struct {
		name string
		mod  func(*models.EnrolmentRule)
		want bool
	}{
		{"mac prefix with colons", func(r *models.EnrolmentRule) { r.MACPrefix = "AA:BB:CC" }, true},
		{"mac prefix with dashes", func(r *models.EnrolmentRule) { r.MACPrefix = "aa-bb-cc" }, true},
		{"mac prefix bare hex", func(r *models.EnrolmentRule) { r.MACPrefix = "aabbcc" }, true},
		{"mac prefix mismatch", func(r *models.EnrolmentRule) { r.MACPrefix = "00:11:22" }, false},
		{"manufacturer contains case-insensitive", func(r *models.EnrolmentRule) { r.ManufacturerContains = "dell" }, true},
		{"manufacturer mismatch", func(r *models.EnrolmentRule) { r.ManufacturerContains = "lenovo" }, false},
		{"product contains", func(r *models.EnrolmentRule) { r.ProductContains = "optiplex" }, true},
		{"serial regexp", func(r *models.EnrolmentRule) { r.SerialPattern = `^ABC\d+$` }, true},
		{"serial regexp mismatch", func(r *models.EnrolmentRule) { r.SerialPattern = `^XYZ` }, false},
		{"uuid prefix", func(r *models.EnrolmentRule) { r.UUIDPrefix = "4C4C4544" }, true},
		{"firmware uefi", func(r *models.EnrolmentRule) { r.Firmware = FirmwareUEFI }, true},
		{"firmware bios mismatch", func(r *models.EnrolmentRule) { r.Firmware = FirmwareBIOS }, false},
		{"all conditions must hold", func(r *models.EnrolmentRule) { r.MACPrefix = "aa:bb"; r.ProductContains = "latitude" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := enabledRule()
			tc.mod(r)
			if got := Matches(r, f); got != tc.want {
				t.Errorf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
	r := enabledRule()
	r.MACPrefix = "aa:bb"
	r.Enabled = false
	if Matches(r, f) {
		t.Error("disabled rules must not match")
	}
}

func TestFirmwareDetection(t *testing.T) {
	cases := map[Facts]string{
		{Platform: "pcbios", BuildArch: "i386"}:    FirmwareBIOS,
		{Platform: "efi", BuildArch: "x86_64"}:     FirmwareUEFI,
		{Platform: "efi", BuildArch: "arm64"}:      FirmwareARM64,
		{Platform: "efi", BuildArch: "aarch64"}:    FirmwareARM64,
		{Platform: "", BuildArch: ""}:              "",
		{Platform: "unknown", BuildArch: "x86_64"}: "",
	}
	for f, want := range cases {
		if got := f.Firmware(); got != want {
			t.Errorf("%+v: firmware = %q, want %q", f, got, want)
		}
	}
}

func TestEvaluatePicksLowestPriorityThenID(t *testing.T) {
	f := facts()
	rules := []*models.EnrolmentRule{
		{ID: 3, Name: "late", Enabled: true, Priority: 50, ManufacturerContains: "dell"},
		{ID: 1, Name: "first", Enabled: true, Priority: 10, ManufacturerContains: "dell"},
		{ID: 2, Name: "disabled", Enabled: false, Priority: 1, ManufacturerContains: "dell"},
		{ID: 4, Name: "same-priority-higher-id", Enabled: true, Priority: 10, ManufacturerContains: "dell"},
		{ID: 5, Name: "no match", Enabled: true, Priority: 0, ManufacturerContains: "hp"},
	}
	if got := Evaluate(rules, f); got == nil || got.Name != "first" {
		t.Fatalf("Evaluate picked %+v, want first", got)
	}
	if got := Evaluate(nil, f); got != nil {
		t.Errorf("no rules should give no match")
	}
}

func TestRenderNameAndHostnameSafe(t *testing.T) {
	f := facts()
	cases := map[string]string{
		"lab-{{MAC_SUFFIX}}":           "lab-ddeeff",
		"{{MANUFACTURER}} {{PRODUCT}}": "Dell Inc. OptiPlex 7090",
		"{{HOSTNAME_SAFE}}":            "abc1234",
		"pc-{{SERIAL}}":                "pc-ABC1234",
		"{{MAC}}":                      "aa:bb:cc:dd:ee:ff",
		"":                             "",
	}
	for pattern, want := range cases {
		if got := RenderName(pattern, f); got != want {
			t.Errorf("RenderName(%q) = %q, want %q", pattern, got, want)
		}
	}
	noSerial := f
	noSerial.Serial = ""
	if got := RenderName("{{HOSTNAME_SAFE}}", noSerial); got != "ddeeff" {
		t.Errorf("HOSTNAME_SAFE without a serial should fall back to the MAC suffix, got %q", got)
	}
	if got := HostnameSafe("  Lab PC #12 (east) "); got != "lab-pc-12-east" {
		t.Errorf("HostnameSafe = %q", got)
	}
}

func TestValidateRule(t *testing.T) {
	gid := uint(1)
	cases := []struct {
		name    string
		rule    models.EnrolmentRule
		wantErr bool
	}{
		{"no name", models.EnrolmentRule{MACPrefix: "aa", ClientGroupID: &gid}, true},
		{"no condition", models.EnrolmentRule{Name: "r", ClientGroupID: &gid}, true},
		{"nothing to apply", models.EnrolmentRule{Name: "r", MACPrefix: "aa"}, true},
		{"bad regexp", models.EnrolmentRule{Name: "r", SerialPattern: "(", ClientGroupID: &gid}, true},
		{"bad firmware", models.EnrolmentRule{Name: "r", Firmware: "legacy", ClientGroupID: &gid}, true},
		{"bad mac prefix", models.EnrolmentRule{Name: "r", MACPrefix: "zz", ClientGroupID: &gid}, true},
		{"ok", models.EnrolmentRule{Name: "r", MACPrefix: "aa:bb", Firmware: "UEFI", ClientGroupID: &gid}, false},
		{"name pattern only", models.EnrolmentRule{Name: "r", ProductContains: "x", NamePattern: "pc-{{MAC_SUFFIX}}"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.rule
			err := ValidateRule(&r)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && tc.name == "ok" && r.Firmware != FirmwareUEFI {
				t.Errorf("firmware should be normalised to lower case, got %q", r.Firmware)
			}
		})
	}
}

func TestEligibleAndPlanPreserveHumanChoices(t *testing.T) {
	gid, pid, other := uint(1), uint(2), uint(9)
	rule := &models.EnrolmentRule{ID: 7, Name: "r", Enabled: true, ClientGroupID: &gid, ProfileID: &pid, MakeStatic: true, NamePattern: "pc-{{MAC_SUFFIX}}"}
	f := facts()

	if !Eligible(&models.Client{}) {
		t.Error("a bare discovered client is eligible")
	}
	for name, c := range map[string]*models.Client{
		"static":   {Static: true},
		"grouped":  {ClientGroupID: &other},
		"profiled": {ProfileID: &other},
		"enrolled": {EnrolledByRuleID: &other},
		"nil":      nil,
	} {
		if Eligible(c) {
			t.Errorf("%s client must not be eligible", name)
		}
	}

	plan := Plan(&models.Client{MACAddress: f.MAC}, rule, f)
	if plan.RuleID != 7 || plan.ClientGroupID == nil || *plan.ClientGroupID != gid || plan.ProfileID == nil || *plan.ProfileID != pid || !plan.MakeStatic || plan.Name != "pc-ddeeff" {
		t.Errorf("full plan wrong: %+v", plan)
	}
	named := Plan(&models.Client{MACAddress: f.MAC, Name: "kept"}, rule, f)
	if named.Name != "" {
		t.Errorf("an existing name must be kept, plan wanted %q", named.Name)
	}
}
