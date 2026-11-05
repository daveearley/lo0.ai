// Package format turns numbers, sizes and dates into the short, consistent
// strings the UI shows.
package format

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Int formats n with thousands separators: 35081 becomes "35,081".
func Int(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

var byteUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// Bytes formats a size in binary units with three significant digits at
// most: "64 GiB", "17.3 GiB", "512 MiB".
func Bytes(n uint64) string {
	v := float64(n)
	unit := 0
	// Compare the rounded value, so 1,048,575 bytes is "1 MiB", not "1024 KiB".
	for math.Round(v) >= 1024 && unit < len(byteUnits)-1 {
		v /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return trimmed(v) + " " + byteUnits[unit]
}

// Tokens formats a token count the way context windows are named, where k is
// 1,024: 32768 becomes "32k" and 1048576 becomes "1M".
func Tokens(n int) string {
	switch {
	case n < 1024:
		return strconv.Itoa(n)
	case math.Round(float64(n)/1024) < 1024:
		return trimmed(float64(n)/1024) + "k"
	default:
		return trimmed(float64(n)/(1024*1024)) + "M"
	}
}

// trimmed prints v with one decimal below 100 and none above, dropping a
// trailing ".0".
func trimmed(v float64) string {
	if v >= 99.95 {
		return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
	}
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}

// Percent formats a fraction as a whole percentage. Values that would round
// to 0% or 100% without being exactly that show as "<1%" and ">99%", so a
// rare event is never reported as absent.
func Percent(f float64) string {
	switch {
	case f <= 0:
		return "0%"
	case f >= 1:
		return "100%"
	case f < 0.005:
		return "<1%"
	case f > 0.995:
		return ">99%"
	default:
		return strconv.Itoa(int(math.Round(f*100))) + "%"
	}
}

// DateRange formats a span of days compactly, omitting the year from the
// start when both ends share it: "8 Mar – 4 Nov 2026".
func DateRange(from, to time.Time) string {
	if from.IsZero() || to.IsZero() {
		return ""
	}
	from, to = from.Local(), to.Local()
	if from.Year() == to.Year() {
		if from.YearDay() == to.YearDay() {
			return to.Format("2 Jan 2006")
		}
		return from.Format("2 Jan") + " – " + to.Format("2 Jan 2006")
	}
	return from.Format("2 Jan 2006") + " – " + to.Format("2 Jan 2006")
}

// Duration formats an elapsed time for humans: "640ms", "3.2s", "1m 4s".
func Duration(d time.Duration) string {
	switch {
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Minute:
		return strings.TrimSuffix(strconv.FormatFloat(d.Seconds(), 'f', 1, 64), ".0") + "s"
	default:
		d = d.Round(time.Second)
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// Seconds formats a wait given in seconds to the whole second, as people
// count one: "6s", "1m 16s". Anything shorter reads as "1s".
func Seconds(s float64) string {
	return Duration(time.Duration(max(math.Round(s), 1)) * time.Second)
}

// Plural returns "1 file" or "3 files".
func Plural(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return Int(n) + " " + plural
}

// USD formats a dollar amount: whole dollars with separators from $10 up,
// cents below.
func USD(v float64) string {
	switch {
	case v < 0:
		return "-" + USD(-v)
	case v < 10:
		return "$" + strconv.FormatFloat(v, 'f', 2, 64)
	default:
		return "$" + Int(int(math.Round(v)))
	}
}

// Words formats a large count the way people say it: "84 million",
// "24.9 billion". Counts under a million get separators.
func Words(n int64) string {
	v := float64(n)
	switch {
	case v >= 1e12:
		return trimmed(v/1e12) + " trillion"
	case v >= 1e9:
		return trimmed(v/1e9) + " billion"
	case v >= 1e6:
		return trimmed(v/1e6) + " million"
	default:
		return Int(int(n))
	}
}

// Multiple formats a ratio as "51×", with one decimal below ten.
func Multiple(f float64) string {
	if f < 10 {
		return strings.TrimSuffix(strconv.FormatFloat(f, 'f', 1, 64), ".0") + "×"
	}
	return strconv.Itoa(int(math.Round(f))) + "×"
}

// Model turns a hosted model's ID into its product name: "claude-opus-5-5"
// becomes "Opus 5.5" and "gpt-5.2-codex" becomes "GPT-5.2-Codex". An ID it
// does not recognise is returned as it is.
func Model(id string) string {
	if rest, ok := strings.CutPrefix(id, "gpt-"); ok && rest != "" {
		parts := strings.Split(rest, "-")
		for i, p := range parts {
			if p != "" && (p[0] < '0' || p[0] > '9') {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
		return "GPT-" + strings.Join(parts, "-")
	}
	name, ok := strings.CutPrefix(id, "claude-")
	if !ok {
		return id
	}
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	parts := strings.Split(name, "-")
	if len(parts) < 2 || parts[0] == "" {
		return id
	}
	family := strings.ToUpper(parts[0][:1]) + parts[0][1:]
	var version []string
	for _, p := range parts[1:] {
		if len(p) > 3 {
			break
		}
		version = append(version, p)
	}
	if len(version) == 0 || version[0] == "" {
		return id
	}
	return family + " " + strings.Join(version, ".")
}
