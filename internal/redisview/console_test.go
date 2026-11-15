package redisview

import (
	"reflect"
	"testing"
)

func TestParseCommand(t *testing.T) {
	for line, want := range map[string][]string{
		`GET a`:                {"GET", "a"},
		`  SET   k   v  `:      {"SET", "k", "v"},
		`SET "a b" 'c d'`:      {"SET", "a b", "c d"},
		`SET k "say \"hi\"\n"`: {"SET", "k", "say \"hi\"\n"},
		`SET k ""`:             {"SET", "k", ""},
		`HSET h f "x y" g z`:   {"HSET", "h", "f", "x y", "g", "z"},
	} {
		got, err := ParseCommand(line)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: got %q %v, want %q", line, got, err, want)
		}
	}
	if _, err := ParseCommand(`GET "oops`); err == nil {
		t.Fatal("unterminated quote accepted")
	}
}

func TestAllowed(t *testing.T) {
	for _, c := range []struct {
		args     []string
		readOnly bool
		ok       bool
	}{
		{[]string{"GET", "a"}, true, true}, {[]string{"DEL", "a"}, true, false}, {[]string{"DEL", "a"}, false, true},
		{[]string{"CONFIG", "GET", "x"}, true, true}, {[]string{"CONFIG", "SET", "x", "y"}, true, false},
		{[]string{"CONFIG", "SET", "x", "y"}, false, true}, {[]string{"MONITOR"}, false, false},
		{[]string{"client", "reply", "off"}, false, false}, {[]string{"CLIENT", "LIST"}, true, true},
		{[]string{"select", "1"}, false, false}, {[]string{}, false, false},
	} {
		if got := Allowed(c.args, c.readOnly) == nil; got != c.ok {
			t.Fatalf("%v readOnly=%v: allowed=%v want %v", c.args, c.readOnly, got, c.ok)
		}
	}
}
