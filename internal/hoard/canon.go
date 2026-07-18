package hoard

// Port of scripts/catalogue_media.py from the media-library-viewer-profile
// skill. Behaviour is pinned against the Python original by the fixture
// corpus in testdata/ — change both together or not at all.

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var videoExts = map[string]bool{
	".avi": true, ".divx": true, ".flv": true, ".m4v": true, ".mkv": true,
	".mov": true, ".mp4": true, ".mpeg": true, ".mpg": true, ".ts": true,
	".webm": true, ".wmv": true,
}

var audioExts = map[string]bool{
	".aac": true, ".flac": true, ".m4a": true, ".mp3": true, ".ogg": true,
	".wav": true,
}

var genericDefaults = []string{
	"audio", "comedy", "complete", "documentaries", "documentary", "film",
	"films", "food", "media", "movies", "scifi", "sci fi",
	"storage", "torrents", "tv", "var", "video", "videos",
}

var (
	episodeMarker = regexp.MustCompile(
		`(?i)\b(?:s\d{1,2}[ ._-]*e\d{1,3}|\d{1,2}x\d{1,3}|` +
			`(?:series|season)\s*[-.:]?\s*\d+|episodes?\s*[-.:]?\s*\d+|` +
			`eps?\s*[-.:]?\s*\d+)\b`)
	dateMarker = regexp.MustCompile(
		`(?i)\b(?:19|20)\d{2}[-./]\d{1,2}[-./]\d{1,2}\b|` +
			`\b\d{1,2}(?:st|nd|rd|th)?\s+` +
			`(?:jan\w*|feb\w*|mar\w*|apr\w*|may|jun\w*|jul\w*|aug\w*|` +
			`sep\w*|oct\w*|nov\w*|dec\w*)\s+(?:19|20)\d{2}\b`)
	techMarker = regexp.MustCompile(
		`(?i)\b(?:2160p|1080p|720p|576p|540p|504p|480p|360p|352p|` +
			`uhd|fhd|hdtv|pdtv|webrip|web[- .]?dl|dvdrip|brrip|bluray|vhsrip|` +
			`x26[45]|h[ ._]?26[45]|xvid|divx|avc|hevc|aac|ac3|proper|repack|` +
			`remux|subs?|subbed|complete)\b`)
	storageComponent = regexp.MustCompile(
		`(?i)^(?:sd[a-z]\d*(?:[-_].*)?|.*scsi.*|mnt|volumes?|home|users?)$`)

	// Python uses a lookahead (`\.(?=[A-Za-z])`); RE2 has none, so capture
	// and restore the letter instead.
	dotBeforeLetter  = regexp.MustCompile(`\.([A-Za-z])`)
	bracketYear      = regexp.MustCompile(`\s*[\[(](?:19|20)\d{2}(?:\s*[-–]\s*\d{2,4})?[^\])]*[\])]`)
	ripTagBracket    = regexp.MustCompile(`(?i)\s*\[[^\]]*(?:rip|xvid|divx|\d+p\b|subs?|web|hdtv|pdtv)[^\]]*\]`)
	partDisc         = regexp.MustCompile(`(?i)\b(?:part|pt|disc|cd)\s*\d+\b.*$`)
	leadingEpisode   = regexp.MustCompile(`(?i)^\s*(?:episode|ep|part)\s*\d+\s*[-.:]?\s*`)
	leadingNumberSep = regexp.MustCompile(`(?i)^\s*\d{1,4}\s*[-.:]\s*`)
	episodeKey       = regexp.MustCompile(`^(?:episode|ep|part|disc|cd)\s*\d+`)
	shortCode        = regexp.MustCompile(`(?i)^(?:[a-z]{1,8}\d+|\d+[a-z]?)$`)
	nonAlnum         = regexp.MustCompile(`[^a-z0-9]+`)
	whitespaceRun    = regexp.MustCompile(`\s+`)
)

