package utils

import (
	"time"
)

var (
	// AsiaJakartaLocation is the timezone for Indonesia
	AsiaJakartaLocation *time.Location
)

func init() {
	// Load Asia/Jakarta timezone
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// Fallback to UTC+7 if timezone data not available
		AsiaJakartaLocation = time.FixedZone("WIB", 7*60*60)
	} else {
		AsiaJakartaLocation = loc
	}
}

// GetAsiaJakartaLocation returns the Asia/Jakarta timezone
func GetAsiaJakartaLocation() *time.Location {
	return AsiaJakartaLocation
}

// FormatWhatsAppTime formats time to WhatsApp export format: DD/MM/YY, HH.mm.ss
// Example: 29/10/25, 22.28.35
func FormatWhatsAppTime(t time.Time) string {
	// Convert to Asia/Jakarta timezone
	jakartaTime := t.In(AsiaJakartaLocation)

	// Format: DD/MM/YY, HH.mm.ss
	return jakartaTime.Format("02/01/06, 15.04.05")
}

// ParseUnixToWhatsAppTime converts Unix timestamp to WhatsApp format string
func ParseUnixToWhatsAppTime(unixTimestamp int64) string {
	t := time.Unix(unixTimestamp, 0)
	return FormatWhatsAppTime(t)
}
