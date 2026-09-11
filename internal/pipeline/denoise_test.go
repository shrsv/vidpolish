package pipeline

import "testing"

func TestDiffNewFile(t *testing.T) {
	before := map[string]bool{"audio.wav": true}
	after := map[string]bool{"audio.wav": true, "audio_DeepFilterNet3.wav": true}

	got := diffNewFile(before, after)
	if got != "audio_DeepFilterNet3.wav" {
		t.Errorf("diffNewFile = %q, want %q", got, "audio_DeepFilterNet3.wav")
	}
}

func TestDiffNewFileNoneFound(t *testing.T) {
	same := map[string]bool{"audio.wav": true}
	if got := diffNewFile(same, same); got != "" {
		t.Errorf("diffNewFile = %q, want empty string", got)
	}
}