func defaultGenericSet(extra []string) map[string]bool {
	generic := make(map[string]bool, len(genericDefaults)+len(extra))
	for _, item := range genericDefaults {
		generic[normKey(item)] = true
	}
	for _, item := range extra {
		generic[normKey(item)] = true
	}
	return generic
}

func normKey(text string) string {
	folded := norm.NFKD.String(text)
	var b strings.Builder
	for _, r := range folded {
		if r < 128 {
			b.WriteRune(r)
		}
	}
	value := strings.ToLower(b.String())
	value = strings.ReplaceAll(value, "&", " and ")
	value = nonAlnum.ReplaceAllString(value, " ")
	value = whitespaceRun.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func cleanTitle(text string) string {
	value := norm.NFKC.String(text)
	value = strings.ReplaceAll(value, "_", " ")
	value = dotBeforeLetter.ReplaceAllString(value, " $1")
	value = dateMarker.ReplaceAllString(value, " ")

	if loc := episodeMarker.FindStringIndex(value); loc != nil && runeIndex(value, loc[0]) > 1 {
		value = value[:loc[0]]
	}

	value = bracketYear.ReplaceAllString(value, " ")
	value = ripTagBracket.ReplaceAllString(value, " ")

	if loc := techMarker.FindStringIndex(value); loc != nil && runeIndex(value, loc[0]) > 2 {
		value = value[:loc[0]]
	}

	value = partDisc.ReplaceAllString(value, " ")
	value = leadingEpisode.ReplaceAllString(value, " ")
	value = leadingNumberSep.ReplaceAllString(value, " ")
	value = whitespaceRun.ReplaceAllString(value, " ")
	return strings.Trim(value, " ._-–[](){}:")
}

func genericTitle(text string, generic map[string]bool) bool {
	key := normKey(text)
	return key == "" ||
		generic[key] ||
		allDigits(key) ||
		len(key) < 3 ||
		storageComponent.MatchString(key) ||
		episodeKey.MatchString(key)
}

func candidateFromPath(rawPath string, generic map[string]bool) string {
	dirs, base := splitDirsAndBase(rawPath)
	stem, _ := stemAndExt(base)
	stem = cleanTitle(stem)

	parent := ""
	for i := len(dirs) - 1; i >= 0; i-- {
		candidate := cleanTitle(dirs[i])
		if !genericTitle(candidate, generic) {
			parent = candidate
			break
		}
	}

	if genericTitle(stem, generic) {
		return parent
	}

	stemKey := normKey(stem)
	parentKey := normKey(parent)
	if parent != "" && (strings.HasPrefix(stemKey, parentKey) || strings.HasPrefix(parentKey, stemKey)) {
		if len(parentKey) <= len(stemKey) {
			return parent
		}
		return stem
	}

	if parent != "" && shortCode.MatchString(stem) {
		return parent
	}

	if stem != "" {
		return stem
	}
	return parent
}

// splitDirsAndBase splits a Unix-style path (as produced by find) into its
// directory components and final element.
func splitDirsAndBase(rawPath string) ([]string, string) {
	trimmed := strings.TrimRight(rawPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return nil, ""
	}
	return parts[:len(parts)-1], parts[len(parts)-1]
}

// stemAndExt mirrors Python pathlib: a leading dot is not a suffix, and the
// returned extension is lower-cased with the dot retained.
func stemAndExt(name string) (string, string) {
	i := strings.LastIndex(name, ".")
	if i <= 0 || i == len(name)-1 {
		return name, ""
	}
	return name[:i], strings.ToLower(name[i:])
}

func mediaTypeForExt(ext string) string {
	switch {
	case videoExts[ext]:
		return "video"
	case audioExts[ext]:
		return "audio"
	default:
		return "other"
	}
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func runeIndex(s string, byteIdx int) int {
	return utf8.RuneCountInString(s[:byteIdx])
}
