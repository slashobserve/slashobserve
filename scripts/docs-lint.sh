#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
DOCS_DIR="${DOCS_DIR:-docs}"

files=()
while IFS= read -r file; do
  files+=("$file")
done < <(find "$DOCS_DIR" -type f -name '*.md' | LC_ALL=C sort)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "docs-lint: no Markdown files under $DOCS_DIR" >&2
  exit 1
fi

perl -CSD - "${files[@]}" <<'PERL'
use strict;
use warnings;
use File::Basename qw(dirname);

my $violations = 0;
my ($state, $fence, %front);

sub report {
  my ($rule) = @_;
  print "$ARGV:$.: $rule\n";
  $violations++;
}

sub prose {
  my ($text) = @_;
  $text =~ s/(`+).*?\1//g;
  $text =~ s/"[^"]*"//g;
  1 while $text =~ s/\([^()]*\)//g;
  return $text;
}

sub check_link {
  my ($target) = @_;
  $target =~ s/\s.*//;
  $target =~ s/^<(.*)>$/$1/;
  return if $target =~ m{^(?:[a-z][a-z0-9+.-]*:|#|/)}i;
  $target =~ s/[#?].*//;
  return unless $target =~ /\.(?:md|png|jpg|svg)$/i;
  my $path = dirname($ARGV) . "/" . $target;
  report("broken link $target") unless -f $path;
}

sub start_file {
  $state = "start";
  $fence = "";
  %front = ();
}

sub end_file {
  if ($state eq "front") {
    report("unclosed front matter");
  } else {
    for my $key ("title", "description") {
      next if $front{$key};
      print "$ARGV:1: front matter has no $key\n";
      $violations++;
    }
  }
}

start_file();
while (my $line = <>) {
  chomp $line;
  report("em or en dash") if $line =~ /[\x{2013}\x{2014}]/;

  if ($state eq "start") {
    $state = $line eq "---" ? "front" : "body";
    next if $state eq "front";
  }
  if ($state eq "front") {
    if ($line eq "---") {
      $state = "body";
    } elsif ($line =~ /^(title|description):\s*\S/) {
      $front{$1} = 1;
    }
    next;
  }

  if ($fence ne "") {
    $fence = "" if $line =~ /^\s*\Q$fence\E\s*$/;
    next;
  }
  if ($line =~ /^\s*(`{3,}|~{3,})/) {
    $fence = $1;
    next;
  }

  report("level 1 heading") if $line =~ /^# /;

  my $links = $line;
  $links =~ s/(`+).*?\1//g;
  report("image without alt text") while $links =~ /!\[\s*\]\(/g;
  check_link($1) while $links =~ /\]\(([^)]+)\)/g;
  check_link($1) if $links =~ /^\s*\[[^\]]+\]:\s*(\S+)/;

  next if $line =~ /^\s*$/;
  next if $line =~ /^\s*(?:[-*+]|\d+[.)])\s/;
  next if $line =~ /^\s*\|/;
  next if $line =~ /^\s*#/;
  report("more than one sentence on the line") if prose($line) =~ /\.\s+[A-Z]/;
}
continue {
  if (eof) {
    end_file();
    close ARGV;
    start_file();
  }
}

exit($violations > 0 ? 1 : 0);
PERL
