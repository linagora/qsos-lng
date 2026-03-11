package sources

import (
	"testing"
)

func TestParseDebtResponse(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		input := `{"score": 3.5, "key_points": ["Good naming", "Some long functions"]}`
		score, points, err := parseDebtResponse(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if score != 3.5 {
			t.Errorf("want score 3.5, got %f", score)
		}
		if len(points) != 2 {
			t.Errorf("want 2 key_points, got %d", len(points))
		}
	})

	t.Run("score clamped to max", func(t *testing.T) {
		input := `{"score": 7.0, "key_points": ["Clean code"]}`
		score, _, err := parseDebtResponse(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if score != 5.0 {
			t.Errorf("want score clamped to 5.0, got %f", score)
		}
	})

	t.Run("score clamped to min", func(t *testing.T) {
		input := `{"score": 0.0, "key_points": ["Very bad"]}`
		score, _, err := parseDebtResponse(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if score != 1.0 {
			t.Errorf("want score clamped to 1.0, got %f", score)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, _, err := parseDebtResponse("not json")
		if err == nil {
			t.Error("want error for malformed JSON, got nil")
		}
	})

	t.Run("empty key_points", func(t *testing.T) {
		input := `{"score": 4.0, "key_points": []}`
		_, _, err := parseDebtResponse(input)
		if err == nil {
			t.Error("want error for empty key_points, got nil")
		}
	})

	t.Run("LLM wraps JSON in markdown fences", func(t *testing.T) {
		input := "```json\n{\"score\": 3.0, \"key_points\": [\"Good\"]}\n```"
		score, _, err := parseDebtResponse(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if score != 3.0 {
			t.Errorf("want 3.0, got %f", score)
		}
	})
}

func TestParseFileSelection(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		input := `["src/main.go", "pkg/core/engine.go"]`
		paths, err := parseFileSelection(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 2 {
			t.Errorf("want 2 paths, got %d", len(paths))
		}
	})

	t.Run("empty array", func(t *testing.T) {
		_, err := parseFileSelection(`[]`)
		if err == nil {
			t.Error("want error for empty array, got nil")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, err := parseFileSelection("not json")
		if err == nil {
			t.Error("want error for malformed JSON, got nil")
		}
	})

	t.Run("LLM wraps JSON in markdown fences", func(t *testing.T) {
		input := "```\n[\"src/main.go\"]\n```"
		paths, err := parseFileSelection(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 1 {
			t.Errorf("want 1 path, got %d", len(paths))
		}
	})
}

func TestTruncateLines(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		input := "line1\nline2\nline3"
		result := truncateLines(input, 10)
		if result != input {
			t.Errorf("want unchanged, got %q", result)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		input := "a\nb\nc\nd\ne"
		result := truncateLines(input, 3)
		want := "a\nb\nc"
		if result != want {
			t.Errorf("want %q, got %q", want, result)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		input := "a\nb\nc"
		result := truncateLines(input, 3)
		if result != input {
			t.Errorf("want unchanged, got %q", result)
		}
	})
}
