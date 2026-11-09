package utils

import "testing"

func TestContainsBannedWords(t *testing.T) {
	for _, n := range []string{"ya KAHBA mohamed", "ni k omok", "9ahba", "ya zebi", "يا كلب", "قحبة", "كَلب"} {
		if !ContainsBannedWords(n) {
			t.Fatalf("expected %q to be flagged", n)
		}
	}
	// Real customers must never be flagged — a false positive loses the sale.
	for _, n := range []string{
		"Nassim Adrouche", "Yassine Hassan", "Monika Zbir", "زبير",
		"Rahma Rida", "Sarah Marie", "Abdelkelb Benali", "Ahmed Kelbi", "Nikolas Dickens",
		"الكلبي", "نيكولا", "قوادري", "Mohamed Benali", "Fatima Zohra Mar",
	} {
		if ContainsBannedWords(n) {
			t.Fatalf("false positive on real name %q", n)
		}
	}
}
