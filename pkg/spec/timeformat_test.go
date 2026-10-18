package spec_test

import (
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var (
	morning   = time.Date(2026, time.March, 5, 7, 8, 9, 45_000_000, time.UTC)
	afternoon = time.Date(2026, time.December, 25, 15, 4, 5, 6_000_000, time.UTC)
	midnight  = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
)

func TestFormatTimeMacro(t *testing.T) {
	cases := []struct {
		name   string
		at     time.Time
		format string
		want   string
	}{
		{"default is epoch milliseconds", morning, "", "1772694489045"},
		{"date", morning, "date", "2026-03-05T07:08:09.045Z"},
		{"iso", morning, "date:iso", "2026-03-05T07:08:09.045Z"},
		{"seconds", morning, "date:seconds", "1772694489"},
		{"seconds floor", morning.Add(954 * time.Millisecond), "date:seconds", "1772694489"},
		{"other zones are rendered in UTC", morning.In(time.FixedZone("IST", 5*3600+1800)), "date", "2026-03-05T07:08:09.045Z"},
		{"YYYY", morning, "date:YYYY", "2026"},
		{"YY", morning, "date:YY", "26"},
		{"MMMM", morning, "date:MMMM", "March"},
		{"MMM", morning, "date:MMM", "Mar"},
		{"MM", morning, "date:MM", "03"},
		{"M", morning, "date:M", "3"},
		{"DD", morning, "date:DD", "05"},
		{"D", morning, "date:D", "5"},
		{"dddd", morning, "date:dddd", "Thursday"},
		{"ddd", morning, "date:ddd", "Thu"},
		{"HH", morning, "date:HH", "07"},
		{"H", morning, "date:H", "7"},
		{"hh in the afternoon", afternoon, "date:hh", "03"},
		{"h in the afternoon", afternoon, "date:h", "3"},
		{"hh at midnight", midnight, "date:hh", "12"},
		{"mm", morning, "date:mm", "08"},
		{"m", morning, "date:m", "8"},
		{"ss", morning, "date:ss", "09"},
		{"s", morning, "date:s", "9"},
		{"SSS", morning, "date:SSS", "045"},
		{"SSS zero", midnight, "date:SSS", "000"},
		{"A morning", morning, "date:A", "AM"},
		{"A afternoon", afternoon, "date:A", "PM"},
		{"a morning", morning, "date:a", "am"},
		{"a afternoon", afternoon, "date:a", "pm"},
		{"X", morning, "date:X", "1772694489"},
		{"x", morning, "date:x", "1772694489045"},
		{"ZZ", morning, "date:ZZ", "+0000"},
		{"Z", morning, "date:Z", "+00:00"},
		{"padded and unpadded differ", morning, "date:MM/M DD/D HH/H mm/m ss/s", "03/3 05/5 07/7 08/8 09/9"},
		{"longest token wins", morning, "date:YYYYYY", "202626"},
		{"literals", morning, "date:YYYY-MM-DDTHH:mm:ss.SSS_/, 12", "2026-03-05T07:08:09.045_/, 12"},
		{"full timestamp", afternoon, "date:YYYY-MM-DD HH:mm:ss", "2026-12-25 15:04:05"},
		{"twelve hour clock", afternoon, "date:h:mm A", "3:04 PM"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := spec.FormatTimeMacro(c.at, c.format)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("FormatTimeMacro(%q) = %q, want %q", c.format, got, c.want)
			}
			if c.format != "" && !spec.ValidTimeMacroFormat(c.format) {
				t.Fatalf("ValidTimeMacroFormat(%q) = false", c.format)
			}
		})
	}
}

func TestInvalidTimeMacroFormats(t *testing.T) {
	for _, format := range []string{
		"",
		"bogus",
		"csv",
		"date:",
		"date:Q",
		"date:dd",
		"date:S",
		"date:YYYY'",
		"date:YYYY*",
		"date:é",
		"date:iso:x",
		"date:" + strings.Repeat("Y", 65),
		"DATE",
		"seconds",
	} {
		t.Run(format, func(t *testing.T) {
			if spec.ValidTimeMacroFormat(format) {
				t.Fatalf("ValidTimeMacroFormat(%q) = true", format)
			}
			if format == "" {
				return
			}
			if got, err := spec.FormatTimeMacro(morning, format); err == nil {
				t.Fatalf("FormatTimeMacro(%q) = %q, want an error", format, got)
			}
		})
	}
	if !spec.ValidTimeMacroFormat("date:" + strings.Repeat("Y", 64)) {
		t.Fatal("a 64 byte pattern must be valid")
	}
}
