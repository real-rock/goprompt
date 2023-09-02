package goprompt_test

import (
	"testing"

	"github.com/real-rock/goprompt"
	"github.com/real-rock/goprompt/test"
)

func TestWordWrap(t *testing.T) {
	t.Parallel()

	text := "ab cde fgh ijklmnopq rs"
	expected := "ab cde\nfgh\nijklmno\npq\nrs"
	assertEqual(t, expected, goprompt.WordWrap(text, 7))
}

func TestHardWrap(t *testing.T) {
	t.Parallel()

	text := "ab cde fgh ijklmnopq rs"
	expected := "ab cde \nfgh ijk\nlmnopq \nrs"
	assertEqual(t, expected, goprompt.HardWrap(text, 7))
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	text := "0123456789\n0123\n0123456789\n"
	expected := "012345\n0123\n012345\n"
	assertEqual(t, expected, goprompt.Truncate(text, 6))
}

func assertEqual(tb testing.TB, expected string, got string) {
	tb.Helper()

	if expected == got {
		return
	}

	comparison := "Expected:\n%s\nGot:\n%s"
	if *test.Inspect {
		comparison = "Expected:\n%q\nGot:\n%q"
	}

	tb.Errorf("unexpected result:\n"+comparison, expected, got)
}
