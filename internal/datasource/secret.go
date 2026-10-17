package datasource

import "log/slog"

const redacted = "[redacted]"

type Secret struct{ value *string }

func NewSecret(v string) Secret { return Secret{value: &v} }

func (s Secret) Reveal() string {
	if s.value == nil {
		return ""
	}
	return *s.value
}

func (s Secret) IsZero() bool { return s.value == nil || *s.value == "" }

func (s Secret) Equal(o Secret) bool { return s.Reveal() == o.Reveal() }

func (s Secret) String() string { return redacted }

func (s Secret) GoString() string { return redacted }

func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

func (s Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }
