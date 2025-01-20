package inserter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTruncateToStartOfDayUTC(t *testing.T) {
	tests := []struct {
		input    time.Time
		expected time.Time
	}{
		{
			input:    time.Date(2023, time.July, 12, 15, 45, 30, 123456789, time.UTC),
			expected: time.Date(2023, time.July, 12, 0, 0, 0, 0, time.UTC),
		},
		{
			input:    time.Date(2023, time.July, 12, 15, 45, 30, 123456789, time.FixedZone("UTC+2", 2*3600)),
			expected: time.Date(2023, time.July, 12, 0, 0, 0, 0, time.UTC),
		},
		{
			input:    time.Date(2023, time.July, 12, 15, 45, 30, 123456789, time.FixedZone("UTC-5", -5*3600)),
			expected: time.Date(2023, time.July, 12, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		output := TruncateToStartOfDayUTC(test.input)
		assert.True(t, output.Equal(test.expected), "For input time %v, expected %v but got %v", test.input, test.expected, output)
	}
}

func TestTruncateToStartOfMinuteUTC(t *testing.T) {
	tests := []struct {
		input    time.Time
		expected time.Time
	}{
		{
			input:    time.Date(2023, time.July, 12, 15, 45, 30, 123456789, time.UTC),
			expected: time.Date(2023, time.July, 12, 15, 45, 0, 0, time.UTC),
		},
		{
			input:    time.Date(2023, time.July, 12, 15, 45, 30, 123456789, time.FixedZone("UTC+2", 2*3600)),
			expected: time.Date(2023, time.July, 12, 15, 45, 0, 0, time.FixedZone("UTC+2", 2*3600)),
		},
		{
			input:    time.Date(2023, time.July, 12, 15, 45, 30, 123456789, time.FixedZone("UTC-5", -5*3600)),
			expected: time.Date(2023, time.July, 12, 15, 45, 0, 0, time.FixedZone("UTC-5", -5*3600)),
		},
	}

	for _, test := range tests {
		output := TruncateToStartOfMinuteUTC(test.input)
		assert.True(t, output.Equal(test.expected), "For input time %v, expected %v but got %v", test.input, test.expected, output)
	}
}
