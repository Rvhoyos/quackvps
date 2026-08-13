package minecraft

import (
	"reflect"
	"testing"
)

func TestFailureReasons(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want []string
	}{
		{
			name: "neoforge missing mod, the real cobblemon case",
			log:  "some earlier line\nCurrently, citresewn is not installed\n\tat net.neoforged.fml.ModLoader...",
			want: []string{"Currently, citresewn is not installed"},
		},
		{
			name: "lists every missing dependency, not just the first",
			log:  "Currently, architectury is not installed\nnoise\nCurrently, geckolib is not installed",
			want: []string{"Currently, architectury is not installed", "Currently, geckolib is not installed"},
		},
		{
			name: "de-duplicates repeated lines",
			log:  "Currently, foo is not installed\nCurrently, foo is not installed",
			want: []string{"Currently, foo is not installed"},
		},
		{
			name: "falls back to Caused by when no mod-load message",
			log:  "big stack trace\nCaused by: java.lang.IllegalStateException: boom\n\tat x",
			want: []string{"Caused by: java.lang.IllegalStateException: boom"},
		},
		{
			name: "strips pipes so it survives a markdown cell",
			log:  "Incompatible mod set: a | b | c",
			want: []string{"Incompatible mod set: a / b / c"},
		},
		{
			name: "ignores an unrelated 'requires' info line",
			log:  "This modpack requires Java 21 to run\njust noise",
			want: nil,
		},
	}
	for _, tt := range tests {
		if got := FailureReasons(tt.log); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: FailureReasons = %#v, want %#v", tt.name, got, tt.want)
		}
	}
}
