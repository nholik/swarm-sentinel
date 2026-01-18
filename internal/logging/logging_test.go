package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestNew_ReturnsInfoLevelLogger(t *testing.T) {
	logger := New()

	// New() should default to info level
	if logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("New() level = %v, want %v", logger.GetLevel(), zerolog.InfoLevel)
	}
}

func TestNewWithLevel_ValidLevels(t *testing.T) {
	tests := []struct {
		input string
		want  zerolog.Level
	}{
		{"trace", zerolog.TraceLevel},
		{"TRACE", zerolog.TraceLevel},
		{"debug", zerolog.DebugLevel},
		{"DEBUG", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"INFO", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"WARN", zerolog.WarnLevel},
		{"warning", zerolog.WarnLevel},
		{"WARNING", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"ERROR", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"FATAL", zerolog.FatalLevel},
		{"panic", zerolog.PanicLevel},
		{"PANIC", zerolog.PanicLevel},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			logger := NewWithLevel(tt.input)
			if logger.GetLevel() != tt.want {
				t.Errorf("NewWithLevel(%q) level = %v, want %v", tt.input, logger.GetLevel(), tt.want)
			}
		})
	}
}

func TestNewWithLevel_InvalidLevelDefaultsToInfo(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"verbose",
		"critical",
		"123",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			logger := NewWithLevel(input)
			if logger.GetLevel() != zerolog.InfoLevel {
				t.Errorf("NewWithLevel(%q) level = %v, want %v (default)", input, logger.GetLevel(), zerolog.InfoLevel)
			}
		})
	}
}

func TestNewWithLevel_TrimsWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  zerolog.Level
	}{
		{"  debug  ", zerolog.DebugLevel},
		{"\twarn\n", zerolog.WarnLevel},
		{" error ", zerolog.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			logger := NewWithLevel(tt.input)
			if logger.GetLevel() != tt.want {
				t.Errorf("NewWithLevel(%q) level = %v, want %v", tt.input, logger.GetLevel(), tt.want)
			}
		})
	}
}

func TestParseLevel_MixedCase(t *testing.T) {
	// Mixed case should work
	logger := NewWithLevel("DeBuG")
	if logger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("Mixed case 'DeBuG' should parse to debug level")
	}
}
