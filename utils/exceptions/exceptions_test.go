package exceptions

import (
	"testing"
)

func TestMatchAll(t *testing.T) {
	tests := []struct {
		name       string
		image      string
		base       string
		exceptions []Exception
		want       bool
	}{
		{
			name:  "success",
			image: "ubuntu-2204-amd64",
			base:  ImageUbuntu,
			exceptions: []Exception{
				Exception{Version: 2204, Type: Equal},
			},
			want: true,
		},
		{
			name:  "multiple-exceptions-success",
			image: "windows-2019",
			base:  ImageWindows,
			exceptions: []Exception{
				Exception{Version: 2022, Type: NotEqual},
				Exception{Version: 2008, Type: GreaterThan},
			},
			want: true,
		},
		{
			name:  "multiple-exceptions-failures",
			image: "windows-2022",
			base:  ImageWindows,
			exceptions: []Exception{
				Exception{Version: 2022, Type: NotEqual},
				Exception{Version: 2008, Type: GreaterThanOrEqualTo},
			},
			want: false,
		},
		{
			name:       "no-exceptions-success",
			image:      "windows-2019",
			base:       ImageWindows,
			exceptions: []Exception{},
			want:       true,
		},
		{
			name:       "no-exceptions-failure",
			image:      "debian-11",
			base:       ImageEL,
			exceptions: []Exception{},
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MatchAll(test.image, test.base, test.exceptions...)
			if got != test.want {
				t.Errorf("MatchAll(%q, %q, %v) = %v, want %v", test.image, test.base, test.exceptions, got, test.want)
			}
		})
	}
}

func TestHasMatch(t *testing.T) {
	tests := []struct {
		name       string
		image      string
		exceptions []Exception
		want       bool
	}{
		{
			name:  "success",
			image: "ubuntu-2204-amd64",
			exceptions: []Exception{
				Exception{Match: ImageUbuntu, Version: 2204, Type: Equal},
			},
			want: true,
		},
		{
			name:  "multiple-exceptions-success",
			image: "windows-2019",
			exceptions: []Exception{
				Exception{Match: ImageWindows, Version: 2019, Type: Equal},
				Exception{Match: ImageEL, Version: 7, Type: GreaterThan},
			},
			want: true,
		},
		{
			name:  "multiple-exceptions-failures",
			image: "windows-2022",
			exceptions: []Exception{
				Exception{Match: ImageWindows, Version: 2022, Type: NotEqual},
				Exception{Match: ImageWindows, Version: 2019, Type: LessThan},
			},
			want: false,
		},
		{
			name:       "no-exceptions",
			image:      "windows-2019",
			exceptions: []Exception{},
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := HasMatch(test.image, test.exceptions)
			if got != test.want {
				t.Errorf("HasMatch(%q, %v) = %v, want %v", test.image, test.exceptions, got, test.want)
			}
		})
	}
}
