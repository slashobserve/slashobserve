# Sourced by scripts/e2e.sh and scripts/replicas.sh: one rule for what a failure dump never prints.
# The token prefixes are every one the server mints: sok_ keys, sot_ run tokens, sob_ the harness's token,
# soa_ and sor_ OAuth access and refresh tokens, soc_ OAuth codes and pub_ public links.

read -r -d '' REDACT_PERL <<'PERL' || true
BEGIN { $lit = $ENV{REDACT_LITERAL} // ''; }
my $key = qr/(?!\w*tokens["']?\s*[:=]\s*\d+(?![\w.~+\/=-]))\w*(?:password|passwd|token|secret|api[_-]?key|private[_-]?key|access[_-]?key)\w*/i;
s{(://)[^/\s:\@]+:[^/\s\@]+\@}{${1}\[redacted]\@}g;
s/\b(so[kbtarc]|pub)_[A-Za-z0-9_-]+/${1}_\[redacted]/gi;
s/(Bearer\s+)[A-Za-z0-9._~+\/=-]+/${1}\[redacted]/gi;
s/\b((?:authorization|cookie|set-cookie|x-api-key|so_session)["']?\s*[:=]\s*)(?:(["'])(?:(?:\\.|(?!\2)[^\\])*\2|(?:(?!\2).)*\2)|.*)/defined $2 ? "${1}${2}\[redacted]${2}" : "${1}\[redacted]"/gie;
s/(so_session\s+)\S+/${1}\[redacted]/gi;
s/(--password)[ =]\S+/${1} \[redacted]/gi;
s/($key)(["']?\s*[:=]\s*)(["'])(?:(?:\\.|(?!\3)[^\\])*\3|(?:(?!\3).)*\3)/${1}${2}${3}\[redacted]${3}/g;
s/($key)(["']?\s*[:=]\s*)(?!\[redacted\])[^\s,;&"'}\])]+/${1}${2}\[redacted]/g;
s/([?&]code=)[^&\s#"']+/${1}\[redacted]/gi;
s/\Q$lit\E/\[redacted]/g if length $lit;
PERL

# REDACT_LITERAL, when set, is one more exact value to hide, such as a token the caller holds.
redact() {
  perl -pe "$REDACT_PERL"
}
