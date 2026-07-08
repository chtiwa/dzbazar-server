package utils

import "testing"

func TestContainsBannedWords(t *testing.T) {
	if !ContainsBannedWords("ya KAHBA mohamed") {
		t.Fatal("expected banned word to be detected")
	}
	if !ContainsBannedWords("ni k omok") {
		t.Fatal("expected banned word to be detected across spaces")
	}
	if ContainsBannedWords("Mohamed Benali") {
		t.Fatal("expected clean name to pass")
	}
}
