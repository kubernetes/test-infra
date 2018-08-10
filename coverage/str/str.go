/*
Helper functions shared by more than one other go files
*/

package str

import (
	"fmt"
)

// PercentStr converts a fraction number to percentage string representation
func PercentStr(f float32) string {
	return fmt.Sprintf("%.1f%%", f*100)
}

func PercentageForTestgrid(f float32) string {
	return fmt.Sprintf("%.1f", f*100)
}

func PercentageForCovbotDelta(f float32) string {
	return fmt.Sprintf("%.1f", f*100)
}
