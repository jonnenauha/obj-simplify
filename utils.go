package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// strings

type caseSensitivity int

const (
	caseSensitive   caseSensitivity = 0
	caseInsensitive caseSensitivity = 1
)

func strIndexOf(str1, str2 string, from int, cs caseSensitivity) int {
	if from >= len(str1) {
		return -1
	}
	src := str1
	if from < 0 {
		from = 0
	} else if from > 0 {
		src = src[from:]
	}
	i := -1
	if cs == caseSensitive {
		i = strings.Index(src, str2)
	} else {
		i = strings.Index(strings.ToLower(src), strings.ToLower(str2))
	}
	if i >= 0 {
		i += from
	}
	return i
}

func strStartsWith(str, prefix string, cs caseSensitivity) bool {
	if cs == caseSensitive {
		return strings.HasPrefix(str, prefix)
	}
	return strings.HasPrefix(strings.ToLower(str), strings.ToLower(prefix))
}

func strEndsWith(str, postfix string, cs caseSensitivity) bool {
	if cs == caseSensitive {
		return strings.HasSuffix(str, postfix)
	}
	return strings.HasSuffix(strings.ToLower(str), strings.ToLower(postfix))
}

func strContains(str, part string, cs caseSensitivity) bool {
	if cs == caseSensitive {
		return strings.Contains(str, part)
	}
	return strings.Contains(strings.ToLower(str), strings.ToLower(part))
}

func strContainsAny(str string, parts []string, cs caseSensitivity) bool {
	for _, part := range parts {
		if strContains(str, part, cs) {
			return true
		}
	}
	return false
}

func substring(str string, i int, iEnd int) string {
	strLen := len(str)
	if i < 0 {
		i = 0
	}
	if i >= strLen {
		return str
	} else if iEnd < 0 || iEnd >= strLen {
		return str[i:]
	}
	return str[i:iEnd]
}

func substringBefore(str, sep string, includeSeparator bool, cs caseSensitivity) string {
	i := strIndexOf(str, sep, -1, cs)
	if i < 0 {
		return str
	}
	if includeSeparator {
		i += len(sep)
	}
	return substring(str, 0, i)
}

// files

func cleanPath(path string) string {
	if len(path) == 0 {
		return path
	}
	path, err := filepath.Abs(path)
	logFatalError(err)
	return filepath.ToSlash(filepath.Clean(path))
}

func fileExists(path string) bool {
	if len(path) == 0 {
		return false
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

func fileBasename(path string) string {
	p := cleanPath(path)
	if p[len(p)-1] == '/' {
		p = p[0 : len(p)-1]
	}
	if i := strings.LastIndex(p, "/"); i != -1 {
		p = p[i+1:]
	}
	if i := strings.LastIndex(p, "."); i != -1 {
		p = p[0:i]
	}
	return p
}

// The returned ext is always lower-cased and contains a prefix "." dot (e.g. ".png")
func fileExtension(path string) string {
	// Strip query from URLs http(s)://domain.com/path/to/my.jpg?id=123312
	if strStartsWith(path, "http", caseInsensitive) && strContains(path, "?", caseSensitive) {
		path = substringBefore(path, "?", false, caseSensitive)
	}
	return strings.ToLower(filepath.Ext(path))
}

func fileSize(path string) int64 {
	if len(path) == 0 {
		return 0
	}
	if fi, err := os.Stat(path); os.IsNotExist(err) {
		return 0
	} else {
		return fi.Size()
	}
}

// formatting

func formatInt(num int) string {
	str := intToString(num)
	for i := len(str) - 1; i > 2; i -= 3 {
		str = str[0:i-2] + " " + str[i-2:]
	}
	return str
}

func formatUInt(num uint) string {
	str := uintToString(num)
	for i := len(str) - 1; i > 2; i -= 3 {
		str = str[0:i-2] + " " + str[i-2:]
	}
	return str
}

func intToString(num int) string {
	return strconv.Itoa(num)
}

func uintToString(num uint) string {
	return strconv.FormatUint(uint64(num), 10)
}

func formatFloat32(f float32, decimals int) string {
	return strconv.FormatFloat(float64(f), 'f', decimals, 32)
}

func formatFloat64(f float64, decimals int) string {
	return strconv.FormatFloat(f, 'f', decimals, 64)
}

func formatBytes(numBytes int64) string {
	prefix := ""
	numAbs := numBytes
	if numBytes < 0 {
		prefix = "-"
		numAbs = -numBytes
	}
	if numAbs >= 1024 {
		if numAbs >= 1024*1024 {
			if numAbs >= 1024*1024*1024 {
				return fmt.Sprintf("%s%.*f GB", prefix, 2, (float32(numAbs)/1024.0)/1024.0/1024.0)
			}
			return fmt.Sprintf("%s%.*f MB", prefix, 2, (float32(numAbs)/1024.0)/1024.0)
		}
		return fmt.Sprintf("%s%.*f kB", prefix, 2, float32(numAbs)/1024.0)
	}
	return fmt.Sprintf("%s%d B", prefix, numAbs)
}

func formatDurationSince(t time.Time) string {
	return formatDuration(time.Since(t))
}

func formatDuration(d time.Duration) (duration string) {
	if d.Minutes() < 1.0 {
		// sec
		duration = fmt.Sprintf("%ss", strconv.FormatFloat(d.Seconds(), 'f', 2, 64))
	} else if d.Minutes() < 60.0 {
		// min sec
		s := math.Mod(d.Seconds(), 60.0)
		duration = fmt.Sprintf("%dm %ss", int(math.Floor(d.Minutes())),
			strconv.FormatFloat(s, 'f', 2, 64))
	} else {
		s := math.Mod(d.Seconds(), 60.0)
		m := math.Mod(d.Minutes(), 60.0)
		if d.Hours() < 24.0 {
			// hour min sec
			duration = fmt.Sprintf("%dh %dm %ss", int(math.Floor(d.Hours())),
				int(math.Floor(m)), strconv.FormatFloat(s, 'f', 2, 64))
		} else {
			h := math.Mod(d.Hours(), 24.0)
			days := d.Hours() / 24.0
			if days < 7.0 {
				// day hour min sec
				duration = fmt.Sprintf("%dd %dh %dm %ss", int(math.Floor(days)), int(math.Floor(h)),
					int(math.Floor(m)), strconv.FormatFloat(s, 'f', 2, 64))
			} else {
				// week day hour min sec
				w := math.Floor(days / 7.0)
				days := math.Mod(days, 7.0)
				duration = fmt.Sprintf("%dw %dd %dh %dm %ss", int(w), int(math.Floor(days)),
					int(math.Floor(h)), int(math.Floor(m)), strconv.FormatFloat(s, 'f', 2, 64))
			}
		}
	}
	return
}
