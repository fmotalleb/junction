package router

import "testing"

func TestPrepareTargetHost(t *testing.T) {
	tests := []struct {
		name       string
		hostHeader string
		targetPort string
		want       string
		wantErr    bool
	}{
		{
			name:       "simple hostname no port",
			hostHeader: "example.com",
			targetPort: "",
			want:       "example.com",
			wantErr:    false,
		},
		{
			name:       "hostname with target port",
			hostHeader: "example.com",
			targetPort: "8080",
			want:       "example.com:8080",
			wantErr:    false,
		},
		{
			name:       "host header contains scheme",
			hostHeader: "http://example.com",
			targetPort: "",
			want:       "example.com",
			wantErr:    false,
		},
		{
			name:       "host header contains scheme and port",
			hostHeader: "https://example.com:8443",
			targetPort: "",
			want:       "example.com",
			wantErr:    false,
		},
		{
			name:       "host header contains host:port",
			hostHeader: "example.com:1234",
			targetPort: "",
			want:       "example.com",
			wantErr:    false,
		},
		{
			name:       "empty host header",
			hostHeader: "",
			targetPort: "",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid hostname",
			hostHeader: "bad_host_name",
			targetPort: "",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid target port non-numeric",
			hostHeader: "example.com",
			targetPort: "abc",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid target port out of range",
			hostHeader: "example.com",
			targetPort: "70000",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid URL in host header",
			hostHeader: "http://[::1",
			targetPort: "",
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareTargetHost(tt.hostHeader, tt.targetPort)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkPrepareTargetHost(b *testing.B) {
	type benchCase struct {
		hostHeader string
		targetPort string
	}

	cases := []benchCase{
		{"example.com", ""},
		{"example.com", "8080"},
		{"http://example.com", ""},
		{"https://example.com:443", ""},
		{"example.com:1234", ""},
		{"bad_host_name", ""},
	}

	for _, bc := range cases {
		b.Run(bc.hostHeader+"_"+bc.targetPort, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = prepareTargetHost(bc.hostHeader, bc.targetPort)
			}
		})
	}
}
