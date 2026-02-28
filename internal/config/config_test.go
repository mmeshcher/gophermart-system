package config

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	type want struct {
		runAddress          string
		databaseURI         string
		accrualSystemAdress string
	}

	tests := []struct {
		name  string
		env   map[string]string
		flags []string
		want  want
	}{
		{
			name:  "defaults",
			env:   map[string]string{},
			flags: []string{},
			want: want{
				runAddress: "localhost:8080",
			},
		},
		{
			name: "env only",
			env: map[string]string{
				"RUN_ADDRESS":            "localhost:9999",
				"DATABASE_URI":           "postgres://user:pass@localhost/db",
				"ACCRUAL_SYSTEM_ADDRESS": "localhost:8081",
			},
			flags: []string{},
			want: want{
				runAddress:          "localhost:9999",
				databaseURI:         "postgres://user:pass@localhost/db",
				accrualSystemAdress: "localhost:8081",
			},
		},
		{
			name: "flags only",
			env:  map[string]string{},
			flags: []string{
				"-a", "localhost:7777",
				"-d", "postgres://flag:flag@localhost/flagdb",
				"-r", "accrual:8080",
			},
			want: want{
				runAddress:          "localhost:7777",
				databaseURI:         "postgres://flag:flag@localhost/flagdb",
				accrualSystemAdress: "accrual:8080",
			},
		},
		{
			name: "env overrides flags",
			env: map[string]string{
				"RUN_ADDRESS":            "env:9000",
				"DATABASE_URI":           "postgres://env:env@localhost/envdb",
				"ACCRUAL_SYSTEM_ADDRESS": "env-accrual:8081",
			},
			flags: []string{
				"-a", "flag:8000",
				"-d", "postgres://flag:flag@localhost/flagdb",
				"-r", "flag-accrual:8080",
			},
			want: want{
				runAddress:          "env:9000",
				databaseURI:         "postgres://env:env@localhost/envdb",
				accrualSystemAdress: "env-accrual:8081",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			os.Args = append([]string{"test"}, tt.flags...)

			cfg, err := Parse()
			require.NoError(t, err)

			assert.Equal(t, tt.want.runAddress, cfg.RunAddress)
			assert.Equal(t, tt.want.databaseURI, cfg.DatabaseURI)
			assert.Equal(t, tt.want.accrualSystemAdress, cfg.AccrualSystemAddress)
		})
	}
}
