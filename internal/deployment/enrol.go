package deployment

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"bootimus/internal/models"
)

const (
	FirmwareBIOS  = "bios"
	FirmwareUEFI  = "uefi"
	FirmwareARM64 = "arm64"
)

type Facts struct {
	MAC          string
	Manufacturer string
	Product      string
	Serial       string
	UUID         string
	Platform     string
	BuildArch    string
}

func FactsFor(client *models.Client, inv *models.HardwareInventory) Facts {
	f := Facts{}
	if client != nil {
		f.MAC = client.MACAddress
	}
	if inv != nil {
		if f.MAC == "" {
			f.MAC = inv.MACAddress
		}
		f.Manufacturer = inv.Manufacturer
		f.Product = inv.Product
		f.Serial = inv.Serial
		f.UUID = inv.UUID
		f.Platform = inv.Platform
		f.BuildArch = inv.BuildArch
	}
	f.MAC = strings.ToLower(strings.ReplaceAll(f.MAC, "-", ":"))
	return f
}

func (f Facts) Firmware() string {
	arch := strings.ToLower(f.BuildArch)
	if strings.Contains(arch, "arm64") || strings.Contains(arch, "aarch64") {
		return FirmwareARM64
	}
	platform := strings.ToLower(f.Platform)
	switch {
	case strings.HasPrefix(platform, "efi"):
		return FirmwareUEFI
	case platform == "pcbios":
		return FirmwareBIOS
	}
	return ""
}

func hexOnly(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func ValidateRule(rule *models.EnrolmentRule) error {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.MACPrefix = strings.TrimSpace(rule.MACPrefix)
	rule.ManufacturerContains = strings.TrimSpace(rule.ManufacturerContains)
	rule.ProductContains = strings.TrimSpace(rule.ProductContains)
	rule.SerialPattern = strings.TrimSpace(rule.SerialPattern)
	rule.UUIDPrefix = strings.TrimSpace(rule.UUIDPrefix)
	rule.Firmware = strings.ToLower(strings.TrimSpace(rule.Firmware))
	rule.NamePattern = strings.TrimSpace(rule.NamePattern)
	if rule.Name == "" {
		return errors.New("rule name is required")
	}
	if rule.MACPrefix == "" && rule.ManufacturerContains == "" && rule.ProductContains == "" && rule.SerialPattern == "" && rule.UUIDPrefix == "" && rule.Firmware == "" {
		return errors.New("a rule needs at least one match condition")
	}
	if rule.MACPrefix != "" && hexOnly(rule.MACPrefix) == "" {
		return errors.New("MAC prefix must contain hexadecimal digits")
	}
	if rule.SerialPattern != "" {
		if _, err := regexp.Compile(rule.SerialPattern); err != nil {
			return fmt.Errorf("serial pattern is not a valid regular expression: %v", err)
		}
	}
	switch rule.Firmware {
	case "", FirmwareBIOS, FirmwareUEFI, FirmwareARM64:
	default:
		return errors.New("firmware must be bios, uefi, arm64 or empty")
	}
	if rule.ClientGroupID != nil && *rule.ClientGroupID == 0 {
		rule.ClientGroupID = nil
	}
	if rule.ProfileID != nil && *rule.ProfileID == 0 {
		rule.ProfileID = nil
	}
	if rule.ClientGroupID == nil && rule.ProfileID == nil && !rule.MakeStatic && rule.NamePattern == "" {
		return errors.New("a rule needs something to apply: a group, a profile, a name pattern or make static")
	}
	return nil
}

func Matches(rule *models.EnrolmentRule, f Facts) bool {
	if rule == nil || !rule.Enabled {
		return false
	}
	if rule.MACPrefix != "" && !strings.HasPrefix(hexOnly(f.MAC), hexOnly(rule.MACPrefix)) {
		return false
	}
	if rule.ManufacturerContains != "" && !strings.Contains(strings.ToLower(f.Manufacturer), strings.ToLower(rule.ManufacturerContains)) {
		return false
	}
	if rule.ProductContains != "" && !strings.Contains(strings.ToLower(f.Product), strings.ToLower(rule.ProductContains)) {
		return false
	}
	if rule.SerialPattern != "" {
		re, err := regexp.Compile(rule.SerialPattern)
		if err != nil || !re.MatchString(f.Serial) {
			return false
		}
	}
	if rule.UUIDPrefix != "" && !strings.HasPrefix(strings.ToLower(f.UUID), strings.ToLower(rule.UUIDPrefix)) {
		return false
	}
	if rule.Firmware != "" && f.Firmware() != rule.Firmware {
		return false
	}
	return true
}

func SortRules(rules []*models.EnrolmentRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
}

func Evaluate(rules []*models.EnrolmentRule, f Facts) *models.EnrolmentRule {
	ordered := append([]*models.EnrolmentRule{}, rules...)
	SortRules(ordered)
	for _, rule := range ordered {
		if Matches(rule, f) {
			return rule
		}
	}
	return nil
}

func Eligible(client *models.Client) bool {
	if client == nil {
		return false
	}
	return !client.Static && client.ClientGroupID == nil && client.ProfileID == nil && client.EnrolledByRuleID == nil
}

var hostnameUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func HostnameSafe(s string) string {
	s = hostnameUnsafe.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-")
	}
	return s
}

func macSuffix(mac string) string {
	h := hexOnly(mac)
	if len(h) > 6 {
		h = h[len(h)-6:]
	}
	return h
}

func RenderName(pattern string, f Facts) string {
	if pattern == "" {
		return ""
	}
	safe := HostnameSafe(f.Serial)
	if safe == "" {
		safe = macSuffix(f.MAC)
	}
	r := strings.NewReplacer(
		"{{MAC}}", f.MAC,
		"{{MAC_SUFFIX}}", macSuffix(f.MAC),
		"{{SERIAL}}", f.Serial,
		"{{PRODUCT}}", f.Product,
		"{{MANUFACTURER}}", f.Manufacturer,
		"{{HOSTNAME_SAFE}}", safe,
	)
	return strings.TrimSpace(r.Replace(pattern))
}

type Enrolment struct {
	RuleID        uint
	ClientGroupID *uint
	ProfileID     *uint
	MakeStatic    bool
	Name          string
	At            time.Time
}

func Plan(client *models.Client, rule *models.EnrolmentRule, f Facts) Enrolment {
	e := Enrolment{RuleID: rule.ID, At: time.Now()}
	if client.ClientGroupID == nil {
		e.ClientGroupID = rule.ClientGroupID
	}
	if client.ProfileID == nil {
		e.ProfileID = rule.ProfileID
	}
	if !client.Static && rule.MakeStatic {
		e.MakeStatic = true
	}
	if strings.TrimSpace(client.Name) == "" {
		e.Name = RenderName(rule.NamePattern, f)
	}
	return e
}
