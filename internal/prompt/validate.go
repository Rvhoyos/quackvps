package prompt

import (
	"fmt"
	"strconv"

	"github.com/rvhoyos/quackvps/internal/config"
	"github.com/rvhoyos/quackvps/internal/mcver"
)

// validateMCVersionInput checks a typed Minecraft version parses and meets the
// supported minimum, reusing the same rules the Config gate enforces.
func validateMCVersionInput(s string) error {
	v, err := mcver.Parse(s)
	if err != nil {
		return err
	}
	min, _ := mcver.Parse(config.MinMCVersion)
	if !mcver.AtLeast(v, min) {
		return fmt.Errorf("minimum supported version is %s", config.MinMCVersion)
	}
	return nil
}

func validateNotEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("cannot be empty")
	}
	return nil
}

// mustAtoi converts a string already validated as an integer. A parse failure
// here is a programming error (the field validator should have caught it), so we
// fall back to zero rather than panicking in a wizard.
func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
