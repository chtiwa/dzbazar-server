package utils

import "strings"

// bannedWords are common Algerian darija insults/cusswords, both latin-script
// transliterations and Arabic script, used to silently flag spam/troll orders
// where the client typed an insult into the fullName field instead of their name.
var bannedWords = []string{
	"fuck", "dick", "bitch", "ass", "nik", "nikomok", "nikmok", "nikbouk", "kahba", "9ahba", "9a7ba",
	"zebi", "zbi", "zamel", "zeml", "khanzir", "hmar", "7mar", "qahba",
	"chermoula", "twassekh", "wesekh", "wsekh", "hchouma", "kelb", "9elb",
	"ya kelb", "yal kelb", "ya khanzir", "tfou", "ya zebi", "mokrez",

	// Arabic script
	"نيك", "نيكك", "نيكمك", "قحبة", "قحبه", "كحبة", "كحبه", "زبي", "زملة",
	"خنزير", "حمار", "كلب", "تفو", "حشومة", "وسخ", "منيك", "قواد",
}

// ContainsBannedWords reports whether s contains any known cussword,
// case-insensitively and ignoring spaces (e.g. "n i k").
func ContainsBannedWords(s string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(s, " ", ""))
	for _, word := range bannedWords {
		if strings.Contains(normalized, word) {
			return true
		}
	}
	return false
}
