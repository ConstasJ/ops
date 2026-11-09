package domain

import (
	"fmt"
	"strings"
)

// GuardLevel represents the protection level of a container
type GuardLevel string

const (
	GuardAuto     GuardLevel = "auto"     // WUD or automated batch updater can touch
	GuardAssisted GuardLevel = "assisted" // Manual confirmation required, standard container
	GuardDB       GuardLevel = "db"       // Mandatory snapshot before update, abort on snapshot failure
	GuardFrozen   GuardLevel = "frozen"   // Refuse automatic update (irreversible schema migrations)
	GuardLocked   GuardLevel = "locked"   // Refuse update (no compose orchestration or control plane)
)

// IsValid checks if the GuardLevel is one of the recognized values
func (g GuardLevel) IsValid() bool {
	switch g {
	case GuardAuto, GuardAssisted, GuardDB, GuardFrozen, GuardLocked:
		return true
	default:
		return false
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for GuardLevel
func (g *GuardLevel) UnmarshalText(text []byte) error {
	str := strings.TrimSpace(string(text))
	if str == "" {
		*g = GuardAssisted
		return nil
	}
	level := GuardLevel(str)
	if !level.IsValid() {
		return fmt.Errorf("invalid guard level: %q (must be auto, assisted, db, frozen, or locked)", str)
	}
	*g = level
	return nil
}

// String returns string representation of GuardLevel
func (g GuardLevel) String() string {
	return string(g)
}
