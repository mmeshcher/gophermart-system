package validation

import "testing"

func TestIsValidOrderNumber(t *testing.T) {
	tests := []struct {
		name   string
		number string
		valid  bool
	}{
		{
			name:   "valid example 1",
			number: "79927398713",
			valid:  true,
		},
		{
			name:   "valid example 2",
			number: "4539578763621486",
			valid:  true,
		},
		{
			name:   "invalid checksum",
			number: "79927398710",
			valid:  false,
		},
		{
			name:   "contains letters",
			number: "1234a67890",
			valid:  false,
		},
		{
			name:   "empty string",
			number: "",
			valid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidOrderNumber(tt.number)
			if got != tt.valid {
				t.Fatalf("IsValidOrderNumber(%q) = %v, want %v", tt.number, got, tt.valid)
			}
		})
	}
}

