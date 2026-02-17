package main

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"HomeNet", "homenet"},
		{"Guest Network", "guest-network"},
		{"Default", "default"},
		{"a--b", "a-b"},
		{"IoT & Sensors", "iot-sensors"},
		{"Acme", "acme"},
		{"Guest WiFi", "guest-wifi"},
		{"  leading trailing  ", "leading-trailing"},
		{"café", "café"},
		{"Ünifi Nëtwork", "ünifi-nëtwork"},
		{"", ""},
		{"---", ""},
		{"!!!@@@", ""},
		{"hello   world", "hello-world"},
		{"A-B-C", "a-b-c"},
		{"123 test", "123-test"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
