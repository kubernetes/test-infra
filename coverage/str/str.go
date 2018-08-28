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

//PercentageForTestgrid converts a fraction number to percentage string representation used on TestGrid
func PercentageForTestgrid(f float32) string {
	return fmt.Sprintf("%.1f", f*100)
}

//PercentageForCovbotDelta converts a fraction number to percentage string representation used by
// covbot
func PercentageForCovbotDelta(f float32) string {
	return fmt.Sprintf("%.1f", f*100)
}
