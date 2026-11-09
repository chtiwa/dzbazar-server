package utils

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// bannedWords are common Algerian darija insults/cusswords, both latin-script
// transliterations and Arabic script, used to flag spam/troll orders where the
// client typed an insult into the fullName field instead of their name.
//
// A false positive here silently costs a real sale, so matching is by WHOLE
// WORD (see ContainsBannedWords). Never add a root that is also a common name
// fragment or an everyday word ("9elb" = heart, "ass", "nik"…).
var bannedWords = []string{
	"fuck", "dick", "bitch", "nikomok", "nikmok", "nikbouk", "kahba", "9ahba", "9a7ba",
	"zebi", "zamel", "zeml", "khanzir", "hmar", "7mar", "qahba",
	"chermoula", "twassekh", "wesekh", "wsekh", "hchouma", "kelb",
	"tfou", "mokrez",

	// Arabic script
	"نيك", "نيكك", "نيكمك", "قحبة", "قحبه", "كحبة", "كحبه", "زملة",
	"خنزير", "حمار", "كلب", "تفو", "حشومة", "وسخ", "منيك", "قواد",
}

// minJoinedLen: words at least this long are also matched with the spaces
// removed ("ni k omok"). Shorter ones only match as a whole word, otherwise
// "Rahma Rida" → "rahmarida" would hit "hmar".
const minJoinedLen = 6

// normalize lowercases, drops Arabic diacritics/tatweel, and turns every
// non-letter/digit into a space.
func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsMark(r) || r == 'ـ':
			return -1
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			return unicode.ToLower(r)
		default:
			return ' '
		}
	}, s)
}

// ContainsBannedWords reports whether s contains a known cussword as a whole
// word (case-insensitive), so real names like Nassim, Monika, Kelbi, Rahma Rida
// or الكلبي are never flagged.
func ContainsBannedWords(s string) bool {
	words := strings.Fields(normalize(s))
	joined := strings.Join(words, "")
	for _, bad := range bannedWords {
		for _, w := range words {
			if w == bad {
				return true
			}
		}
		if utf8.RuneCountInString(bad) >= minJoinedLen && strings.Contains(joined, bad) {
			return true
		}
	}
	return false
}
