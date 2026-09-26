package compute

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseNameServersFiltersAndBoundsResolverInput(t *testing.T) {
	input := strings.NewReader("# generated\nsearch internal.example\nnameserver not-an-ip\nnameserver 10.0.0.2\nnameserver 2001:db8::53\nnameserver 1.1.1.1\nnameserver 8.8.8.8\n")
	want := []string{"10.0.0.2", "2001:db8::53", "1.1.1.1"}
	if got := parseNameServers(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNameServers() = %v, want %v", got, want)
	}
}
